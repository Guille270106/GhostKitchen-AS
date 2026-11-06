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
