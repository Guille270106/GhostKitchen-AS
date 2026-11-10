package ports

import (
	"context"
	"time"

	"ghostkitchen/pedidos/internal/domain"
)

// CarritosRepo guarda el carrito activo de cada cliente.
type CarritosRepo interface {
	// ConBloqueo serializa los cambios del cliente, incluso entre instancias.
	ConBloqueo(ctx context.Context, clienteID string, hacer func(context.Context) error) error
	IniciarCambio(ctx context.Context, clienteID string, op CambioCarrito) error
	CambioPendiente(ctx context.Context, clienteID string) (*CambioCarrito, error)
	TerminarCambio(ctx context.Context, clienteID, id string, estado EstadoCambio) error
	ReintentarCompensacion(ctx context.Context, op CambioCarrito) error
	ClientesConCambiosPendientes(ctx context.Context) ([]string, error)
	// Obtener devuelve el carrito del cliente. Si todavía no tiene, devuelve uno vacío
	// con Version 0 (sin error).
	Obtener(ctx context.Context, clienteID string) (domain.Carrito, error)
	// Guardar escribe el carrito solo si sigue en versionAnterior. Si otro cambio se guardó
	// antes, devuelve domain.ErrCarritoModificado.
	// Si c.Operacion no está vacía, la anota en la misma transacción; si esa clave ya estaba,
	// no guarda nada y devuelve domain.ErrOperacionRepetida.
	// Si hay CambioID, el diario avanza en el mismo commit que el carrito.
	Guardar(ctx context.Context, c domain.Carrito, versionAnterior int64) error
	// OperacionAplicada dice si esa Idempotency-Key ya se aplicó al carrito del cliente.
	OperacionAplicada(ctx context.Context, clienteID, clave string) (bool, error)
	// MarcarVencido vacía el carrito que tiene esa reserva y anota que venció. Si el carrito ya no
	// tiene esa reserva (ya se vació, se confirmó o es otro), no hace nada y devuelve false: por
	// eso procesar el mismo aviso dos veces no cambia nada.
	MarcarVencido(ctx context.Context, carritoID, reservaID string) (bool, error)
}

type EstadoCambio string

const (
	CambioIncierto   EstadoCambio = "incierta"
	CambioPorLiberar EstadoCambio = "por_liberar"
	CambioCompletado EstadoCambio = "completada"
	CambioCompensado EstadoCambio = "compensada"
)

// CambioCarrito conserva el cuerpo exacto de la petición remota. La clave se
// asigna una sola vez, antes de enviarla a Inventario.
type CambioCarrito struct {
	ID           string
	Estado       EstadoCambio
	Destino      domain.Carrito
	Vaciar       bool
	Compensacion int64
}

// Catalogo es lo que Pedidos le pregunta a Catálogo.
type Catalogo interface {
	// ObtenerPlato devuelve el plato activo (con precio y cocina). Errores:
	// domain.ErrPlatoNoEncontrado o domain.ErrCatalogoCaido.
	ObtenerPlato(ctx context.Context, platoID string) (domain.PlatoCatalogo, error)
}

// ItemReserva es un plato y su cantidad, como los pide Inventario.
type ItemReserva struct {
	PlatoID  string `json:"platoId"`
	Cantidad int64  `json:"cantidad"`
}

// Inventario es lo que Pedidos le pide a Inventario para apartar ingredientes.
// La clave hace que repetir la misma operación (por un reintento) no la duplique.
type Inventario interface {
	// Reservar crea la reserva del carrito y devuelve su id y cuándo vence.
	Reservar(ctx context.Context, clave, carritoID, cocinaID string, items []ItemReserva) (reservaID string, venceEn time.Time, err error)
	// Ajustar deja en la reserva exactamente estos platos y cantidades.
	Ajustar(ctx context.Context, clave, reservaID string, items []ItemReserva) error
	// Liberar devuelve todos los ingredientes de la reserva.
	Liberar(ctx context.Context, clave, reservaID string) error
}
