package application

import (
	"context"

	"ghostkitchen/inventario/internal/domain"
	"ghostkitchen/inventario/internal/ports"
)

type Consumos struct{ repo ports.Consumos }

func NuevosConsumos(repo ports.Consumos) *Consumos { return &Consumos{repo: repo} }

func (s *Consumos) Consumir(ctx context.Context, clave, reservaID, pedidoID string) (domain.Consumo, error) {
	if err := domain.ValidarClave(clave); err != nil {
		return domain.Consumo{}, err
	}
	if !domain.IdentificadorValido(reservaID) || !domain.IdentificadorValido(pedidoID) {
		return domain.Consumo{}, domain.ErrReservaInvalida
	}
	return s.repo.Consumir(ctx, clave, reservaID, pedidoID)
}

func (s *Consumos) Devolver(ctx context.Context, clave, pedidoID string) (domain.Consumo, error) {
	if err := domain.ValidarClave(clave); err != nil {
		return domain.Consumo{}, err
	}
	if !domain.IdentificadorValido(pedidoID) {
		return domain.Consumo{}, domain.ErrReservaInvalida
	}
	return s.repo.Devolver(ctx, clave, pedidoID)
}

func (s *Consumos) Obtener(ctx context.Context, pedidoID string) (domain.Consumo, error) {
	if !domain.IdentificadorValido(pedidoID) {
		return domain.Consumo{}, domain.ErrReservaInvalida
	}
	return s.repo.ObtenerConsumo(ctx, pedidoID)
}
