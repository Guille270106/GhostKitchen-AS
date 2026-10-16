// Package memoria es un repositorio en memoria para los tests. No se usa en producción.
package memoria

import (
	"context"
	"sort"
	"sync"

	"ghostkitchen/catalogo/internal/domain"
)

// Repositorio guarda cocinas y platos en mapas. Si Falla != nil, todas las operaciones devuelven ese error.
type Repositorio struct {
	mu      sync.RWMutex
	cocinas map[string]domain.Cocina
	platos  map[string]domain.Plato
	Falla   error
}

// Nuevo crea un repositorio vacío.
func Nuevo() *Repositorio {
	return &Repositorio{cocinas: map[string]domain.Cocina{}, platos: map[string]domain.Plato{}}
}

func (r *Repositorio) ListarCocinas(_ context.Context, soloActivas bool) ([]domain.Cocina, error) {
	if r.Falla != nil {
		return nil, r.Falla
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []domain.Cocina{}
	for _, c := range r.cocinas {
		if !soloActivas || c.Activa {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *Repositorio) ObtenerCocina(_ context.Context, id string) (domain.Cocina, bool, error) {
	if r.Falla != nil {
		return domain.Cocina{}, false, r.Falla
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.cocinas[id]
	return c, ok, nil
}

func (r *Repositorio) ListarPlatosDeCocina(_ context.Context, cocinaID string) ([]domain.Plato, error) {
	if r.Falla != nil {
		return nil, r.Falla
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []domain.Plato{}
	for _, p := range r.platos {
		if p.CocinaID == cocinaID && p.Activo {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *Repositorio) ObtenerPlato(_ context.Context, id string) (domain.Plato, bool, error) {
	if r.Falla != nil {
		return domain.Plato{}, false, r.Falla
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.platos[id]
	return p, ok, nil
}

func (r *Repositorio) PaginaPlatos(_ context.Context, page, pageSize int) ([]domain.Plato, int, error) {
	if r.Falla != nil {
		return nil, 0, r.Falla
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	todos := make([]domain.Plato, 0, len(r.platos))
	for _, p := range r.platos {
		todos = append(todos, p)
	}
	sort.Slice(todos, func(i, j int) bool { return todos[i].ID < todos[j].ID })
	desde := min((page-1)*pageSize, len(todos))
	hasta := min(desde+pageSize, len(todos))
	return todos[desde:hasta], len(todos), nil
}

func (r *Repositorio) CantidadCocinas(context.Context) (int, error) {
	if r.Falla != nil {
		return 0, r.Falla
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.cocinas), nil
}

func (r *Repositorio) InsertarIniciales(_ context.Context, cocinas []domain.Cocina, platos []domain.Plato) error {
	if r.Falla != nil {
		return r.Falla
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range cocinas {
		if _, ya := r.cocinas[c.ID]; !ya {
			r.cocinas[c.ID] = c
		}
	}
	for _, p := range platos {
		if _, ya := r.platos[p.ID]; !ya {
			r.platos[p.ID] = p
		}
	}
	return nil
}

func (r *Repositorio) Ping(context.Context) error { return r.Falla }

// Guardar agrega o reemplaza una cocina o un plato (solo para preparar tests).
func (r *Repositorio) Guardar(c *domain.Cocina, p *domain.Plato) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c != nil {
		r.cocinas[c.ID] = *c
	}
	if p != nil {
		r.platos[p.ID] = *p
	}
}
