package ports

import (
	"context"
	"ghostkitchen/inventario/internal/domain"
)

type Ingredientes interface {
	Crear(context.Context, domain.Ingrediente) error
	Listar(context.Context, string) ([]domain.Ingrediente, error)
	Obtener(context.Context, string, string) (domain.Ingrediente, error)
	MoverStock(context.Context, string, string, string, int64) (domain.Ingrediente, error)
}
