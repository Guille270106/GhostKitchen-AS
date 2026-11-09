package ports

import (
	"context"
	"time"

	"ghostkitchen/pedidos/internal/domain"
)

// ConfirmacionesRepo guarda las confirmaciones (la saga) y en qué paso va cada una.
type ConfirmacionesRepo interface {
	// Iniciar guarda una confirmación nueva. Si el cliente ya usó esa clave, o ese carrito ya
	// se está confirmando, no crea otra: devuelve la que ya existe y existia=true.
	Iniciar(ctx context.Context, c domain.Confirmacion) (guardada domain.Confirmacion, existia bool, err error)
	// Obtener devuelve una confirmación por id, o domain.ErrNoEncontrado.
	Obtener(ctx context.Context, id string) (domain.Confirmacion, error)
	// BuscarPorClave devuelve la confirmación de ese cliente con esa Idempotency-Key, o
	// domain.ErrNoEncontrado si todavía no la usó.
	BuscarPorClave(ctx context.Context, clienteID, clave string) (domain.Confirmacion, error)
	// MarcarPaso anota que la confirmación pasó al paso indicado.
	MarcarPaso(ctx context.Context, id string, paso domain.PasoConfirmacion) error
	// Completar crea el pedido, marca la confirmación COMPLETADA y vacía el carrito, todo en
	// una sola transacción. Si ya estaba completada, no hace nada.
	Completar(ctx context.Context, c domain.Confirmacion, p domain.Pedido) error
	// Fallar marca la confirmación FALLIDA con su motivo y vacía el carrito.
	Fallar(ctx context.Context, c domain.Confirmacion, motivo string) error
	// Pendientes devuelve las confirmaciones EN_CURSO que no cambiaron desde antes de "antes".
	Pendientes(ctx context.Context, antes time.Time) ([]domain.Confirmacion, error)
}

// Consumo es el paso de Inventario en la confirmación: descontar lo reservado.
type Consumo interface {
	// Consumir descuenta la reserva para ese pedido. Con la misma clave se puede repetir sin
	// riesgo: si ya se descontó, Inventario devuelve el mismo resultado y no descuenta dos veces.
	// Errores: domain.ErrCarritoVencido (la reserva venció) o domain.ErrInventarioCaido
	// (no sabemos si se hizo).
	Consumir(ctx context.Context, clave, reservaID, pedidoID string) error
}
