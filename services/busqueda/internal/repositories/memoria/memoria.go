// Package memoria es un repositorio en memoria para los tests de servicios y handlers.
// No se usa en producción: reemplaza a Solr para probar las reglas sin levantar nada.
package memoria

import (
	"context"
	"sort"
	"strings"
	"sync"

	"ghostkitchen/busqueda/internal/domain"
	"ghostkitchen/busqueda/internal/geo"
)

// Repositorio guarda sedes y platos en mapas. Si Falla != nil, todas las lecturas devuelven ese error.
type Repositorio struct {
	mu     sync.RWMutex
	sedes  map[string]domain.Sede
	platos map[string]domain.Plato
	Falla  error
}

// Nuevo crea un repositorio vacío.
func Nuevo() *Repositorio {
	return &Repositorio{sedes: map[string]domain.Sede{}, platos: map[string]domain.Plato{}}
}

func (r *Repositorio) PrepararEsquema(context.Context) error { return nil }

func (r *Repositorio) GuardarSedes(_ context.Context, sedes []domain.Sede) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range sedes {
		r.sedes[s.ID] = s
	}
	return nil
}

func (r *Repositorio) GuardarPlatos(_ context.Context, platos []domain.Plato) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range platos {
		r.platos[p.ID] = p
	}
	return nil
}

func (r *Repositorio) ListarSedes(context.Context) ([]domain.Sede, error) {
	if r.Falla != nil {
		return nil, r.Falla
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []domain.Sede
	for _, s := range r.sedes {
		if s.Activa {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *Repositorio) ObtenerSede(_ context.Context, id string) (domain.Sede, bool, error) {
	if r.Falla != nil {
		return domain.Sede{}, false, r.Falla
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.sedes[id]
	return s, ok && s.Activa, nil
}

func (r *Repositorio) SedeMasCercana(ctx context.Context, punto domain.Ubicacion) (domain.Sede, bool, error) {
	sedes, err := r.ListarSedes(ctx) // ya vienen ordenadas por id
	if err != nil || len(sedes) == 0 {
		return domain.Sede{}, false, err
	}
	mejor := sedes[0]
	dMejor := geo.DistanciaKm(punto, mejor.Ubicacion)
	for _, s := range sedes[1:] {
		if d := geo.DistanciaKm(punto, s.Ubicacion); d < dMejor {
			mejor, dMejor = s, d
		}
	}
	return mejor, true, nil
}

func (r *Repositorio) BuscarPlatos(_ context.Context, f domain.FiltroPlatos) ([]domain.Plato, int, error) {
	if r.Falla != nil {
		return nil, 0, r.Falla
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	texto := strings.ToLower(strings.TrimSpace(f.Texto))
	var res []domain.Plato
	for _, p := range r.platos {
		switch {
		case !p.Activo, p.Sede.ID != f.SedeID,
			f.SoloDisponibles && !p.Disponible,
			f.Categoria != "" && p.Categoria != f.Categoria,
			f.RestauranteID != "" && p.Restaurante.ID != f.RestauranteID,
			f.PrecioMax > 0 && p.Precio > f.PrecioMax,
			f.Etiqueta != "" && !contiene(p.Etiquetas, f.Etiqueta),
			texto != "" && !strings.Contains(strings.ToLower(p.Nombre+" "+p.Descripcion+" "+p.Categoria+" "+p.Restaurante.Nombre), texto):
			continue
		}
		res = append(res, p)
	}
	sort.Slice(res, func(i, j int) bool {
		switch f.Orden {
		case domain.OrdenPrecioAsc:
			if res[i].Precio != res[j].Precio {
				return res[i].Precio < res[j].Precio
			}
		case domain.OrdenPrecioDesc:
			if res[i].Precio != res[j].Precio {
				return res[i].Precio > res[j].Precio
			}
		}
		return res[i].ID < res[j].ID
	})
	total := len(res)
	desde := (f.Pagina - 1) * f.TamPagina
	if desde > total {
		desde = total
	}
	hasta := desde + f.TamPagina
	if hasta > total {
		hasta = total
	}
	return res[desde:hasta], total, nil
}

func (r *Repositorio) ObtenerPlato(_ context.Context, id string) (domain.Plato, bool, error) {
	if r.Falla != nil {
		return domain.Plato{}, false, r.Falla
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.platos[id]
	return p, ok && p.Activo, nil
}

func (r *Repositorio) Ping(context.Context) error { return r.Falla }

func contiene(lista []string, v string) bool {
	for _, x := range lista {
		if x == v {
			return true
		}
	}
	return false
}
