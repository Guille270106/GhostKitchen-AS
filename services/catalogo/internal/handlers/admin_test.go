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

	"ghostkitchen/catalogo/internal/handlers"
	"ghostkitchen/catalogo/internal/repositories/memoria"
	"ghostkitchen/catalogo/internal/router"
	"ghostkitchen/catalogo/internal/seed"
	"ghostkitchen/catalogo/internal/services"
)

func entornoAdmin(t *testing.T) http.Handler {
	t.Helper()
	repo := memoria.Nuevo()
	if _, _, err := seed.Cargar(context.Background(), repo, time.Now()); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return router.Nuevo(log, handlers.Nuevo(services.Nuevo(repo), repo.Ping), handlers.NuevoAdmin(services.NuevoAdmin(repo, repo)))
}

// pedirComo hace una solicitud con la identidad que pondría el gateway (G9).
func pedirComo(h http.Handler, rol, localidad, metodo, ruta, cuerpo string) (*httptest.ResponseRecorder, map[string]any) {
	req := httptest.NewRequest(metodo, ruta, strings.NewReader(cuerpo))
	if rol != "" {
		req.Header.Set("X-Usuario-Id", "u1")
		req.Header.Set("X-Rol", rol)
	}
	if localidad != "" {
		req.Header.Set("X-Localidad", localidad)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	return w, m
}

// Así llega la localidad desde el gateway: codificada para URL.
const nc = "Nueva%20C%C3%B3rdoba"

func TestAdminListaSoloSuLocalidad(t *testing.T) {
	h := entornoAdmin(t)
	w, m := pedirComo(h, "admin", nc, "GET", "/admin/cocinas", "")
	items, _ := m["items"].([]any)
	if w.Code != 200 || len(items) != 1 || items[0].(map[string]any)["id"] != "nueva-cordoba" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestAdminCreaCocinaYRestaurantes(t *testing.T) {
	h := entornoAdmin(t)
	w, m := pedirComo(h, "admin", nc, "POST", "/admin/cocinas",
		`{"nombre":"Cocina Güemes","direccion":"Belgrano 800","ubicacion":{"lat":-31.43,"lng":-64.19},"tiempoEntregaMin":20,"tiempoEntregaMax":30,"costoEnvio":1000}`)
	if w.Code != 201 || m["id"] != "guemes" || m["localidad"] != "Nueva Córdoba" || w.Header().Get("Location") != "/cocinas/guemes" {
		t.Fatalf("crear: %d %s", w.Code, w.Body)
	}
	// La cocina nueva ya se ve en la lectura pública.
	if w, _ := get(t, h, "/cocinas/guemes"); w.Code != 200 {
		t.Fatalf("la cocina nueva tiene que verse: %d", w.Code)
	}
	for i, n := range []string{"Mostaza", "Sushi Pop", "Café Martínez", "La Cabrera"} {
		if w, _ := pedirComo(h, "admin", nc, "POST", "/admin/cocinas/guemes/restaurantes", `{"nombre":"`+n+`"}`); w.Code != 201 {
			t.Fatalf("restaurante %d: %d %s", i, w.Code, w.Body)
		}
	}
	w, m = pedirComo(h, "admin", nc, "POST", "/admin/cocinas/guemes/restaurantes", `{"nombre":"Quinto"}`)
	if w.Code != 409 || m["code"] != "COCINA_LLENA" {
		t.Fatalf("quinto: %d %s", w.Code, w.Body)
	}
}

func TestAdminDesactivaCocinaYDejaDeVerse(t *testing.T) {
	h := entornoAdmin(t)
	w, m := pedirComo(h, "admin", nc, "PATCH", "/admin/cocinas/nueva-cordoba", `{"activa":false}`)
	if w.Code != 200 || m["activa"] != false || m["version"].(float64) < 2 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w, _ := get(t, h, "/cocinas/nueva-cordoba"); w.Code != 404 {
		t.Fatalf("una cocina desactivada no se publica (RN19): %d", w.Code)
	}
}

func TestAdminDesactivaRestauranteYSusPlatosNoSeVen(t *testing.T) {
	h := entornoAdmin(t)
	_, antes := get(t, h, "/cocinas/nueva-cordoba/platos")
	n := len(antes["items"].([]any))
	_, cocina := get(t, h, "/cocinas/nueva-cordoba")
	rid := cocina["restaurantes"].([]any)[0].(map[string]any)["id"].(string)
	if w, _ := pedirComo(h, "admin", nc, "PATCH", "/admin/cocinas/nueva-cordoba/restaurantes/"+rid, `{"activo":false}`); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	_, despues := get(t, h, "/cocinas/nueva-cordoba/platos")
	if len(despues["items"].([]any)) >= n {
		t.Fatalf("los platos del restaurante desactivado no se publican: antes %d, después %d", n, len(despues["items"].([]any)))
	}
}

func TestAdminRechazos(t *testing.T) {
	h := entornoAdmin(t)
	casos := []struct {
		nombre, rol, localidad, metodo, ruta, cuerpo string
		estado                                       int
		codigo                                       string
	}{
		{"sin identidad", "", "", "GET", "/admin/cocinas", "", 401, "NO_AUTENTICADO"},
		{"un cliente", "cliente", "", "GET", "/admin/cocinas", "", 403, "SIN_PERMISO"},
		{"una cuenta Cocina", "cocina", "", "POST", "/admin/cocinas/nueva-cordoba/restaurantes", `{"nombre":"X"}`, 403, "SIN_PERMISO"},
		{"otra localidad", "admin", nc, "PATCH", "/admin/cocinas/cerro", `{"activa":false}`, 403, "OTRA_LOCALIDAD"},
		{"cocina inexistente", "admin", nc, "PATCH", "/admin/cocinas/rio-cuarto", `{"activa":false}`, 404, "COCINA_NO_ENCONTRADA"},
		{"restaurante repetido", "admin", nc, "POST", "/admin/cocinas/nueva-cordoba/restaurantes", `{"nombre":"MOSTAZA"}`, 409, "RESTAURANTE_REPETIDO"},
		{"manda la localidad", "admin", nc, "POST", "/admin/cocinas", `{"nombre":"X","localidad":"Cerro"}`, 400, "DATOS_INVALIDOS"},
		{"datos inválidos", "admin", nc, "PATCH", "/admin/cocinas/nueva-cordoba", `{"costoEnvio":-5}`, 400, "DATOS_INVALIDOS"},
		{"restaurante inexistente", "admin", nc, "PATCH", "/admin/cocinas/nueva-cordoba/restaurantes/zz", `{"activo":false}`, 404, "RESTAURANTE_NO_ENCONTRADO"},
	}
	for _, c := range casos {
		w, m := pedirComo(h, c.rol, c.localidad, c.metodo, c.ruta, c.cuerpo)
		if w.Code != c.estado || m["code"] != c.codigo {
			t.Errorf("%s: esperaba %d %s, obtuve %d %s", c.nombre, c.estado, c.codigo, w.Code, w.Body)
		}
	}
}
