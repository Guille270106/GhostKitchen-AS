// Package memoria es un repositorio en memoria para los tests. No se usa en producción.
package memoria

import (
	"context"
	"sort"
	"sync"
	"time"

	"ghostkitchen/catalogo/internal/domain"
)

// Repositorio guarda cocinas y platos en mapas. Si Falla != nil, todas las operaciones devuelven ese error.
type Repositorio struct {
	mu      sync.RWMutex
	cocinas map[string]domain.Cocina
	platos  map[string]domain.Plato
	Falla   error
	// AntesDeReemplazar simula una escritura concurrente (ver ReemplazarCocina).
	AntesDeReemplazar func()
	outbox            []registro
}

// registro es un evento del outbox en memoria.
type registro struct {
	ev        domain.Evento
	sobre     []byte
	publicado bool
}

// Nuevo crea un repositorio vacío.
func Nuevo() *Repositorio {
	return &Repositorio{cocinas: map[string]domain.Cocina{}, platos: map[string]domain.Plato{}}
}

// ListarCocinas devuelve las cocinas ordenadas por id (solo las activas si soloActivas).
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

// ObtenerCocina devuelve la cocina por id, activa o no.
func (r *Repositorio) ObtenerCocina(_ context.Context, id string) (domain.Cocina, bool, error) {
	if r.Falla != nil {
		return domain.Cocina{}, false, r.Falla
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.cocinas[id]
	return copiaCocina(c), ok, nil
}

// ListarPlatosDeCocina devuelve los platos activos de una cocina, ordenados por id.
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

// ObtenerPlato devuelve el plato por id, activo o no.
func (r *Repositorio) ObtenerPlato(_ context.Context, id string) (domain.Plato, bool, error) {
	if r.Falla != nil {
		return domain.Plato{}, false, r.Falla
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.platos[id]
	return copiaPlato(p), ok, nil
}

// PaginaPlatos devuelve todos los platos de a páginas, ordenados por id.
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

// CantidadCocinas cuenta las cocinas guardadas.
func (r *Repositorio) CantidadCocinas(context.Context) (int, error) {
	if r.Falla != nil {
		return 0, r.Falla
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.cocinas), nil
}

// InsertarIniciales agrega solo lo que no existe, igual que la versión de MongoDB.
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

// Ping devuelve Falla: así un test simula que la base no responde.
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

// CrearCocina guarda una cocina nueva, igual que la versión de MongoDB.
func (r *Repositorio) CrearCocina(_ context.Context, c domain.Cocina, ev domain.Evento) error {
	if r.Falla != nil {
		return r.Falla
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ya := r.cocinas[c.ID]; ya {
		return domain.ErrYaExiste
	}
	r.cocinas[c.ID] = copiaCocina(c)
	r.anotar(ev)
	return nil
}

// ReemplazarCocina guarda la cocina solo si su versión es versionAnterior. AntesDeReemplazar,
// si no es nil, se llama justo antes de comparar: un test lo usa para simular que otra
// solicitud cambió la cocina en el medio.
func (r *Repositorio) ReemplazarCocina(_ context.Context, c domain.Cocina, versionAnterior int64, ev domain.Evento) error {
	if r.Falla != nil {
		return r.Falla
	}
	r.mu.Lock()
	f := r.AntesDeReemplazar
	r.AntesDeReemplazar = nil
	r.mu.Unlock()
	if f != nil {
		f()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	actual, ok := r.cocinas[c.ID]
	if !ok || actual.Version != versionAnterior {
		return domain.ErrConflicto
	}
	r.cocinas[c.ID] = copiaCocina(c)
	r.anotar(ev)
	return nil
}

// copiaCocina copia también la lista de restaurantes. Sin esto, dos solicitudes que leen la
// misma cocina compartirían la lista y una podría cambiar la de la otra; con MongoDB no pasa,
// porque cada lectura arma su propia copia.
func copiaCocina(c domain.Cocina) domain.Cocina {
	if c.Restaurantes != nil {
		c.Restaurantes = append([]domain.Restaurante(nil), c.Restaurantes...)
	}
	return c
}

// ListarPlatosDeCocinaTodos devuelve todos los platos de una cocina, ordenados por id.
func (r *Repositorio) ListarPlatosDeCocinaTodos(_ context.Context, cocinaID string) ([]domain.Plato, error) {
	if r.Falla != nil {
		return nil, r.Falla
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []domain.Plato{}
	for _, p := range r.platos {
		if p.CocinaID == cocinaID {
			out = append(out, copiaPlato(p))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// CrearPlato guarda un plato nuevo, igual que la versión de MongoDB.
func (r *Repositorio) CrearPlato(_ context.Context, p domain.Plato, ev domain.Evento) error {
	if r.Falla != nil {
		return r.Falla
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ya := r.platos[p.ID]; ya {
		return domain.ErrYaExiste
	}
	r.platos[p.ID] = copiaPlato(p)
	r.anotar(ev)
	return nil
}

// ReemplazarPlato guarda el plato solo si su versión es versionAnterior.
func (r *Repositorio) ReemplazarPlato(_ context.Context, p domain.Plato, versionAnterior int64, ev domain.Evento) error {
	if r.Falla != nil {
		return r.Falla
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	actual, ok := r.platos[p.ID]
	if !ok || actual.Version != versionAnterior {
		return domain.ErrConflicto
	}
	r.platos[p.ID] = copiaPlato(p)
	r.anotar(ev)
	return nil
}

func copiaPlato(p domain.Plato) domain.Plato {
	if p.Etiquetas != nil {
		p.Etiquetas = append([]string(nil), p.Etiquetas...)
	}
	return p
}

// anotar guarda el evento en el outbox, con el cambio ya bajo el mismo candado: igual que la
// transacción de MongoDB, o quedan los dos o ninguno.
func (r *Repositorio) anotar(ev domain.Evento) {
	sobre, _ := ev.Sobre()
	r.outbox = append(r.outbox, registro{ev: ev, sobre: sobre})
}

// Eventos devuelve todos los eventos guardados, publicados o no (para los tests).
func (r *Repositorio) Eventos() []domain.Evento {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Evento, len(r.outbox))
	for i, reg := range r.outbox {
		out[i] = reg.ev
	}
	return out
}

// Pendientes devuelve los eventos sin publicar, en el orden en que se guardaron.
func (r *Repositorio) Pendientes(_ context.Context, limite int) ([]domain.Pendiente, error) {
	if r.Falla != nil {
		return nil, r.Falla
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []domain.Pendiente{}
	for _, reg := range r.outbox {
		if !reg.publicado && len(out) < limite {
			out = append(out, domain.Pendiente{EventoID: reg.ev.EventoID, Tipo: reg.ev.Tipo, Sobre: reg.sobre})
		}
	}
	return out, nil
}

// MarcarPublicado anota el evento como publicado.
func (r *Repositorio) MarcarPublicado(_ context.Context, eventoID string, _ time.Time) error {
	if r.Falla != nil {
		return r.Falla
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.outbox {
		if r.outbox[i].ev.EventoID == eventoID {
			r.outbox[i].publicado = true
		}
	}
	return nil
}
