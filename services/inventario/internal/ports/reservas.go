package ports

import (
	"context"
	"ghostkitchen/inventario/internal/domain"
)

type Reservas interface {
	CrearReserva(context.Context, string, domain.SolicitudReserva) (domain.Reserva, error)
	AjustarReserva(context.Context, string, string, []domain.PlatoSolicitado) (domain.Reserva, error)
	LiberarReserva(context.Context, string, string) (domain.Reserva, error)
	ObtenerReserva(context.Context, string) (domain.Reserva, error)
}

type Consumos interface {
	Consumir(context.Context, string, string, string) (domain.Consumo, error)
	Devolver(context.Context, string, string) (domain.Consumo, error)
	ObtenerConsumo(context.Context, string) (domain.Consumo, error)
}
