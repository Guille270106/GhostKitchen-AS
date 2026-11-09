// Package ports tiene los puertos de salida: lo que el centro de Pedidos necesita de afuera.
// Son interfaces; los adaptadores (MySQL, Catálogo, Inventario) las implementan, y en los
// tests las reemplazamos por mocks en memoria, igual que el ItemsMock de clase.
package ports

import (
	"context"

	"ghostkitchen/pedidos/internal/domain"
)

// PedidosRepo guarda y lee pedidos.
type PedidosRepo interface {
	// Crear guarda el pedido, sus items y su primera entrada del historial, todo junto.
	Crear(ctx context.Context, p domain.Pedido) error
	// Obtener devuelve el pedido con sus items e historial, o domain.ErrNoEncontrado.
	Obtener(ctx context.Context, id string) (domain.Pedido, error)
	// GuardarCambio guarda el nuevo estado solo si el pedido sigue en la versión anterior.
	// Si otro cambio se guardó antes, devuelve domain.ErrConflictoVersion.
	GuardarCambio(ctx context.Context, p domain.Pedido, c domain.CambioEstado) error
	// Ready comprueba la conexión y que las migraciones estén aplicadas.
	Ready(ctx context.Context) error
}
