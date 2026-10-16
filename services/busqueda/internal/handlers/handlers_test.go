package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ghostkitchen/busqueda/internal/domain"
	"ghostkitchen/busqueda/internal/handlers"
	"ghostkitchen/busqueda/internal/repositories/memoria"
	"ghostkitchen/busqueda/internal/router"
	"ghostkitchen/busqueda/internal/seed"
	"ghostkitchen/busqueda/internal/services"
)

type entorno struct {
	srv   http.Handler
	repo  *memoria.Repositorio
	listo *atomic.Bool
}

func nuevoEntorno(t *testing.T, claves ...string) entorno {
	t.Helper()
	repo := memoria.Nuevo()
	if _, _, err := seed.Cargar(context.Background(), repo, time.Now()); err != nil {
		t.Fatal(err)
	}
	var listo atomic.Bool
	listo.Store(true)
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	srv := router.Nuevo(log,
		handlers.NuevoInterno(claves, repo.Ping, &listo),
		handlers.NuevoPublico(services.Nueva(repo)))
	return entorno{srv: srv, repo: repo, listo: &listo}
}

func (e entorno) pedir(t *testing.T, metodo, ruta, cuerpo string, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	var body io.Reader
	if cuerpo != "" {
		body = strings.NewReader(cuerpo)
	}
	req := httptest.NewRequest(metodo, ruta, body)
	if cuerpo != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	e.srv.ServeHTTP(w, req)
	return w
}

func leer(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("respuesta no es JSON: %s", w.Body.String())
	}
	return m
}

// problema verifica estado, code y formato Problem Details.
func problema(t *testing.T, w *httptest.ResponseRecorder, estado int, code string) map[string]any {
	t.Helper()
	if w.Code != estado {
		t.Fatalf("esperaba %d, obtuve %d: %s", estado, w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Fatalf("Content-Type %q", ct)
	}
	m := leer(t, w)
	if m["code"] != code || m["status"] != float64(estado) || m["correlationId"] == "" {
		t.Fatalf("problema inválido: %v", m)
	}
	return m
}

func TestListarSedes(t *testing.T) {
	e := nuevoEntorno(t)
	w := e.pedir(t, "GET", "/v1/sedes", "")
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	items := leer(t, w)["items"].([]any)
	if len(items) != 4 {
		t.Fatalf("esperaba 4 sedes, obtuve %d", len(items))
	}
	primera := items[0].(map[string]any)
	if primera["id"] != "barrio-jardin" || primera["ubicacion"] == nil || len(primera["restaurantes"].([]any)) != 4 {
		t.Fatalf("%v", primera)
	}
	if _, hay := primera["Activa"]; hay {
		t.Fatal("se filtró un campo interno")
	}
}

func TestCorrelationIdSeDevuelveOSeGenera(t *testing.T) {
	e := nuevoEntorno(t)
	w := e.pedir(t, "GET", "/v1/sedes", "", "X-Correlation-Id", "prueba-123")
	if w.Header().Get("X-Correlation-Id") != "prueba-123" {
		t.Fatal("no devolvió el X-Correlation-Id recibido")
	}
	w = e.pedir(t, "GET", "/v1/sedes", "", "X-Correlation-Id", "con espacios y <script>")
	if id := w.Header().Get("X-Correlation-Id"); id == "" || strings.Contains(id, " ") {
		t.Fatalf("tenía que generar uno nuevo, obtuve %q", id)
	}
}

func TestSedeCercana(t *testing.T) {
	e := nuevoEntorno(t)
	w := e.pedir(t, "GET", "/v1/sedes/cercana?lat=-31.333&lng=-64.255", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	m := leer(t, w)
	if m["sede"].(map[string]any)["id"] != "cerro" {
		t.Fatalf("%v", m)
	}
}

func TestSedeCercanaErrores(t *testing.T) {
	e := nuevoEntorno(t)
	m := problema(t, e.pedir(t, "GET", "/v1/sedes/cercana?lat=200&lng=-64.18", ""), 400, handlers.CodigoParametroInvalido)
	if errs := m["errores"].([]any); errs[0].(map[string]any)["parametro"] != "lat" {
		t.Fatalf("%v", errs)
	}
	problema(t, e.pedir(t, "GET", "/v1/sedes/cercana?lng=-64.18", ""), 400, handlers.CodigoParametroInvalido)
	problema(t, e.pedir(t, "GET", "/v1/sedes/cercana?lat=abc&lng=-64.18", ""), 400, handlers.CodigoParametroInvalido)
	problema(t, e.pedir(t, "GET", "/v1/sedes/cercana?lat=-34.60&lng=-58.38", ""), 404, handlers.CodigoSinCobertura)
}

func TestBuscarPlatos(t *testing.T) {
	e := nuevoEntorno(t)
	w := e.pedir(t, "GET", "/v1/sedes/nueva-cordoba/platos?q=HAMBURGUESAS&orden=precio_asc&pageSize=2", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	m := leer(t, w)
	if m["page"] != 1.0 || m["pageSize"] != 2.0 || m["sede"].(map[string]any)["id"] != "nueva-cordoba" {
		t.Fatalf("%v", m)
	}
	items := m["items"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["precio"].(float64) > items[1].(map[string]any)["precio"].(float64) {
		t.Fatalf("orden por precio incorrecto: %v", items)
	}
	if _, ok := items[0].(map[string]any)["actualizadoEn"]; !ok {
		t.Fatal("falta actualizadoEn")
	}
}

func TestBuscarPlatosErrores(t *testing.T) {
	e := nuevoEntorno(t)
	problema(t, e.pedir(t, "GET", "/v1/sedes/rio-cuarto/platos", ""), 404, handlers.CodigoSedeNoEncontrada)
	problema(t, e.pedir(t, "GET", "/v1/sedes/Nueva_Cordoba/platos", ""), 400, handlers.CodigoParametroInvalido)
	for _, q := range []string{"pageSize=51", "page=0", "precioMax=0", "orden=nombre", "etiqueta=Keto", "soloDisponibles=si", "q=" + strings.Repeat("a", 101)} {
		problema(t, e.pedir(t, "GET", "/v1/sedes/nueva-cordoba/platos?"+q, ""), 400, handlers.CodigoParametroInvalido)
	}
}

func TestObtenerPlato(t *testing.T) {
	e := nuevoEntorno(t)
	w := e.pedir(t, "GET", "/v1/platos/p5", "")
	if w.Code != 200 || leer(t, w)["disponible"] != false {
		t.Fatal(w.Code, w.Body.String())
	}
	problema(t, e.pedir(t, "GET", "/v1/platos/p999", ""), 404, handlers.CodigoPlatoNoEncontrado)
	problema(t, e.pedir(t, "GET", "/v1/platos/p64", ""), 404, handlers.CodigoPlatoNoEncontrado) // desactivado
}

func TestIndiceCaidoDevuelve503(t *testing.T) {
	e := nuevoEntorno(t)
	e.repo.Falla = errors.Join(domain.ErrNoDisponible, errors.New("connection refused"))
	for _, ruta := range []string{"/v1/sedes", "/v1/sedes/cercana?lat=-31.4&lng=-64.2", "/v1/sedes/nueva-cordoba/platos", "/v1/platos/p1"} {
		w := e.pedir(t, "GET", ruta, "")
		problema(t, w, 503, handlers.CodigoNoDisponible)
		if w.Header().Get("Retry-After") == "" {
			t.Fatalf("%s: falta Retry-After", ruta)
		}
	}
}

const cuerpoValido = `{"ubicacionCliente":{"lat":-31.4262,"lng":-64.1883},"ubicacionSede":{"lat":-31.40,"lng":-64.20},"radioKm":5,"incluirDarkKitchen":true}`

func TestProximidad(t *testing.T) {
	e := nuevoEntorno(t)
	w := e.pedir(t, "POST", "/v1/proximidad", cuerpoValido)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	m := leer(t, w)
	if m["sedeExterna"].(map[string]any)["dentroDelRango"] != true || m["darkKitchen"].(map[string]any)["estado"] != "EN_RANGO" {
		t.Fatalf("%v", m)
	}
	// Sin incluirDarkKitchen: darkKitchen viene en null.
	w = e.pedir(t, "POST", "/v1/proximidad", `{"ubicacionCliente":{"lat":0,"lng":0},"ubicacionSede":{"lat":0,"lng":0},"radioKm":1}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"darkKitchen":null`) {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestProximidadValidaciones(t *testing.T) {
	e := nuevoEntorno(t)
	casos := map[string]string{
		"booleano como texto": `{"ubicacionCliente":{"lat":0,"lng":0},"ubicacionSede":{"lat":0,"lng":0},"radioKm":5,"incluirDarkKitchen":"true"}`,
		"lista":               `[{"ubicacionCliente":{"lat":0,"lng":0},"ubicacionSede":{"lat":0,"lng":0},"radioKm":5}]`,
		"lista de sedes":      `{"ubicacionCliente":{"lat":0,"lng":0},"ubicacionSede":[{"lat":0,"lng":0}],"radioKm":5}`,
		"campo extra":         `{"ubicacionCliente":{"lat":0,"lng":0},"ubicacionSede":{"lat":0,"lng":0},"radioKm":5,"sedes":[]}`,
		"falta radio":         `{"ubicacionCliente":{"lat":0,"lng":0},"ubicacionSede":{"lat":0,"lng":0}}`,
		"radio 0":             `{"ubicacionCliente":{"lat":0,"lng":0},"ubicacionSede":{"lat":0,"lng":0},"radioKm":0}`,
		"radio 501":           `{"ubicacionCliente":{"lat":0,"lng":0},"ubicacionSede":{"lat":0,"lng":0},"radioKm":501}`,
		"latitud inválida":    `{"ubicacionCliente":{"lat":91,"lng":0},"ubicacionSede":{"lat":0,"lng":0},"radioKm":5}`,
		"falta lng":           `{"ubicacionCliente":{"lat":0},"ubicacionSede":{"lat":0,"lng":0},"radioKm":5}`,
		"falta sede":          `{"ubicacionCliente":{"lat":0,"lng":0},"radioKm":5}`,
		"dos objetos":         cuerpoValido + cuerpoValido,
		"mal formado":         `{"ubicacionCliente":`,
		"vacío":               ` `,
	}
	for nombre, cuerpo := range casos {
		t.Run(nombre, func(t *testing.T) {
			problema(t, e.pedir(t, "POST", "/v1/proximidad", cuerpo), 400, handlers.CodigoParametroInvalido)
		})
	}
}

func TestProximidadCuerpoDemasiadoGrande(t *testing.T) {
	e := nuevoEntorno(t)
	grande := `{"ubicacionCliente":{"lat":0,"lng":0},"ubicacionSede":{"lat":0,"lng":0},"radioKm":5,"x":"` + strings.Repeat("a", 70<<10) + `"}`
	problema(t, e.pedir(t, "POST", "/v1/proximidad", grande), 413, handlers.CodigoSolicitudMuyGrande)
}

func TestProximidadSinDarkKitchenFuncionaConIndiceCaido(t *testing.T) {
	e := nuevoEntorno(t)
	e.repo.Falla = domain.ErrNoDisponible
	w := e.pedir(t, "POST", "/v1/proximidad", `{"ubicacionCliente":{"lat":0,"lng":0},"ubicacionSede":{"lat":0,"lng":0},"radioKm":1}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	problema(t, e.pedir(t, "POST", "/v1/proximidad", cuerpoValido), 503, handlers.CodigoNoDisponible)
}

func TestVerificarAPIKey(t *testing.T) {
	e := nuevoEntorno(t, "clave-a", "clave-b")
	if w := e.pedir(t, "GET", "/internal/apikey/verificar", "", "X-API-Key", "clave-b"); w.Code != 204 {
		t.Fatalf("clave válida: %d", w.Code)
	}
	if w := e.pedir(t, "GET", "/internal/apikey/verificar", "", "X-API-Key", "otra"); w.Code != 401 {
		t.Fatalf("clave inválida: %d", w.Code)
	}
	if w := e.pedir(t, "GET", "/internal/apikey/verificar", ""); w.Code != 401 {
		t.Fatalf("sin clave: %d", w.Code)
	}
	sinClaves := nuevoEntorno(t)
	if w := sinClaves.pedir(t, "GET", "/internal/apikey/verificar", "", "X-API-Key", "cualquiera"); w.Code != 401 {
		t.Fatalf("sin claves configuradas tiene que rechazar todo: %d", w.Code)
	}
}

func TestSaludYPreparado(t *testing.T) {
	e := nuevoEntorno(t)
	if w := e.pedir(t, "GET", "/health", ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := e.pedir(t, "GET", "/ready", ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	e.listo.Store(false)
	if w := e.pedir(t, "GET", "/ready", ""); w.Code != 503 {
		t.Fatal("antes de preparar el índice /ready tiene que dar 503")
	}
	e.listo.Store(true)
	e.repo.Falla = domain.ErrNoDisponible
	if w := e.pedir(t, "GET", "/ready", ""); w.Code != 503 {
		t.Fatal("con Solr caído /ready tiene que dar 503")
	}
}
