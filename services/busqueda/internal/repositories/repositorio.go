// Package repositories define cómo Búsqueda lee y escribe su índice.
// La implementación real usa Solr (subpaquete solr); los tests usan una en memoria.
//
// Es el patrón Repository (ADR-002): services depende de estas interfaces y no de Solr.
// main elige la implementación y se la pasa (inyección de dependencias). Así las reglas se
// prueban sin levantar Solr, y cambiar de motor de búsqueda no toca los servicios.
//
// Hay dos interfaces chicas en lugar de una grande: quien responde consultas (services)
// solo puede leer, y quien carga datos (indexer, seed) solo puede escribir.
package repositories

import (
	"context"

	"ghostkitchen/busqueda/internal/domain"
)

// Lector es lo que necesitan los servicios para responder consultas.
// Todas las operaciones devuelven domain.ErrNoDisponible si el índice no responde.
type Lector interface {
	// ListarSedes devuelve las sedes activas ordenadas por id.
	ListarSedes(ctx context.Context) ([]domain.Sede, error)
	// ObtenerSede devuelve la sede si existe y está activa (ok=false si no).
	ObtenerSede(ctx context.Context, id string) (sede domain.Sede, ok bool, err error)
	// SedeMasCercana devuelve la sede activa más cercana al punto; empates por id.
	// ok=false si no hay ninguna sede activa.
	SedeMasCercana(ctx context.Context, punto domain.Ubicacion) (sede domain.Sede, ok bool, err error)
	// BuscarPlatos devuelve una página de platos y el total de resultados.
	BuscarPlatos(ctx context.Context, f domain.FiltroPlatos) (platos []domain.Plato, total int, err error)
	// ObtenerPlato devuelve el plato si existe y está activo (ok=false si no).
	ObtenerPlato(ctx context.Context, id string) (plato domain.Plato, ok bool, err error)
	// Ping verifica que el índice responde.
	Ping(ctx context.Context) error
}

// Indexador carga datos en el índice. Lo usan la reconstrucción desde Catálogo (indexer) y
// la carga inicial (seed); más adelante, también el lector de mensajes.
type Indexador interface {
	// PrepararEsquema crea los campos que falten. Es idempotente.
	PrepararEsquema(ctx context.Context) error
	// GuardarSedes agrega o reemplaza sedes por id.
	GuardarSedes(ctx context.Context, sedes []domain.Sede) error
	// GuardarPlatos agrega o reemplaza platos por id.
	GuardarPlatos(ctx context.Context, platos []domain.Plato) error
}
