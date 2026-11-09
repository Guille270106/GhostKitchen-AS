package ports

import "context"

// DevolucionPendiente es una devolución de ingredientes que todavía no se le mandó a Inventario.
type DevolucionPendiente struct {
	PedidoID string
	Intentos int
}

// DevolucionesRepo sigue las devoluciones pendientes. Las crea GuardarCambio, en la misma
// transacción que el cambio del pedido a CANCELADO.
type DevolucionesRepo interface {
	// Pendientes devuelve hasta "limite" devoluciones sin hacer, las más viejas primero.
	Pendientes(ctx context.Context, limite int) ([]DevolucionPendiente, error)
	// MarcarHecha anota que Inventario ya devolvió los ingredientes.
	MarcarHecha(ctx context.Context, pedidoID string) error
	// SumarIntento anota que Inventario no contestó; se vuelve a probar en la próxima vuelta.
	SumarIntento(ctx context.Context, pedidoID string) error
}

// Devolucion es el paso de Inventario al cancelar: devolver lo que se descontó para el pedido.
type Devolucion interface {
	// Devolver devuelve los ingredientes descontados para ese pedido. Con la misma clave se
	// puede repetir sin riesgo: Inventario no devuelve dos veces.
	// Errores: domain.ErrNoEncontrado (Inventario no tiene un descuento de ese pedido: no hay
	// nada que devolver) o domain.ErrInventarioCaido (no sabemos si se hizo).
	Devolver(ctx context.Context, clave, pedidoID string) error
}
