package cache

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"ghostkitchen/catalogo/internal/domain"
	"ghostkitchen/catalogo/internal/repositories/memoria"
)

// almacenFalso es una caché en memoria que puede simular que Memcached se cae.
type almacenFalso struct {
	mu    sync.Mutex
	datos map[string][]byte
	gen   uint64
	caido bool
}

func (a *almacenFalso) Leer(k string) ([]byte, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.caido {
		return nil, errors.New("memcached: timeout")
	}
	b, ok := a.datos[k]
	if !ok {
		return nil, ErrNoEsta
	}
	return b, nil
}
func (a *almacenFalso) Guardar(k string, v []byte, _ time.Duration) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.caido {
		return errors.New("memcached: timeout")
	}
	a.datos[k] = v
	return nil
}
func (a *almacenFalso) Generacion() (uint64, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.caido {
		return 0, errors.New("memcached: timeout")
	}
	return a.gen, nil
}
func (a *almacenFalso) NuevaGeneracion() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.caido {
		return errors.New("memcached: timeout")
	}
	a.gen++
	return nil
}

// contador cuenta cuántas veces se leyó de "MongoDB" (el repositorio en memoria).
type contador struct {
	*memoria.Repositorio
	lecturas atomic.Int64
	demora   time.Duration
}

func (c *contador) ListarPlatosDeCocina(ctx context.Context, id string) ([]domain.Plato, error) {
	c.lecturas.Add(1)
	time.Sleep(c.demora)
	return c.Repositorio.ListarPlatosDeCocina(ctx, id)
}

func (c *contador) ObtenerPlato(ctx context.Context, id string) (domain.Plato, bool, error) {
	c.lecturas.Add(1)
	return c.Repositorio.ObtenerPlato(ctx, id)
}

func entorno(t *testing.T) (*Repositorio, *contador, *almacenFalso) {
	t.Helper()
	base := memoria.Nuevo()
	base.Guardar(&domain.Cocina{ID: "nc", Nombre: "NC", Activa: true, Version: 1}, &domain.Plato{ID: "p1", CocinaID: "nc", Nombre: "Mega", Precio: 100, Activo: true, Version: 1})
	cont := &contador{Repositorio: base}
	alm := &almacenFalso{datos: map[string][]byte{}, gen: 1}
	return Nuevo(cont, base, base, alm, slog.New(slog.NewJSONHandler(io.Discard, nil))), cont, alm
}

func TestDevuelveLoMismoYLaSegundaVezNoVaAMongo(t *testing.T) {
	r, cont, _ := entorno(t)
	a, _ := r.ListarPlatosDeCocina(context.Background(), "nc")
	b, _ := r.ListarPlatosDeCocina(context.Background(), "nc")
	if len(a) != 1 || len(b) != 1 || a[0].ID != b[0].ID || a[0].Precio != b[0].Precio {
		t.Fatalf("tiene que devolver lo mismo: %v %v", a, b)
	}
	if cont.lecturas.Load() != 1 || r.Stats.Aciertos.Load() != 1 || r.Stats.Fallos.Load() != 1 {
		t.Fatalf("lecturas a MongoDB %d, aciertos %d, fallos %d", cont.lecturas.Load(), r.Stats.Aciertos.Load(), r.Stats.Fallos.Load())
	}
}

func TestDespuesDeEscribirNoSeSirveElDatoViejo(t *testing.T) {
	r, _, _ := entorno(t)
	ctx := context.Background()
	p, _, _ := r.ObtenerPlato(ctx, "p1") // queda en caché con precio 100
	nuevo := p
	nuevo.Precio, nuevo.Version = 999, 2
	if err := r.ReemplazarPlato(ctx, nuevo, 1, domain.EventoPlato(domain.PlatoActualizado, nuevo, "", time.Now())); err != nil {
		t.Fatal(err)
	}
	if p, _, _ := r.ObtenerPlato(ctx, "p1"); p.Precio != 999 {
		t.Fatalf("después de escribir tiene que verse el precio nuevo, no %d", p.Precio)
	}
}

// Una lectura que empezó antes del cambio y termina después escribe en la clave de la
// generación vieja: la siguiente lectura ya busca en la nueva y ve el dato correcto.
func TestLecturaConcurrenteNoReinsertaElDatoViejo(t *testing.T) {
	r, cont, alm := entorno(t)
	ctx := context.Background()
	cont.demora = 50 * time.Millisecond
	listo := make(chan struct{})
	go func() { _, _ = r.ListarPlatosDeCocina(ctx, "nc"); close(listo) }() // lee precio 100 de MongoDB, lento
	time.Sleep(10 * time.Millisecond)

	p, _, _ := cont.Repositorio.ObtenerPlato(ctx, "p1")
	p.Precio, p.Version = 555, 2
	_ = r.ReemplazarPlato(ctx, p, 1, domain.EventoPlato(domain.PlatoActualizado, p, "", time.Now())) // gen 1 → 2
	<-listo                                                                                          // la lectura lenta guarda el precio viejo... en la clave de la generación 1

	cont.demora = 0
	menu, _ := r.ListarPlatosDeCocina(ctx, "nc")
	if menu[0].Precio != 555 {
		t.Fatalf("se sirvió el dato viejo: %d", menu[0].Precio)
	}
	if _, ok := alm.datos["catalogo:v1:menu:nc:g1"]; !ok {
		t.Fatal("la lectura lenta tenía que escribir en la clave vieja (g1)")
	}
}

func TestMemcachedCaidoSeLeeDeMongo(t *testing.T) {
	r, cont, alm := entorno(t)
	alm.caido = true
	menu, err := r.ListarPlatosDeCocina(context.Background(), "nc")
	if err != nil || len(menu) != 1 || cont.lecturas.Load() != 1 || r.Stats.Errores.Load() != 1 {
		t.Fatalf("con Memcached caído se lee de MongoDB: %v %v (lecturas %d, errores %d)", menu, err, cont.lecturas.Load(), r.Stats.Errores.Load())
	}
	// Escribir con Memcached caído no falla: el cambio ya está en MongoDB.
	p, _, _ := cont.Repositorio.ObtenerPlato(context.Background(), "p1")
	p.Version = 2
	if err := r.ReemplazarPlato(context.Background(), p, 1, domain.EventoPlato(domain.PlatoActualizado, p, "", time.Now())); err != nil {
		t.Fatalf("la escritura no puede fallar por la caché: %v", err)
	}
}

// 50 lecturas a la vez de una clave vacía: una sola va a MongoDB (contra el stampede).
func TestUnaSolaLecturaAMongoPorClave(t *testing.T) {
	r, cont, _ := entorno(t)
	cont.demora = 30 * time.Millisecond
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = r.ListarPlatosDeCocina(context.Background(), "nc") }()
	}
	wg.Wait()
	if n := cont.lecturas.Load(); n != 1 {
		t.Fatalf("lecturas a MongoDB: %d, esperaba 1", n)
	}
}

func TestNoSeCacheanLasLecturasInternas(t *testing.T) {
	r, _, alm := entorno(t)
	_, _ = r.ListarCocinas(context.Background(), false)
	if len(alm.datos) != 0 {
		t.Fatalf("la lista de todas las cocinas (Búsqueda, admin) no se cachea: %v", alm.datos)
	}
}

func TestTTLConMargen(t *testing.T) {
	for i := 0; i < 100; i++ {
		if d := ttlConMargen(); d < 54*time.Second || d > 66*time.Second {
			t.Fatalf("TTL fuera de 60 s ±10 %%: %v", d)
		}
	}
}

func TestCuentaAciertosYFallosEnMetricas(t *testing.T) {
	r, _, _ := entorno(t)
	aciertos := testutil.ToFloat64(usos.WithLabelValues("menu", "acierto"))
	fallos := testutil.ToFloat64(usos.WithLabelValues("menu", "fallo"))
	_, _ = r.ListarPlatosDeCocina(context.Background(), "nc") // fallo: va a MongoDB
	_, _ = r.ListarPlatosDeCocina(context.Background(), "nc") // acierto
	if testutil.ToFloat64(usos.WithLabelValues("menu", "fallo")) != fallos+1 ||
		testutil.ToFloat64(usos.WithLabelValues("menu", "acierto")) != aciertos+1 {
		t.Fatal("catalogo_cache_total no contó un fallo y un acierto del menú")
	}
}
