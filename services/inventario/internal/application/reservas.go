package application

import (
	"context"
	"ghostkitchen/inventario/internal/domain"
	"ghostkitchen/inventario/internal/ports"
)

type Reservas struct{ repo ports.Reservas }

func NuevasReservas(repo ports.Reservas) *Reservas { return &Reservas{repo: repo} }
func (s *Reservas) Crear(ctx context.Context, clave string, datos domain.SolicitudReserva) (domain.Reserva, error) {
	if err := domain.ValidarClave(clave); err != nil {
		return domain.Reserva{}, err
	}
	if !domain.IdentificadorValido(datos.CarritoID) || !domain.IdentificadorValido(datos.CocinaID) {
		return domain.Reserva{}, domain.ErrReservaInvalida
	}
	items, err := domain.NormalizarItems(datos.Items, false)
	if err != nil {
		return domain.Reserva{}, err
	}
	datos.Items = items
	return s.repo.CrearReserva(ctx, clave, datos)
}
func (s *Reservas) Ajustar(ctx context.Context, clave, id string, items []domain.PlatoSolicitado) (domain.Reserva, error) {
	if err := domain.ValidarClave(clave); err != nil {
		return domain.Reserva{}, err
	}
	if !domain.IdentificadorValido(id) {
		return domain.Reserva{}, domain.ErrReservaInvalida
	}
	normalizados, err := domain.NormalizarItems(items, true)
	if err != nil {
		return domain.Reserva{}, err
	}
	return s.repo.AjustarReserva(ctx, clave, id, normalizados)
}
func (s *Reservas) Liberar(ctx context.Context, clave, id string) (domain.Reserva, error) {
	if err := domain.ValidarClave(clave); err != nil {
		return domain.Reserva{}, err
	}
	if !domain.IdentificadorValido(id) {
		return domain.Reserva{}, domain.ErrReservaInvalida
	}
	return s.repo.LiberarReserva(ctx, clave, id)
}
func (s *Reservas) Obtener(ctx context.Context, id string) (domain.Reserva, error) {
	if !domain.IdentificadorValido(id) {
		return domain.Reserva{}, domain.ErrReservaInvalida
	}
	return s.repo.ObtenerReserva(ctx, id)
}
