// Package services tiene las reglas de lectura de Catálogo.
//
// Es la capa del medio (ADR-002): no conoce HTTP ni MongoDB. Recibe un
// repositories.Repositorio (una interfaz) por el constructor; main le pasa el de MongoDB
// y los tests, uno en memoria. Acá se decide qué se publica: solo lo activo (RN19).
package services

import (
	"context"
	"math"

	"ghostkitchen/catalogo/internal/domain"
	"ghostkitchen/catalogo/internal/repositories"
)

// MaxTamPagina es el máximo de platos por página en la lectura interna. Limita cuánto puede
// pedir de una vez quien llama, para que una sola solicitud no cargue a MongoDB.
const MaxTamPagina = 500

// Catalogo reúne las lecturas de cocinas y platos.
type Catalogo struct {
	repo repositories.Repositorio
}

// Nuevo crea el servicio.
func Nuevo(repo repositories.Repositorio) *Catalogo { return &Catalogo{repo: repo} }

// ---------- Lecturas públicas (frontend) ----------

// ListarCocinas devuelve las cocinas activas, cada una solo con sus restaurantes activos.
func (s *Catalogo) ListarCocinas(ctx context.Context) ([]domain.Cocina, error) {
	cocinas, err := s.repo.ListarCocinas(ctx, true)
	if err != nil {
		return nil, err
	}
	for i := range cocinas {
		cocinas[i].Restaurantes = restaurantesActivos(cocinas[i].Restaurantes)
	}
	return cocinas, nil
}

// ObtenerCocina devuelve una cocina activa con sus restaurantes activos.
func (s *Catalogo) ObtenerCocina(ctx context.Context, id string) (domain.Cocina, error) {
	c, ok, err := s.repo.ObtenerCocina(ctx, id)
	if err != nil {
		return domain.Cocina{}, err
	}
	if !ok || !c.Activa {
		return domain.Cocina{}, domain.ErrCocinaNoEncontrada
	}
	c.Restaurantes = restaurantesActivos(c.Restaurantes)
	return c, nil
}

// Menu devuelve los platos activos de una cocina activa, solo de sus restaurantes activos (RN19).
func (s *Catalogo) Menu(ctx context.Context, cocinaID string) (domain.Cocina, []domain.Plato, error) {
	c, err := s.ObtenerCocina(ctx, cocinaID)
	if err != nil {
		return domain.Cocina{}, nil, err
	}
	activos := map[string]bool{}
	for _, r := range c.Restaurantes {
		activos[r.ID] = true
	}
	platos, err := s.repo.ListarPlatosDeCocina(ctx, cocinaID)
	if err != nil {
		return domain.Cocina{}, nil, err
	}
	menu := make([]domain.Plato, 0, len(platos))
	for _, p := range platos {
		if activos[p.RestauranteID] {
			menu = append(menu, p)
		}
	}
	return c, menu, nil
}

// ObtenerPlato devuelve un plato activo.
func (s *Catalogo) ObtenerPlato(ctx context.Context, id string) (domain.Plato, error) {
	p, ok, err := s.repo.ObtenerPlato(ctx, id)
	if err != nil {
		return domain.Plato{}, err
	}
	if !ok || !p.Activo {
		return domain.Plato{}, domain.ErrPlatoNoEncontrado
	}
	return p, nil
}

// ---------- Lecturas internas (reconstrucción de Búsqueda) ----------

// TodasLasCocinas devuelve todas las cocinas, activas o no, con todos sus restaurantes.
// Búsqueda las necesita para marcar como inactivo lo que se desactivó.
func (s *Catalogo) TodasLasCocinas(ctx context.Context) ([]domain.Cocina, error) {
	return s.repo.ListarCocinas(ctx, false)
}

// PaginaPlatos devuelve todos los platos, activos o no, de a páginas ordenadas por id.
func (s *Catalogo) PaginaPlatos(ctx context.Context, page, pageSize int) (domain.Pagina[domain.Plato], error) {
	platos, total, err := s.repo.PaginaPlatos(ctx, page, pageSize)
	if err != nil {
		return domain.Pagina[domain.Plato]{}, err
	}
	return domain.Pagina[domain.Plato]{
		Items: platos, Page: page, PageSize: pageSize, Total: total,
		TotalPages: int(math.Ceil(float64(total) / float64(pageSize))),
	}, nil
}

func restaurantesActivos(rs []domain.Restaurante) []domain.Restaurante {
	out := make([]domain.Restaurante, 0, len(rs))
	for _, r := range rs {
		if r.Activo {
			out = append(out, r)
		}
	}
	return out
}
