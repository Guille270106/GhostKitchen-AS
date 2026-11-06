package application

import (
	"context"
	"ghostkitchen/inventario/internal/domain"
	"ghostkitchen/inventario/internal/ports"
	"github.com/google/uuid"
)

type Ingredientes struct{ repo ports.Ingredientes }

func Nuevo(repo ports.Ingredientes) *Ingredientes { return &Ingredientes{repo: repo} }

func (s *Ingredientes) Crear(ctx context.Context, quien domain.Identidad, nombre, unidad string, inicial int64) (domain.Ingrediente, error) {
	if err := quien.Validar(); err != nil {
		return domain.Ingrediente{}, err
	}
	i, err := domain.NuevoIngrediente(uuid.NewString(), quien.CocinaID, nombre, unidad, inicial)
	if err != nil {
		return i, err
	}
	return i, s.repo.Crear(ctx, i)
}
func (s *Ingredientes) Listar(ctx context.Context, quien domain.Identidad) ([]domain.Ingrediente, error) {
	if err := quien.Validar(); err != nil {
		return nil, err
	}
	return s.repo.Listar(ctx, quien.CocinaID)
}
func (s *Ingredientes) Obtener(ctx context.Context, quien domain.Identidad, id string) (domain.Ingrediente, error) {
	if err := quien.Validar(); err != nil {
		return domain.Ingrediente{}, err
	}
	return s.repo.Obtener(ctx, quien.CocinaID, id)
}
