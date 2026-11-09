package ports

import (
	"context"
	"ghostkitchen/inventario/internal/domain"
)

type Recetas interface {
	GuardarReceta(context.Context, domain.Receta) (domain.Receta, error)
	ObtenerReceta(context.Context, string, string) (domain.Receta, error)
	ListarRecetas(context.Context, string) ([]domain.Receta, error)
}
