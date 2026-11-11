package application

import (
	"context"
	"errors"
	"time"

	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/ports"
	"github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"
)

// ResultadoConfirmacion es cómo quedó la confirmación. Si se completó, trae el pedido.
type ResultadoConfirmacion struct {
	Confirmacion domain.Confirmacion
	Pedido       *domain.Pedido
}

// ConfirmacionService confirma el pedido como una saga coordinada por Pedidos (ADR-004):
//
//  1. Anota que la confirmación empezó (con una copia del carrito).
//  2. Le pide a Inventario que descuente lo reservado.
//  3. Crea el pedido en RECIBIDO y vacía el carrito, en una sola transacción.
//
// La factura del proveedor no es un paso de la saga: se pide en segundo plano después (ADR-009).
//
// Cada paso queda guardado. Si no se sabe cómo terminó un paso (Inventario no contestó), NO se
// dice que falló: se responde "Estamos confirmando tu pedido" y un proceso en segundo plano
// sigue desde el paso guardado.
type ConfirmacionService struct {
	Repo     ports.ConfirmacionesRepo
	Carritos ports.CarritosRepo
	Pedidos  ports.PedidosRepo
	Consumo  ports.Consumo
	// Envios da la tarifa de la cocina y el nombre de sus restaurantes al confirmar. Sin él, el
	// envío queda sin registrar.
	Envios ports.Envios
}

// Confirmar es lo que pasa cuando el cliente aprieta "Confirmar pedido". direccion es adónde
// se lleva el pedido; se guarda con el pedido para que la cocina la vea.
func (s ConfirmacionService) Confirmar(ctx context.Context, actor domain.Actor, clave, direccion string) (ResultadoConfirmacion, error) {
	if err := validarCliente(actor); err != nil {
		return ResultadoConfirmacion{}, err
	}
	if !domain.ClaveValida(clave) {
		return ResultadoConfirmacion{}, domain.ErrClaveInvalida
	}
	direccion, err := domain.NormalizarDireccion(direccion)
	if err != nil {
		return ResultadoConfirmacion{}, err
	}

	var conf domain.Confirmacion
	err = s.Carritos.ConBloqueo(ctx, actor.UsuarioID, func(ctx context.Context) error {
		pendiente, err := s.Carritos.CambioPendiente(ctx, actor.UsuarioID)
		if err != nil {
			return err
		}
		if pendiente != nil {
			return domain.ErrCarritoModificado
		}
		conf, err = s.iniciarConfirmacion(ctx, actor, clave, direccion)
		return err
	})
	if err != nil {
		return ResultadoConfirmacion{}, err
	}
	return s.avanzar(ctx, conf)
}

func (s ConfirmacionService) iniciarConfirmacion(ctx context.Context, actor domain.Actor, clave, direccion string) (domain.Confirmacion, error) {
	// 1. ¿Ya usó esta clave? (doble clic o reintento del navegador): devolvemos esa misma
	//    confirmación y no creamos otro pedido (con la dirección de la primera vez).
	existente, err := s.Repo.BuscarPorClave(ctx, actor.UsuarioID, clave)
	if err == nil {
		return existente, nil
	}
	if !errors.Is(err, domain.ErrNoEncontrado) {
		return domain.Confirmacion{}, err
	}

	// 2. Armamos la confirmación con una copia del carrito (el dominio valida que se pueda).
	carrito, err := s.Carritos.Obtener(ctx, actor.UsuarioID)
	if err != nil {
		return domain.Confirmacion{}, err
	}
	nueva, err := domain.NuevaConfirmacion(uuid.NewString(), uuid.NewString(), actor, clave, carrito, time.Now())
	if err != nil {
		return domain.Confirmacion{}, err
	}
	nueva.DireccionEntrega = direccion

	// 2b. El envío que se cobra: la tarifa de la cocina ahora. Si Catálogo no contesta, todavía no
	//     se hizo nada (ni se descontó stock): se responde que no está disponible.
	if s.Envios != nil {
		cocina, err := s.Envios.DatosDeCocina(ctx, carrito.CocinaID)
		if err != nil {
			return domain.Confirmacion{}, err
		}
		envio := cocina.CostoEnvio
		nueva.CostoEnvio = &envio
		// El nombre de cada restaurante queda copiado en el pedido (lo muestran la cocina, las
		// métricas y la factura), igual que el precio: si después lo renombran, el pedido no cambia.
		for i, it := range nueva.Items {
			nueva.Items[i].RestauranteNombre = cocina.Restaurantes[it.RestauranteID]
		}
	}

	// 3. La guardamos. Si dos solicitudes llegan a la vez para el mismo carrito, la base deja
	//    guardar una sola y las dos siguen con la misma confirmación.
	conf, _, err := s.Repo.Iniciar(ctx, nueva)
	if err != nil {
		return domain.Confirmacion{}, err
	}

	return conf, nil
}

// Ver devuelve cómo va una confirmación (para cuando se respondió "Estamos confirmando").
func (s ConfirmacionService) Ver(ctx context.Context, actor domain.Actor, id string) (domain.Confirmacion, error) {
	if err := validarCliente(actor); err != nil {
		return domain.Confirmacion{}, err
	}
	conf, err := s.Repo.Obtener(ctx, id)
	if err != nil {
		return domain.Confirmacion{}, err
	}
	if conf.ClienteID != actor.UsuarioID {
		return domain.Confirmacion{}, domain.ErrNoEncontrado
	}
	return conf, nil
}

// RecuperarPendientes sigue las confirmaciones que quedaron a medias (por ejemplo, porque
// Inventario no contestó o porque Pedidos se reinició). La llama main cada 10 segundos.
func (s ConfirmacionService) RecuperarPendientes(ctx context.Context) (revisadas int, err error) {
	// Esperamos 5 s antes de tocarlas, así no competimos con una solicitud que sigue en curso.
	pendientes, err := s.Repo.Pendientes(ctx, time.Now().Add(-5*time.Second))
	if err != nil {
		return 0, err
	}
	for _, conf := range pendientes {
		conPlazo, cancelar := context.WithTimeout(ctx, 3*time.Second)
		_, _ = s.avanzar(conPlazo, conf)
		cancelar()
	}
	return len(pendientes), nil
}

// avanzar hace los pasos que le falten a la confirmación, desde el que tenga guardado.
func (s ConfirmacionService) avanzar(ctx context.Context, conf domain.Confirmacion) (ResultadoConfirmacion, error) {
	// Ya terminó: devolvemos el mismo resultado de la primera vez.
	switch conf.Estado {
	case domain.ConfirmacionCompletada:
		pedido, err := s.Pedidos.Obtener(ctx, conf.PedidoID)
		if err != nil {
			return ResultadoConfirmacion{}, err
		}
		return ResultadoConfirmacion{Confirmacion: conf, Pedido: &pedido}, nil
	case domain.ConfirmacionFallida:
		return ResultadoConfirmacion{Confirmacion: conf}, errorDeMotivo(conf.Motivo)
	}

	// Paso: descontar en Inventario.
	if conf.Paso == domain.PasoDescontar {
		err := s.descontar(ctx, conf)
		if errors.Is(err, domain.ErrCarritoVencido) {
			// Respuesta segura de Inventario: no se descontó nada. La saga falla y el carrito
			// se vacía (los ingredientes ya los liberó Inventario).
			if err := s.Repo.Fallar(ctx, conf, "RESERVA_VENCIDA"); err != nil {
				return ResultadoConfirmacion{Confirmacion: conf}, nil
			}
			return ResultadoConfirmacion{}, domain.ErrCarritoVencido
		}
		if err != nil {
			// No sabemos si se descontó: queda EN_CURSO y sigue en segundo plano.
			return ResultadoConfirmacion{Confirmacion: conf}, nil
		}
		if err := s.Repo.MarcarPaso(ctx, conf.ID, domain.PasoCrearPedido); err != nil {
			return ResultadoConfirmacion{Confirmacion: conf}, nil
		}
		conf.Paso = domain.PasoCrearPedido
	}

	// (Acá va el paso del grupo proveedor, L15.)

	// Paso: crear el pedido en RECIBIDO, marcar la confirmación y vaciar el carrito (juntos).
	pedido, err := conf.CrearPedido(time.Now())
	if err != nil {
		return ResultadoConfirmacion{}, err
	}
	if err := s.Repo.Completar(ctx, conf, pedido); err != nil {
		// El stock ya se descontó: no se devuelve nada. Se reintenta en segundo plano.
		return ResultadoConfirmacion{Confirmacion: conf}, nil
	}
	conf.Estado = domain.ConfirmacionCompletada
	return ResultadoConfirmacion{Confirmacion: conf, Pedido: &pedido}, nil
}

// descontar le pide a Inventario que descuente, reintentando con espera creciente y azar
// (backoff con jitter) mientras Inventario no conteste. Usa siempre la MISMA clave:
// si un intento anterior sí llegó, Inventario no descuenta dos veces.
func (s ConfirmacionService) descontar(ctx context.Context, conf domain.Confirmacion) error {
	// Dejamos 1 s de los 3 s de la solicitud para guardar el pedido después.
	conPlazo, cancelar := context.WithTimeout(ctx, 2*time.Second)
	defer cancelar()

	espera := backoff.NewExponentialBackOff()
	espera.InitialInterval = 100 * time.Millisecond // primer reintento a los ~100 ms
	espera.Multiplier = 2                           // después ~200 ms, ~400 ms...
	espera.RandomizationFactor = 0.5                // ±50 % de azar: que no reintenten todos juntos
	espera.MaxInterval = time.Second

	clave := conf.ID + "-descontar"
	intento := func() error {
		err := s.Consumo.Consumir(conPlazo, clave, conf.ReservaID, conf.PedidoID)
		if errors.Is(err, domain.ErrInventarioCaido) {
			return err // no contestó: vale la pena reintentar
		}
		if err != nil {
			return backoff.Permanent(err) // respuesta definitiva (por ejemplo, venció): no reintentar
		}
		return nil
	}
	return backoff.Retry(intento, backoff.WithContext(espera, conPlazo))
}

// errorDeMotivo traduce el motivo guardado de una confirmación fallida.
func errorDeMotivo(motivo string) error {
	if motivo == "RESERVA_VENCIDA" {
		return domain.ErrCarritoVencido
	}
	return domain.ErrCarritoVacio
}
