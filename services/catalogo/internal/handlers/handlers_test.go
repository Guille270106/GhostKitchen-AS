package handlers_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ghostkitchen/catalogo/internal/domain"
	"ghostkitchen/catalogo/internal/handlers"
	"ghostkitchen/catalogo/internal/repositories/memoria"
	"ghostkitchen/catalogo/internal/router"
	"ghostkitchen/catalogo/internal/seed"
	"ghostkitchen/catalogo/internal/services"
)

func entorno(t *testing.T) (http.Handler, *memoria.Repositorio) {
	t.Helper()
	repo := memoria.Nuevo()
	if _, _, err := seed.Cargar(context.Background(), repo, time.Now()); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return router.Nuevo(log, handlers.Nuevo(services.Nuevo(repo), repo.Ping)), repo
}

func get(t *testing.T, h http.Handler, ruta string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", ruta, nil))
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	return w, m
}

func TestListarCocinas(t *testing.T) {
	h, _ := entorno(t)
	w, m := get(t, h, "/cocinas")
	if w.Code != 200 || len(m["items"].([]any)) != 4 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestMenuDeCocina(t *testing.T) {
	h, _ := entorno(t)
	w, m := get(t, h, "/cocinas/nueva-cordoba/platos")
	if w.Code != 200 || len(m["items"].([]any)) != 16 {
		t.Fatal(w.Code, len(m["items"].([]any)))
	}
	if w, _ := get(t, h, "/cocinas/rio-cuarto/platos"); w.Code != 404 {
		t.Fatal(w.Code)
	}
}

func TestPlatoDesactivadoNoSeMuestraPeroSiSeReconstruye(t *testing.T) {
	h, _ := entorno(t)
	if w, _ := get(t, h, "/platos/p64"); w.Code != 404 {
		t.Fatalf("p64 está desactivado: %d", w.Code)
	}
	_, m := get(t, h, "/internal/platos?page=1&pageSize=100")
	if m["total"] != 64.0 || len(m["items"].([]any)) != 64 {
		t.Fatalf("la lectura interna tiene que incluir los desactivados: %v", m["total"])
	}
}

func TestPaginaInterna(t *testing.T) {
	h, _ := entorno(t)
	_, m := get(t, h, "/internal/platos?page=7&pageSize=10")
	if m["totalPages"] != 7.0 || len(m["items"].([]any)) != 4 {
		t.Fatalf("%v %v", m["totalPages"], len(m["items"].([]any)))
	}
	for _, q := range []string{"page=0", "pageSize=501", "pageSize=x"} {
		if w, _ := get(t, h, "/internal/platos?"+q); w.Code != 400 {
			t.Fatalf("%s: %d", q, w.Code)
		}
	}
}

func TestRestauranteDesactivadoOcultaSusPlatos(t *testing.T) {
	h, repo := entorno(t)
	c, _, _ := repo.ObtenerCocina(context.Background(), "nueva-cordoba")
	c.Restaurantes[0].Activo = false // Mostaza
	repo.Guardar(&c, nil)
	_, m := get(t, h, "/cocinas/nueva-cordoba/platos")
	for _, it := range m["items"].([]any) {
		if it.(map[string]any)["restauranteId"] == "r1" {
			t.Fatal("aparece un plato de un restaurante desactivado")
		}
	}
	_, m = get(t, h, "/cocinas/nueva-cordoba")
	if len(m["restaurantes"].([]any)) != 3 {
		t.Fatal("la cocina tiene que mostrar solo los restaurantes activos")
	}
}

func TestDetalleDePlatoRespetaRestauranteYCocinaActivos(t *testing.T) {
	h, repo := entorno(t)
	if w, _ := get(t, h, "/platos/p1"); w.Code != 200 {
		t.Fatalf("p1 activo: %d", w.Code)
	}

	c, _, _ := repo.ObtenerCocina(context.Background(), "nueva-cordoba")
	c.Restaurantes[0].Activo = false // Mostaza, el restaurante de p1
	repo.Guardar(&c, nil)
	if w, _ := get(t, h, "/platos/p1"); w.Code != 404 {
		t.Fatalf("p1 con su restaurante desactivado: %d", w.Code)
	}
	if w, _ := get(t, h, "/platos/p5"); w.Code != 200 {
		t.Fatalf("p5 es de otro restaurante de la misma cocina: %d", w.Code)
	}

	c.Restaurantes[0].Activo = true
	c.Activa = false
	repo.Guardar(&c, nil)
	if w, _ := get(t, h, "/platos/p5"); w.Code != 404 {
		t.Fatalf("p5 con su cocina desactivada: %d", w.Code)
	}
}

func TestMongoCaidoDevuelve503(t *testing.T) {
	h, repo := entorno(t)
	repo.Falla = domain.ErrNoDisponible
	for _, ruta := range []string{"/cocinas", "/cocinas/cerro/platos", "/platos/p1", "/internal/platos", "/ready"} {
		w, _ := get(t, h, ruta)
		if w.Code != 503 {
			t.Fatalf("%s: %d", ruta, w.Code)
		}
		if ruta != "/ready" && !strings.HasPrefix(w.Header().Get("Content-Type"), "application/problem+json") {
			t.Fatalf("%s: no es Problem Details", ruta)
		}
	}
}

func TestSeedNoPisaCambios(t *testing.T) {
	_, repo := entorno(t)
	p, _, _ := repo.ObtenerPlato(context.Background(), "p1")
	p.Precio = 99999
	repo.Guardar(nil, &p)
	if _, _, err := seed.Cargar(context.Background(), repo, time.Now()); err != nil {
		t.Fatal(err)
	}
	p, _, _ = repo.ObtenerPlato(context.Background(), "p1")
	if p.Precio != 99999 {
		t.Fatal("volver a cargar los datos iniciales pisó un cambio")
	}
}

// Las rutas internas de un plato y una cocina leen de la fuente de verdad, no de la caché: si la
// caché tiene un dato viejo, Pedidos y Usuarios igual deciden con el dato real.
func TestRutasInternasLeenDeLaFuenteYNoDeLaCache(t *testing.T) {
	cacheVieja, real := memoria.Nuevo(), memoria.Nuevo()
	for _, r := range []*memoria.Repositorio{cacheVieja, real} {
		if _, _, err := seed.Cargar(context.Background(), r, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	// En la fuente se desactivó el restaurante de p1 y la cocina "cerro"; la "caché" no se enteró.
	c, _, _ := real.ObtenerCocina(context.Background(), "nueva-cordoba")
	c.Restaurantes[0].Activo = false
	real.Guardar(&c, nil)
	cerro, _, _ := real.ObtenerCocina(context.Background(), "cerro")
	cerro.Activa = false
	real.Guardar(&cerro, nil)

	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	h := router.Nuevo(log, handlers.Nuevo(services.Nuevo(cacheVieja), real.Ping).ConFuente(services.Nuevo(real)))

	if w, _ := get(t, h, "/platos/p1"); w.Code != 200 {
		t.Fatalf("la ruta pública lee de la caché (vieja): %d", w.Code)
	}
	if w, _ := get(t, h, "/internal/platos/p1"); w.Code != 404 {
		t.Fatalf("la ruta interna tenía que ver el restaurante desactivado: %d", w.Code)
	}
	if w, m := get(t, h, "/internal/platos/p5"); w.Code != 200 || m["id"] != "p5" {
		t.Fatalf("un plato activo por la ruta interna: %d %v", w.Code, m)
	}
	if w, _ := get(t, h, "/internal/cocinas/cerro"); w.Code != 404 {
		t.Fatalf("la cocina desactivada por la ruta interna: %d", w.Code)
	}
	if w, _ := get(t, h, "/internal/cocinas/nueva-cordoba"); w.Code != 200 {
		t.Fatalf("una cocina activa por la ruta interna: %d", w.Code)
	}
}
