package ports

import (
	"context"
	"time"

	"ghostkitchen/pedidos/internal/domain"
)

// CocinaLocalidad es una cocina de Catálogo con su localidad.
type CocinaLocalidad struct {
	ID     string
	Nombre string
}

// Cocinas dice qué cocinas hay en una localidad (lo sabe Catálogo). Así un admin solo ve las
// métricas de las cocinas de su localidad.
type Cocinas interface {
	CocinasDeLocalidad(ctx context.Context, localidad string) ([]CocinaLocalidad, error)
}

// MetricasRepo calcula las métricas con los pedidos guardados. Solo lee.
type MetricasRepo interface {
	// PedidosPorCocina cuenta los pedidos no cancelados de cada cocina desde una fecha.
	PedidosPorCocina(ctx context.Context, cocinaIDs []string, desde time.Time) (map[string]int64, error)
	// MetricasDeCocina arma el detalle de una cocina desde una fecha. Devuelve también los
	// pedidos de cada día (fecha AAAA-MM-DD en hora de Córdoba).
	MetricasDeCocina(ctx context.Context, cocinaID string, desde time.Time) (domain.MetricasCocina, map[string]int64, error)
}
