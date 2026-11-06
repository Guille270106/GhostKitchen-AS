// Package repositories define cómo Catálogo lee y escribe sus datos.
// La implementación real usa MongoDB (subpaquete mongo); los tests usan una en memoria.
//
// Es el patrón Repository (ADR-002): services depende de esta interfaz y no de MongoDB, y
// main le pasa la implementación (inyección de dependencias). Así las reglas se prueban sin
// base, y la caché de Memcached (ADR-007) se podrá agregar como un decorator que implementa
// la misma interfaz, sin que el servicio se entere.
package repositories

import (
	"context"

	"ghostkitchen/catalogo/internal/domain"
)

// Repositorio es el acceso a cocinas y platos. Toda falla de la base se devuelve
// envuelta en domain.ErrNoDisponible.
type Repositorio interface {
	// ListarCocinas devuelve las cocinas ordenadas por id. Con soloActivas=false incluye las desactivadas.
	ListarCocinas(ctx context.Context, soloActivas bool) ([]domain.Cocina, error)
	// ObtenerCocina devuelve la cocina por id (ok=false si no existe).
	ObtenerCocina(ctx context.Context, id string) (domain.Cocina, bool, error)
	// ListarPlatosDeCocina devuelve los platos activos de una cocina, ordenados por id.
	ListarPlatosDeCocina(ctx context.Context, cocinaID string) ([]domain.Plato, error)
	// ObtenerPlato devuelve el plato por id (ok=false si no existe).
	ObtenerPlato(ctx context.Context, id string) (domain.Plato, bool, error)
	// PaginaPlatos devuelve todos los platos (activos y no) de a páginas, ordenados por id.
	PaginaPlatos(ctx context.Context, page, pageSize int) ([]domain.Plato, int, error)
	// CantidadCocinas cuenta las cocinas guardadas. Hoy no la usa el código del servicio:
	// la carga inicial ya no la necesita porque InsertarIniciales no pisa lo existente.
	CantidadCocinas(ctx context.Context) (int, error)
	// InsertarIniciales guarda cocinas y platos nuevos. No pisa documentos existentes.
	InsertarIniciales(ctx context.Context, cocinas []domain.Cocina, platos []domain.Plato) error
	// Ping verifica que la base responde.
	Ping(ctx context.Context) error
}
