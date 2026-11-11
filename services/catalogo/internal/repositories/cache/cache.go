// Package cache es la caché de Catálogo (ADR-007): un decorator del repositorio que guarda en
// Memcached las lecturas públicas más repetidas.
//
// Patrón cache-aside: el servicio pide los datos igual que siempre; el decorator busca en
// Memcached y, si no están, va a MongoDB y los guarda. Implementa las mismas interfaces que el
// repositorio de MongoDB, así que el servicio no se entera de que hay caché (ADR-002).
//
// Invalidación por generación: cada escritura suma 1 a catalogo:v1:gen, y todas las claves
// llevan la generación en el nombre. Después de un cambio nadie vuelve a leer las claves viejas
// (vencen solas por TTL). A diferencia de "borrar la clave", una lectura que empezó antes del
// cambio y escribe después lo hace en una clave que ya nadie consulta: no reinserta el dato viejo.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"ghostkitchen/catalogo/internal/domain"
	"ghostkitchen/catalogo/internal/repositories"
)

// TTL de cada clave (ADR-007): 60 s, con ±10 % al azar para que no venzan todas juntas.
const TTL = 60 * time.Second

// usos cuenta en /metrics cada lectura de la caché, por tipo de clave (cocinas, cocina,
// menu, plato) y resultado: acierto (estaba), fallo (no estaba: se leyó de MongoDB) o error
// (Memcached no respondió). Es lo que mide si la caché sirve (ADR-007, punto 6).
var usos = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "catalogo_cache_total",
	Help: "Lecturas de la caché de Catálogo, por tipo de clave y resultado (acierto, fallo, error).",
}, []string{"tipo", "resultado"})

// Estadisticas cuenta aciertos, fallos y errores de la caché de este decorator (lo mismo que
// catalogo_cache_total, pero por instancia: lo usan los tests).
type Estadisticas struct {
	Aciertos atomic.Int64 // estaba en la caché
	Fallos   atomic.Int64 // no estaba: se leyó de MongoDB
	Errores  atomic.Int64 // Memcached no respondió: se leyó de MongoDB
}

// Repositorio es el decorator. Las lecturas que no se cachean y las del outbox pasan directo.
type Repositorio struct {
	repositories.Repositorio
	esc    repositories.Escritor
	escP   repositories.EscritorPlatos
	alm    Almacen
	log    *slog.Logger
	vuelos vuelos
	Stats  Estadisticas
}

// Nuevo arma el decorator sobre el repositorio real.
func Nuevo(base repositories.Repositorio, esc repositories.Escritor, escP repositories.EscritorPlatos, alm Almacen, log *slog.Logger) *Repositorio {
	return &Repositorio{Repositorio: base, esc: esc, escP: escP, alm: alm, log: log, vuelos: vuelos{enCurso: map[string]*vuelo{}}}
}

// ---------- Lecturas cacheadas (las públicas del frontend) ----------

// ListarCocinas cachea solo la lista de activas (la pública). La de todas (Búsqueda y admin)
// va directo a MongoDB.
func (r *Repositorio) ListarCocinas(ctx context.Context, soloActivas bool) ([]domain.Cocina, error) {
	if !soloActivas {
		return r.Repositorio.ListarCocinas(ctx, false)
	}
	return leer(r, "cocinas", "cocinas", func() ([]domain.Cocina, error) { return r.Repositorio.ListarCocinas(ctx, true) })
}

type cocinaGuardada struct {
	Cocina domain.Cocina `json:"cocina"`
	Existe bool          `json:"existe"`
}

// ObtenerCocina cachea una cocina por id (la usan el detalle, el menú y el detalle de un plato).
func (r *Repositorio) ObtenerCocina(ctx context.Context, id string) (domain.Cocina, bool, error) {
	g, err := leer(r, "cocina", "cocina:"+id, func() (cocinaGuardada, error) {
		c, ok, err := r.Repositorio.ObtenerCocina(ctx, id)
		return cocinaGuardada{Cocina: c, Existe: ok}, err
	})
	return g.Cocina, g.Existe, err
}

// ListarPlatosDeCocina cachea el menú de una cocina.
func (r *Repositorio) ListarPlatosDeCocina(ctx context.Context, cocinaID string) ([]domain.Plato, error) {
	return leer(r, "menu", "menu:"+cocinaID, func() ([]domain.Plato, error) { return r.Repositorio.ListarPlatosDeCocina(ctx, cocinaID) })
}

type platoGuardado struct {
	Plato  domain.Plato `json:"plato"`
	Existe bool         `json:"existe"`
}

// ObtenerPlato cachea el detalle de un plato.
func (r *Repositorio) ObtenerPlato(ctx context.Context, id string) (domain.Plato, bool, error) {
	g, err := leer(r, "plato", "plato:"+id, func() (platoGuardado, error) {
		p, ok, err := r.Repositorio.ObtenerPlato(ctx, id)
		return platoGuardado{Plato: p, Existe: ok}, err
	})
	return g.Plato, g.Existe, err
}

// leer es el cache-aside. Si Memcached falla, se lee de MongoDB y no se guarda nada: la caché
// nunca hace fallar una lectura que MongoDB puede contestar.
func leer[T any](r *Repositorio, tipo, clave string, cargar func() (T, error)) (T, error) {
	gen, err := r.alm.Generacion()
	if err != nil {
		r.fallaCache(tipo, err)
		return cargar()
	}
	k := fmt.Sprintf("catalogo:v1:%s:g%d", clave, gen)

	if b, err := r.alm.Leer(k); err == nil {
		var v T
		if json.Unmarshal(b, &v) == nil {
			r.Stats.Aciertos.Add(1)
			usos.WithLabelValues(tipo, "acierto").Inc()
			return v, nil
		}
	} else if !errors.Is(err, ErrNoEsta) {
		r.fallaCache(tipo, err)
		return cargar()
	}

	// Miss: una sola solicitud por clave va a MongoDB; las que llegan mientras tanto esperan
	// su resultado (contra el "stampede" cuando vence una clave muy pedida).
	r.Stats.Fallos.Add(1)
	usos.WithLabelValues(tipo, "fallo").Inc()
	v, err, primera := r.vuelos.hacer(k, func() (any, error) { return cargar() })
	if err != nil {
		var cero T
		return cero, err
	}
	if primera {
		if b, err := json.Marshal(v); err == nil {
			if err := r.alm.Guardar(k, b, ttlConMargen()); err != nil {
				r.fallaCache(tipo, err)
			}
		}
	}
	return v.(T), nil
}

func ttlConMargen() time.Duration {
	return TTL + time.Duration((rand.Float64()*0.2-0.1)*float64(TTL))
}

func (r *Repositorio) fallaCache(tipo string, err error) {
	r.Stats.Errores.Add(1)
	usos.WithLabelValues(tipo, "error").Inc()
	r.log.Warn("Memcached no respondió; se lee de MongoDB", "clave", tipo, "motivo", err.Error())
}

// ---------- Escrituras: se guardan y después se invalida ----------

// invalidar suma una generación. Si Memcached no responde, el dato viejo puede verse hasta que
// venza su TTL (60 s): se registra y se sigue, porque el cambio ya quedó guardado en MongoDB.
func (r *Repositorio) invalidar(err error) error {
	if err != nil {
		return err
	}
	if e := r.alm.NuevaGeneracion(); e != nil {
		r.fallaCache("generacion", e)
	}
	return nil
}

func (r *Repositorio) CrearCocina(ctx context.Context, c domain.Cocina, ev domain.Evento) error {
	return r.invalidar(r.esc.CrearCocina(ctx, c, ev))
}

func (r *Repositorio) ReemplazarCocina(ctx context.Context, c domain.Cocina, versionAnterior int64, ev domain.Evento) error {
	return r.invalidar(r.esc.ReemplazarCocina(ctx, c, versionAnterior, ev))
}

func (r *Repositorio) ListarPlatosDeCocinaTodos(ctx context.Context, cocinaID string) ([]domain.Plato, error) {
	return r.escP.ListarPlatosDeCocinaTodos(ctx, cocinaID)
}

func (r *Repositorio) CrearPlato(ctx context.Context, p domain.Plato, ev domain.Evento) error {
	return r.invalidar(r.escP.CrearPlato(ctx, p, ev))
}

func (r *Repositorio) ReemplazarPlato(ctx context.Context, p domain.Plato, versionAnterior int64, ev domain.Evento) error {
	return r.invalidar(r.escP.ReemplazarPlato(ctx, p, versionAnterior, ev))
}

// ---------- Una sola carga por clave a la vez (singleflight) ----------

type vuelo struct {
	listo chan struct{}
	v     any
	err   error
}

type vuelos struct {
	mu      sync.Mutex
	enCurso map[string]*vuelo
}

// hacer corre f una sola vez por clave a la vez. Las demás llamadas con la misma clave esperan
// y reciben el mismo resultado. primera dice si esta llamada fue la que cargó.
func (g *vuelos) hacer(clave string, f func() (any, error)) (any, error, bool) {
	g.mu.Lock()
	if v, ok := g.enCurso[clave]; ok {
		g.mu.Unlock()
		<-v.listo
		return v.v, v.err, false
	}
	v := &vuelo{listo: make(chan struct{})}
	g.enCurso[clave] = v
	g.mu.Unlock()

	v.v, v.err = f()
	close(v.listo)

	g.mu.Lock()
	delete(g.enCurso, clave)
	g.mu.Unlock()
	return v.v, v.err, true
}
