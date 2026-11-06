package http_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	adapter "ghostkitchen/inventario/internal/adapters/http"
	"ghostkitchen/inventario/internal/application"
	"ghostkitchen/inventario/internal/domain"
)

type memoria struct{ items []domain.Ingrediente }

func (m *memoria) Crear(_ context.Context, i domain.Ingrediente) error {
	m.items = append(m.items, i)
	return nil
}
func (m *memoria) Listar(_ context.Context, c string) ([]domain.Ingrediente, error) {
	out := []domain.Ingrediente{}
	for _, i := range m.items {
		if i.CocinaID == c {
			out = append(out, i)
		}
	}
	return out, nil
}
func (m *memoria) Obtener(_ context.Context, c, id string) (domain.Ingrediente, error) {
	for _, i := range m.items {
		if i.CocinaID == c && i.ID == id {
			return i, nil
		}
	}
	return domain.Ingrediente{}, domain.ErrNoEncontrado
}

func TestCargaIngredientesYPermisos(t *testing.T) {
	repo := &memoria{}
	r := adapter.Nuevo(application.Nuevo(repo), func(context.Context) error { return nil }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	hacer := func(method, path, body, rol, cocina string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if rol != "" {
			req.Header.Set("X-Usuario-Id", "u1")
			req.Header.Set("X-Rol", rol)
			req.Header.Set("X-Cocina-Id", cocina)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	valido := `{"nombre":" Harina ","unidad":"gramos","stockInicial":10}`
	for _, tc := range []struct {
		nombre, body, rol string
		status            int
	}{
		{"sin identidad", valido, "", 401}, {"admin", valido, "admin", 403}, {"cliente", valido, "cliente", 403},
		{"negativo", `{"nombre":"Sal","unidad":"g","stockInicial":-1}`, "cocina", 400},
		{"fraccionario", `{"nombre":"Sal","unidad":"g","stockInicial":1.5}`, "cocina", 400},
		{"sin stock", `{"nombre":"Sal","unidad":"g"}`, "cocina", 400},
		{"stock nulo", `{"nombre":"Sal","unidad":"g","stockInicial":null}`, "cocina", 400},
		{"sin nombre", `{"nombre":" ","unidad":"g","stockInicial":0}`, "cocina", 400},
		{"sin unidad", `{"nombre":"Sal","unidad":" ","stockInicial":0}`, "cocina", 400},
		{"cocina manipulada", `{"nombre":"Sal","unidad":"g","stockInicial":0,"cocinaId":"cerro"}`, "cocina", 400},
		{"dos objetos", valido + valido, "cocina", 400},
	} {
		t.Run(tc.nombre, func(t *testing.T) {
			rec := hacer(http.MethodPost, "/cocina/ingredientes", tc.body, tc.rol, "cerro")
			if rec.Code != tc.status {
				t.Fatalf("%d: %s", rec.Code, rec.Body.String())
			}
		})
	}
	if len(repo.items) != 0 {
		t.Fatal("una solicitud inválida escribió en el repositorio")
	}
	rec := hacer("POST", "/cocina/ingredientes", valido, "Cocina", "cerro")
	if rec.Code != 201 {
		t.Fatalf("%d: %s", rec.Code, rec.Body.String())
	}
	var creado domain.Ingrediente
	if err := json.Unmarshal(rec.Body.Bytes(), &creado); err != nil {
		t.Fatal(err)
	}
	if creado.Nombre != "Harina" || creado.Total != 10 || creado.Reservado != 0 || creado.Disponible != 10 || creado.CocinaID != "cerro" {
		t.Fatal(creado)
	}
	if rec.Header().Get("Location") != "/cocina/ingredientes/"+creado.ID {
		t.Fatal("falta Location")
	}
	if got := hacer("GET", "/cocina/ingredientes/"+creado.ID, "", "cocina", "barrio-jardin"); got.Code != 404 {
		t.Fatal("acceso cruzado", got.Code)
	}
	if got := hacer("GET", "/cocina/ingredientes", "", "cocina", "barrio-jardin"); got.Body.String() != `{"items":[]}` {
		t.Fatal("filtró ingredientes de otra cocina", got.Body.String())
	}
	if got := hacer("GET", "/cocina/ingredientes/"+creado.ID, "", "cocina", "cerro"); got.Code != 200 {
		t.Fatal(got.Code)
	}
}

func TestSaludNoOcultaFalloDeBase(t *testing.T) {
	r := adapter.Nuevo(application.Nuevo(&memoria{}), func(context.Context) error { return domain.ErrNoEncontrado }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for path, status := range map[string]int{"/health": 200, "/ready": 503} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != status {
			t.Fatalf("%s: %d", path, rec.Code)
		}
	}
}

func TestAPIReal(t *testing.T) {
	if os.Getenv("TEST_MYSQL") != "1" {
		t.Skip("requiere el servicio del entorno descartable")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	hacer := func(method, path, body, cocina, rol string) (int, []byte) {
		t.Helper()
		req, err := http.NewRequest(method, "http://inventario:8084"+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Usuario-Id", "prueba-interna")
		req.Header.Set("X-Rol", rol)
		req.Header.Set("X-Cocina-Id", cocina)
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		data, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode, data
	}
	status, body := hacer("POST", "/cocina/ingredientes", `{"nombre":"Arroz API","unidad":"gramos","stockInicial":7}`, "barrio-jardin", "cocina")
	if status != 201 {
		t.Fatalf("crear: %d %s", status, body)
	}
	var i domain.Ingrediente
	if err := json.Unmarshal(body, &i); err != nil {
		t.Fatal(err)
	}
	if i.Total != 7 || i.Reservado != 0 || i.Disponible != 7 {
		t.Fatal(i)
	}
	if status, _ := hacer("GET", "/cocina/ingredientes/"+i.ID, "", "barrio-jardin", "cocina"); status != 200 {
		t.Fatal("consultar", status)
	}
	if status, _ := hacer("GET", "/cocina/ingredientes/"+i.ID, "", "cerro", "cocina"); status != 404 {
		t.Fatal("acceso cruzado", status)
	}
	if status, _ := hacer("GET", "/cocina/ingredientes", "", "barrio-jardin", "admin"); status != 403 {
		t.Fatal("admin", status)
	}
}
