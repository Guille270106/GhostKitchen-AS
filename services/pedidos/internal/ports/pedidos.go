// Package ports define lo que el centro de Pedidos necesita de afuera. Los adaptadores
// (MySQL, y más adelante Catálogo, Inventario, RabbitMQ y el proveedor) los implementan.
package ports

import (
	"context"

	"ghostkitchen/pedidos/internal/domain"
)

// Pedidos guarda y lee el agregado Pedido.
type Pedidos interface {
	// Crear guarda el pedido, sus items y su primera entrada del historial, todo junto.
	Crear(ctx context.Context, p domain.Pedido) error
	// Obtener devuelve el pedido con sus items e historial, o domain.ErrNoEncontrado.
	Obtener(ctx context.Context, id string) (domain.Pedido, error)
	// GuardarCambio guarda el nuevo estado y su entrada del historial solo si el pedido sigue
	// en la versión anterior al cambio. Si otro cambio se guardó antes, devuelve
	// domain.ErrConflictoVersion y no guarda nada.
	GuardarCambio(ctx context.Context, p domain.Pedido, c domain.CambioEstado) error
	// Ready comprueba la conexión y que las migraciones estén aplicadas.
	Ready(ctx context.Context) error
}
