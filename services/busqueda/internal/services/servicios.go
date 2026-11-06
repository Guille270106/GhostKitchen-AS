// Package services tiene las reglas de la capacidad publicada:
// sedes, cocina más cercana, búsqueda de platos y proximidad.
//
// Es la capa del medio (ADR-002): no conoce HTTP ni Solr. Recibe un repositories.Lector
// (una interfaz) por el constructor, y main le pasa el de Solr; los tests, uno en memoria.
package services

import (
	"context"
	"math"

	"ghostkitchen/busqueda/internal/domain"
	"ghostkitchen/busqueda/internal/geo"
	"ghostkitchen/busqueda/internal/repositories"
)

// CoberturaKm es el radio que cubre una cocina (RN24).
const CoberturaKm = 25.0

// Busqueda reúne las operaciones que usa la API publicada.
type Busqueda struct {
	repo repositories.Lector
}

// Nueva crea el servicio.
func Nueva(repo repositories.Lector) *Busqueda { return &Busqueda{repo: repo} }

// ListarSedes devuelve las sedes activas.
func (b *Busqueda) ListarSedes(ctx context.Context) ([]domain.Sede, error) {
	sedes, err := b.repo.ListarSedes(ctx)
	if sedes == nil && err == nil { // para que el JSON sea [] y no null
		sedes = []domain.Sede{}
	}
	return sedes, err
}

// SedeCercana devuelve la sede activa más cercana y su distancia redondeada a 0,1 km.
// Si ninguna está a 25 km o menos, devuelve domain.ErrSinCobertura (RN24).
func (b *Busqueda) SedeCercana(ctx context.Context, punto domain.Ubicacion) (domain.Sede, float64, error) {
	sede, ok, err := b.repo.SedeMasCercana(ctx, punto)
	if err != nil {
		return domain.Sede{}, 0, err
	}
	if !ok {
		return domain.Sede{}, 0, domain.ErrSinCobertura
	}
	d := geo.DistanciaKm(punto, sede.Ubicacion)
	if d > CoberturaKm {
		return domain.Sede{}, 0, domain.ErrSinCobertura
	}
	return sede, geo.Redondear(d, 1), nil
}

// ResultadoPlatos es una página de la búsqueda de platos.
type ResultadoPlatos struct {
	Sede        domain.Referencia `json:"sede"`
	Items       []domain.Plato    `json:"items"`
	Pagina      int               `json:"page"`
	TamPagina   int               `json:"pageSize"`
	Total       int               `json:"total"`
	TotalPagina int               `json:"totalPages"`
}

// BuscarPlatos busca platos de una sede activa. Si la sede no existe o no está activa,
// devuelve domain.ErrSedeNoEncontrada.
//
// Se busca primero la sede para separar dos respuestas que el consumidor tiene que poder
// distinguir: "esa sede no existe" (404) y "esa sede no tiene platos con ese filtro"
// (200 con la lista vacía).
func (b *Busqueda) BuscarPlatos(ctx context.Context, f domain.FiltroPlatos) (ResultadoPlatos, error) {
	sede, ok, err := b.repo.ObtenerSede(ctx, f.SedeID)
	if err != nil {
		return ResultadoPlatos{}, err
	}
	if !ok {
		return ResultadoPlatos{}, domain.ErrSedeNoEncontrada
	}
	platos, total, err := b.repo.BuscarPlatos(ctx, f)
	if err != nil {
		return ResultadoPlatos{}, err
	}
	if platos == nil {
		platos = []domain.Plato{}
	}
	return ResultadoPlatos{
		Sede:        sede.Ref(),
		Items:       platos,
		Pagina:      f.Pagina,
		TamPagina:   f.TamPagina,
		Total:       total,
		TotalPagina: int(math.Ceil(float64(total) / float64(f.TamPagina))),
	}, nil
}

// ObtenerPlato devuelve un plato activo o domain.ErrPlatoNoEncontrado.
func (b *Busqueda) ObtenerPlato(ctx context.Context, id string) (domain.Plato, error) {
	p, ok, err := b.repo.ObtenerPlato(ctx, id)
	if err != nil {
		return domain.Plato{}, err
	}
	if !ok {
		return domain.Plato{}, domain.ErrPlatoNoEncontrado
	}
	return p, nil
}
