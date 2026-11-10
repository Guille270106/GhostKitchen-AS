package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/ports"
	"github.com/google/uuid"
)

// CarritoService tiene los casos de uso del carrito. Usa tres puertos: dónde se guarda el
// carrito, Catálogo (precio y cocina del plato) e Inventario (reservar los ingredientes).
type CarritoService struct {
	Repo       ports.CarritosRepo
	Catalogo   ports.Catalogo
	Inventario ports.Inventario
}

// Ver devuelve el carrito del cliente (vacío si todavía no tiene) con el tiempo que le queda
// a la reserva. Si ya venció lo avisa con "vencido": true; no lo cambia, porque mirar el
// carrito no tiene que modificar nada. El próximo cambio lo vacía (Inventario responde que venció).
func (s CarritoService) Ver(ctx context.Context, actor domain.Actor) (domain.Carrito, error) {
	if err := validarCliente(actor); err != nil {
		return domain.Carrito{}, err
	}
	carrito, err := s.Repo.Obtener(ctx, actor.UsuarioID)
	if err != nil {
		return domain.Carrito{}, err
	}
	if carrito.Items == nil {
		carrito.Items = []domain.ItemCarrito{}
	}
	carrito.ActualizarTiempo(time.Now())
	return carrito, nil
}

// AgregarPlato suma un plato al carrito y aparta sus ingredientes en Inventario.
func (s CarritoService) AgregarPlato(ctx context.Context, actor domain.Actor, platoID string, cantidad int64) (domain.Carrito, error) {
	return s.AgregarPlatoConClave(ctx, actor, platoID, cantidad, "")
}

// AgregarPlatoConClave es AgregarPlato con la Idempotency-Key del cliente. "Agregar" suma, así
// que reenviar la misma solicitud (por ejemplo, porque se cortó la respuesta) sumaría dos veces.
// Con la clave no: si esa clave ya se aplicó, se devuelve el carrito como está, sin tocar nada.
// Dos clics distintos mandan claves distintas, así que se suman los dos. La clave es opcional.
func (s CarritoService) AgregarPlatoConClave(ctx context.Context, actor domain.Actor, platoID string, cantidad int64, clave string) (domain.Carrito, error) {
	if err := validarCliente(actor); err != nil {
		return domain.Carrito{}, err
	}
	return s.cambiar(ctx, actor.UsuarioID, func(ctx context.Context) (domain.Carrito, error) {
		if clave != "" {
			if !domain.ClaveValida(clave) {
				return domain.Carrito{}, domain.ErrClaveInvalida
			}
			aplicada, err := s.Repo.OperacionAplicada(ctx, actor.UsuarioID, clave)
			if err != nil {
				return domain.Carrito{}, err
			}
			if aplicada {
				return s.Ver(ctx, actor)
			}
		}

		// 1. Le preguntamos a Catálogo el plato: si existe, su precio y de qué cocina es.
		plato, err := s.Catalogo.ObtenerPlato(ctx, platoID)
		if err != nil {
			return domain.Carrito{}, err
		}

		// 2. Buscamos el carrito del cliente y le agregamos el plato (el dominio valida las reglas).
		carrito, err := s.Repo.Obtener(ctx, actor.UsuarioID)
		if err != nil {
			return domain.Carrito{}, err
		}
		versionAnterior := carrito.Version
		if carrito.ReservaID == "" {
			carrito.ID = uuid.NewString()
		}
		if err := carrito.Agregar(plato, cantidad); err != nil {
			return domain.Carrito{}, err
		}
		carrito.Operacion = clave

		// 3. Reservamos en Inventario y guardamos.
		resultado, err := s.reservarYGuardar(ctx, carrito, versionAnterior)
		if errors.Is(err, domain.ErrOperacionRepetida) {
			// La misma clave llegó dos veces a la vez y la otra ganó: ya está sumado una vez.
			return s.Ver(ctx, actor)
		}
		return resultado, err
	})
}

// CambiarCantidad deja una cantidad final para un plato del carrito.
func (s CarritoService) CambiarCantidad(ctx context.Context, actor domain.Actor, platoID string, cantidad int64) (domain.Carrito, error) {
	if err := validarCliente(actor); err != nil {
		return domain.Carrito{}, err
	}
	return s.cambiar(ctx, actor.UsuarioID, func(ctx context.Context) (domain.Carrito, error) {
		carrito, err := s.Repo.Obtener(ctx, actor.UsuarioID)
		if err != nil {
			return domain.Carrito{}, err
		}
		versionAnterior := carrito.Version
		if err := carrito.CambiarCantidad(platoID, cantidad); err != nil {
			return domain.Carrito{}, err
		}
		return s.reservarYGuardar(ctx, carrito, versionAnterior)
	})
}

// QuitarPlato saca un plato. Si el carrito queda vacío, se libera toda la reserva.
func (s CarritoService) QuitarPlato(ctx context.Context, actor domain.Actor, platoID string) (domain.Carrito, error) {
	if err := validarCliente(actor); err != nil {
		return domain.Carrito{}, err
	}
	return s.cambiar(ctx, actor.UsuarioID, func(ctx context.Context) (domain.Carrito, error) {
		carrito, err := s.Repo.Obtener(ctx, actor.UsuarioID)
		if err != nil {
			return domain.Carrito{}, err
		}
		versionAnterior := carrito.Version
		if err := carrito.Quitar(platoID); err != nil {
			return domain.Carrito{}, err
		}
		if carrito.Vacio() {
			return s.vaciarYGuardar(ctx, carrito, versionAnterior)
		}
		return s.reservarYGuardar(ctx, carrito, versionAnterior)
	})
}

// Vaciar saca todos los platos y libera la reserva.
func (s CarritoService) Vaciar(ctx context.Context, actor domain.Actor) (domain.Carrito, error) {
	if err := validarCliente(actor); err != nil {
		return domain.Carrito{}, err
	}
	return s.cambiar(ctx, actor.UsuarioID, func(ctx context.Context) (domain.Carrito, error) {
		carrito, err := s.Repo.Obtener(ctx, actor.UsuarioID)
		if err != nil {
			return domain.Carrito{}, err
		}
		if carrito.Vacio() {
			carrito.ActualizarTiempo(time.Now())
			return carrito, nil
		}
		return s.vaciarYGuardar(ctx, carrito, carrito.Version)
	})
}

func (s CarritoService) cambiar(ctx context.Context, clienteID string, hacer func(context.Context) (domain.Carrito, error)) (domain.Carrito, error) {
	var resultado domain.Carrito
	err := s.Repo.ConBloqueo(ctx, clienteID, func(ctx context.Context) error {
		if err := s.recuperarCambio(ctx, clienteID); err != nil {
			return err
		}
		var err error
		resultado, err = hacer(ctx)
		return err
	})
	return resultado, err
}

func (s CarritoService) reservarYGuardar(ctx context.Context, carrito domain.Carrito, versionAnterior int64) (domain.Carrito, error) {
	op := ports.CambioCarrito{ID: uuid.NewString(), Estado: ports.CambioIncierto, Destino: carrito}
	if err := s.Repo.IniciarCambio(ctx, carrito.ClienteID, op); err != nil {
		return domain.Carrito{}, err
	}
	carrito, err := s.aplicarReserva(ctx, op)
	if err != nil {
		if !reservaRechazada(err) {
			return domain.Carrito{}, err
		}
		return domain.Carrito{}, errors.Join(err, s.cerrarRechazo(ctx, op, err))
	}
	carrito.VencioEn = nil
	carrito.Version = versionAnterior + 1
	carrito.CambioID = op.ID
	if err := s.Repo.Guardar(ctx, carrito, versionAnterior); err != nil {
		// Una respuesta de commit perdida también llega aquí. La recuperación
		// consulta el diario antes de decidir si corresponde compensar.
		recuperacion, cancelar := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancelar()
		return domain.Carrito{}, errors.Join(err, s.recuperarCambio(recuperacion, carrito.ClienteID))
	}
	carrito.ActualizarTiempo(time.Now())
	return carrito, nil
}

func (s CarritoService) aplicarReserva(ctx context.Context, op ports.CambioCarrito) (domain.Carrito, error) {
	c := op.Destino
	clave := "carrito-" + op.ID
	items := itemsParaReserva(c)
	if c.ReservaID != "" {
		return c, s.Inventario.Ajustar(ctx, clave, c.ReservaID, items)
	}
	id, vence, err := s.Inventario.Reservar(ctx, clave, c.ID, c.CocinaID, items)
	if err != nil {
		return c, err
	}
	c.ReservaID, c.VenceEn = id, &vence
	return c, nil
}

func reservaRechazada(err error) bool {
	return errors.Is(err, domain.ErrCarritoVencido) || errors.Is(err, domain.ErrSinStock) ||
		errors.Is(err, domain.ErrPlatoNoDisponible) || errors.Is(err, domain.ErrCarritoModificado)
}

func (s CarritoService) cerrarRechazo(ctx context.Context, op ports.CambioCarrito, rechazo error) error {
	c := op.Destino
	if errors.Is(rechazo, domain.ErrCarritoVencido) && c.ReservaID != "" {
		if _, err := s.Repo.MarcarVencido(ctx, c.ID, c.ReservaID); err != nil {
			return err
		}
	}
	return s.Repo.TerminarCambio(ctx, c.ClienteID, op.ID, ports.CambioCompensado)
}

func (s CarritoService) recuperarCambio(ctx context.Context, clienteID string) error {
	op, err := s.Repo.CambioPendiente(ctx, clienteID)
	if err != nil || op == nil {
		return err
	}
	if op.Estado == ports.CambioPorLiberar {
		return s.liberarGuardado(ctx, *op)
	}
	if op.Vaciar {
		// Si el commit no ocurrió, todavía no se pidió liberar.
		return s.Repo.TerminarCambio(ctx, clienteID, op.ID, ports.CambioCompensado)
	}
	return s.reconciliar(ctx, *op)
}

func (s CarritoService) reconciliar(ctx context.Context, op ports.CambioCarrito) error {
	// Repetir la petición original resuelve la incertidumbre de un timeout.
	// Su resultado queda fijado antes de enviar la compensación.
	intento, err := s.aplicarReserva(ctx, op)
	if reservaRechazada(err) {
		return s.cerrarRechazo(ctx, op, err)
	}
	if err != nil {
		return err
	}
	guardado, err := s.Repo.Obtener(ctx, intento.ClienteID)
	if err != nil {
		return err
	}
	clave := fmt.Sprintf("compensar-%s-%d", op.ID, op.Compensacion)
	if guardado.ReservaID != intento.ReservaID || guardado.Vacio() {
		err = s.Inventario.Liberar(ctx, clave, intento.ReservaID)
	} else {
		err = s.Inventario.Ajustar(ctx, clave, intento.ReservaID, itemsParaReserva(guardado))
	}
	if errors.Is(err, domain.ErrCarritoVencido) {
		return s.cerrarRechazo(ctx, op, err)
	}
	if errors.Is(err, domain.ErrSinStock) || errors.Is(err, domain.ErrPlatoNoDisponible) {
		// Al reducir una reserva, otro carrito puede tomar el stock liberado.
		// Inventario cachea ese rechazo. Solo después de conocerlo avanzamos
		// el intento de compensación, que se vuelve a evaluar al recuperar.
		op.Compensacion++
		return errors.Join(err, s.Repo.ReintentarCompensacion(ctx, op))
	}
	if err != nil {
		return err
	}
	return s.Repo.TerminarCambio(ctx, intento.ClienteID, op.ID, ports.CambioCompensado)
}

func (s CarritoService) vaciarYGuardar(ctx context.Context, carrito domain.Carrito, versionAnterior int64) (domain.Carrito, error) {
	op := ports.CambioCarrito{ID: uuid.NewString(), Estado: ports.CambioIncierto, Destino: carrito, Vaciar: true}
	if err := s.Repo.IniciarCambio(ctx, carrito.ClienteID, op); err != nil {
		return domain.Carrito{}, err
	}
	carrito.Reiniciar(uuid.NewString())
	carrito.Version = versionAnterior + 1
	carrito.CambioID = op.ID
	carrito.LiberacionPendiente = true
	if err := s.Repo.Guardar(ctx, carrito, versionAnterior); err != nil {
		return domain.Carrito{}, err
	}
	if err := s.liberarGuardado(ctx, op); err != nil {
		return domain.Carrito{}, err
	}
	carrito.ActualizarTiempo(time.Now())
	return carrito, nil
}

func (s CarritoService) liberarGuardado(ctx context.Context, op ports.CambioCarrito) error {
	c := op.Destino
	if c.ReservaID != "" {
		err := s.Inventario.Liberar(ctx, "liberar-"+op.ID, c.ReservaID)
		if err != nil && !errors.Is(err, domain.ErrCarritoVencido) {
			return err
		}
	}
	return s.Repo.TerminarCambio(ctx, c.ClienteID, op.ID, ports.CambioCompletado)
}

func (s CarritoService) RecuperarCambiosPendientes(ctx context.Context) error {
	clientes, err := s.Repo.ClientesConCambiosPendientes(ctx)
	if err != nil {
		return err
	}
	var fallos []error
	for _, clienteID := range clientes {
		plazo, cancelar := context.WithTimeout(ctx, 5*time.Second)
		err := s.Repo.ConBloqueo(plazo, clienteID, func(ctx context.Context) error {
			return s.recuperarCambio(ctx, clienteID)
		})
		cancelar()
		if err != nil {
			fallos = append(fallos, fmt.Errorf("recuperar cambio de %s: %w", clienteID, err))
		}
	}
	return errors.Join(fallos...)
}

func itemsParaReserva(c domain.Carrito) []ports.ItemReserva {
	items := []ports.ItemReserva{}
	for _, it := range c.Items {
		items = append(items, ports.ItemReserva{PlatoID: it.PlatoID, Cantidad: it.Cantidad})
	}
	return items
}

// validarCliente: el carrito es solo del rol cliente.
func validarCliente(actor domain.Actor) error {
	if err := actor.Validar(); err != nil {
		return err
	}
	if actor.Rol != domain.RolCliente {
		return domain.ErrSinPermiso
	}
	return nil
}

// ReservaVencida atiende el aviso reserva.vencida de Inventario (L14): la reserva del carrito
// venció y sus ingredientes ya se liberaron, así que el carrito se vacía y queda marcado como
// vencido ("Tu carrito venció"). Si el carrito ya no tiene esa reserva, no hace nada: el mismo
// aviso dos veces, o uno viejo que llega tarde, no cambia nada.
func (s CarritoService) ReservaVencida(ctx context.Context, reservaID, carritoID string) error {
	if !domain.IdentificadorValido(reservaID) || !domain.IdentificadorValido(carritoID) {
		return domain.ErrDatosInvalidos
	}
	_, err := s.Repo.MarcarVencido(ctx, carritoID, reservaID)
	return err
}
