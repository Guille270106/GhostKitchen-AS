package application

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

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

// Ver devuelve el carrito del cliente (vacío si todavía no tiene).
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
	return carrito, nil
}

// AgregarPlato suma un plato al carrito y aparta sus ingredientes en Inventario.
func (s CarritoService) AgregarPlato(ctx context.Context, actor domain.Actor, platoID string, cantidad int64) (domain.Carrito, error) {
	if err := validarCliente(actor); err != nil {
		return domain.Carrito{}, err
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
	if carrito.ID == "" {
		carrito.ID = uuid.NewString()
	}
	if err := carrito.Agregar(plato, cantidad); err != nil {
		return domain.Carrito{}, err
	}

	// 3. Reservamos en Inventario y guardamos.
	return s.reservarYGuardar(ctx, carrito, versionAnterior)
}

// CambiarCantidad deja una cantidad final para un plato del carrito.
func (s CarritoService) CambiarCantidad(ctx context.Context, actor domain.Actor, platoID string, cantidad int64) (domain.Carrito, error) {
	if err := validarCliente(actor); err != nil {
		return domain.Carrito{}, err
	}
	carrito, err := s.Repo.Obtener(ctx, actor.UsuarioID)
	if err != nil {
		return domain.Carrito{}, err
	}
	versionAnterior := carrito.Version
	if err := carrito.CambiarCantidad(platoID, cantidad); err != nil {
		return domain.Carrito{}, err
	}
	return s.reservarYGuardar(ctx, carrito, versionAnterior)
}

// QuitarPlato saca un plato. Si el carrito queda vacío, se libera toda la reserva.
func (s CarritoService) QuitarPlato(ctx context.Context, actor domain.Actor, platoID string) (domain.Carrito, error) {
	if err := validarCliente(actor); err != nil {
		return domain.Carrito{}, err
	}
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
}

// Vaciar saca todos los platos y libera la reserva.
func (s CarritoService) Vaciar(ctx context.Context, actor domain.Actor) (domain.Carrito, error) {
	if err := validarCliente(actor); err != nil {
		return domain.Carrito{}, err
	}
	carrito, err := s.Repo.Obtener(ctx, actor.UsuarioID)
	if err != nil {
		return domain.Carrito{}, err
	}
	if carrito.Vacio() {
		return carrito, nil
	}
	return s.vaciarYGuardar(ctx, carrito, carrito.Version)
}

// reservarYGuardar pide a Inventario que la reserva quede igual al carrito y después lo guarda.
func (s CarritoService) reservarYGuardar(ctx context.Context, carrito domain.Carrito, versionAnterior int64) (domain.Carrito, error) {
	items := itemsParaReserva(carrito)
	clave := claveDeCambio(carrito.ID, versionAnterior+1, items)

	var err error
	if carrito.ReservaID == "" {
		// Primer plato: se crea la reserva. Vence en 10 minutos y agregar más platos no lo renueva.
		reservaID, venceEn, errReserva := s.Inventario.Reservar(ctx, clave, carrito.ID, carrito.CocinaID, items)
		err = errReserva
		if err == nil {
			carrito.ReservaID = reservaID
			carrito.VenceEn = &venceEn
		}
	} else {
		// Ya había reserva: se le manda el contenido final del carrito.
		err = s.Inventario.Ajustar(ctx, clave, carrito.ReservaID, items)
	}

	if errors.Is(err, domain.ErrCarritoVencido) {
		// La reserva venció: Inventario ya liberó los ingredientes. Dejamos el carrito vacío.
		carrito.Reiniciar(uuid.NewString())
		carrito.Version = versionAnterior + 1
		_ = s.Repo.Guardar(ctx, carrito, versionAnterior)
		return domain.Carrito{}, domain.ErrCarritoVencido
	}
	if err != nil {
		return domain.Carrito{}, err
	}

	carrito.Version = versionAnterior + 1
	if err := s.Repo.Guardar(ctx, carrito, versionAnterior); err != nil {
		return domain.Carrito{}, err
	}
	return carrito, nil
}

// vaciarYGuardar libera la reserva y deja el carrito vacío, con otro id para la próxima.
func (s CarritoService) vaciarYGuardar(ctx context.Context, carrito domain.Carrito, versionAnterior int64) (domain.Carrito, error) {
	if carrito.ReservaID != "" {
		clave := fmt.Sprintf("%s-liberar", carrito.ReservaID)
		err := s.Inventario.Liberar(ctx, clave, carrito.ReservaID)
		// Si ya había vencido, los ingredientes ya están libres: seguimos igual.
		if err != nil && !errors.Is(err, domain.ErrCarritoVencido) {
			return domain.Carrito{}, err
		}
	}
	carrito.Reiniciar(uuid.NewString())
	carrito.Version = versionAnterior + 1
	if err := s.Repo.Guardar(ctx, carrito, versionAnterior); err != nil {
		return domain.Carrito{}, err
	}
	return carrito, nil
}

// claveDeCambio arma la Idempotency-Key: carrito + número de cambio + una huella de los platos.
//   - Si reintentamos el MISMO cambio (por ejemplo, después de un timeout), sale la misma clave
//     e Inventario no reserva dos veces (ADR-005).
//   - Si el intento anterior falló (por ejemplo, sin stock), el número de cambio no avanzó; la
//     huella hace que el siguiente cambio, con otros platos, use otra clave.
func claveDeCambio(carritoID string, numeroDeCambio int64, items []ports.ItemReserva) string {
	texto := ""
	for _, it := range items {
		texto += fmt.Sprintf("%s:%d;", it.PlatoID, it.Cantidad)
	}
	huella := sha256.Sum256([]byte(texto))
	return fmt.Sprintf("%s-%d-%x", carritoID, numeroDeCambio, huella[:6])
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
