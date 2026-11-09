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
	// Listar devuelve los pedidos de un cliente o de una cocina, del más nuevo al más viejo.
	Listar(ctx context.Context, f FiltroPedidos) ([]domain.Pedido, error)
	// Ready comprueba la conexión y que las migraciones estén aplicadas.
	Ready(ctx context.Context) error
}

// FiltroPedidos es qué pedidos listar: los de un cliente o los de una cocina (uno de los dos),
// y si se quiere, solo los de un estado. Se lista de a una página.
type FiltroPedidos struct {
	ClienteID string
	CocinaID  string
	Estado    domain.Estado // vacío: todos los estados
	Pagina    int           // desde 1
	PorPagina int
}
