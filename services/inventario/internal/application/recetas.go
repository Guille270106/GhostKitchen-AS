package application

import (
	"context"
	"ghostkitchen/inventario/internal/domain"
	"ghostkitchen/inventario/internal/ports"
)

type Recetas struct{ repo ports.Recetas }

func NuevasRecetas(repo ports.Recetas) *Recetas { return &Recetas{repo: repo} }

func (s *Recetas) Guardar(ctx context.Context, quien domain.Identidad, plato string, items []domain.RecetaItem) (domain.Receta, error) {
	if err := quien.Validar(); err != nil {
		return domain.Receta{}, err
	}
	receta, err := domain.NuevaReceta(plato, quien.CocinaID, items)
	if err != nil {
		return receta, err
	}
	return s.repo.GuardarReceta(ctx, receta)
}
func (s *Recetas) Obtener(ctx context.Context, quien domain.Identidad, plato string) (domain.Receta, error) {
	if err := quien.Validar(); err != nil {
		return domain.Receta{}, err
	}
	return s.repo.ObtenerReceta(ctx, quien.CocinaID, plato)
}
func (s *Recetas) Listar(ctx context.Context, quien domain.Identidad) ([]domain.Receta, error) {
	if err := quien.Validar(); err != nil {
		return nil, err
	}
	return s.repo.ListarRecetas(ctx, quien.CocinaID)
}
