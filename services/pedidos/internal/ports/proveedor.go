package ports

import (
	"context"

	"ghostkitchen/pedidos/internal/domain"
)

// SolicitudComprobante es lo que le pedimos al proveedor: a quién mandarle la factura y qué
// líneas lleva. Son nuestras palabras; el adaptador las traduce a su contrato (ADR-009).
type SolicitudComprobante struct {
	Email  string
	Lineas []domain.LineaComprobante
}

// ComprobanteEmitido es lo que nos devuelve el proveedor cuando emite la factura.
type ComprobanteEmitido struct {
	ID     string
	Numero string
}

// Proveedor es el grupo proveedor (Grupo 4): emite la factura y la manda por correo.
type Proveedor interface {
	// Emitir pide la factura. Con la misma clave se puede repetir sin riesgo: el proveedor
	// devuelve la misma y no manda otro correo. Errores: domain.ErrProveedorCaido (no sabemos si
	// la emitió: reintentar) o domain.ErrProveedorRechazo (no la va a emitir: no reintentar).
	Emitir(ctx context.Context, clave string, s SolicitudComprobante) (ComprobanteEmitido, error)
}

// ComprobantesRepo sigue las facturas pendientes. Las crea la confirmación, en la misma
// transacción que crea el pedido.
type ComprobantesRepo interface {
	// Pendientes devuelve hasta "limite" pedidos con la factura PENDIENTE, los más viejos primero.
	Pendientes(ctx context.Context, limite int) ([]string, error)
	// MarcarEmitido guarda el id y el número que dio el proveedor.
	MarcarEmitido(ctx context.Context, pedidoID string, c ComprobanteEmitido) error
	// MarcarRechazado anota que el proveedor la rechazó y por qué.
	MarcarRechazado(ctx context.Context, pedidoID, motivo string) error
	// SumarIntento anota que el proveedor no contestó; se vuelve a probar en la próxima vuelta.
	SumarIntento(ctx context.Context, pedidoID string) error
}
