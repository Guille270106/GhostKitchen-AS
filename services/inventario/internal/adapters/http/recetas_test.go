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

type recetasMemoria struct {
	receta     domain.Receta
	escrituras int
}

func TestRecetaAPIReal(t *testing.T) {
	if os.Getenv("TEST_MYSQL") != "1" {
		t.Skip("requiere el entorno descartable")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	hacer := func(method, path, cocina, body string) (int, []byte) {
		t.Helper()
		req, err := http.NewRequest(method, "http://inventario:8084"+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Usuario-Id", "prueba-recetas")
		req.Header.Set("X-Rol", "cocina")
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
	crear := func(cocina string) domain.Ingrediente {
		t.Helper()
		status, data := hacer("POST", "/cocina/ingredientes", cocina, `{"nombre":"Receta API","unidad":"gramos","stockInicial":10}`)
		if status != 201 {
			t.Fatal(status, string(data))
		}
		var i domain.Ingrediente
		if err := json.Unmarshal(data, &i); err != nil {
			t.Fatal(err)
		}
		return i
	}
	propio, ajeno := crear("nueva-cordoba"), crear("barrio-jardin")
	body := `{"items":[{"ingredienteId":"` + propio.ID + `","cantidad":2}]}`
	status, data := hacer("PUT", "/cocina/platos/p1/receta", "nueva-cordoba", body)
	if status != 200 {
		t.Fatal(status, string(data))
	}
	var receta domain.Receta
	if err := json.Unmarshal(data, &receta); err != nil {
		t.Fatal(err)
	}
	// p1 puede traer ya la receta de ejemplo (migración 000009): la versión sube, no vuelve a 1.
	if receta.Version < 1 || len(receta.Items) != 1 || receta.Items[0].Cantidad != 2 {
		t.Fatal(receta)
	}
	if status, data := hacer("PUT", "/cocina/platos/p1/receta", "nueva-cordoba", `{"items":[{"ingredienteId":"`+ajeno.ID+`","cantidad":1}]}`); status != 404 {
		t.Fatal("ingrediente ajeno", status, string(data))
	}
	status, data = hacer("GET", "/cocina/platos/p1/receta", "nueva-cordoba", "")
	if status != 200 {
		t.Fatal(status, string(data))
	}
	var despues domain.Receta
	if err := json.Unmarshal(data, &despues); err != nil {
		t.Fatal(err)
	}
	if despues.Version != receta.Version || len(despues.Items) != 1 || despues.Items[0].IngredienteID != propio.ID {
		t.Fatal("modificación parcial", despues)
	}
	if status, _ := hacer("GET", "/cocina/platos/p1/receta", "barrio-jardin", ""); status != 404 {
		t.Fatal("plato ajeno", status)
	}
	if status, _ := hacer("GET", "/cocina/recetas", "nueva-cordoba", ""); status != 200 {
		t.Fatal("listado", status)
	}
	if status, _ := hacer("PUT", "/cocina/platos/p64/receta", "barrio-jardin", body); status != 404 {
		t.Fatal("plato inactivo", status)
	}
}

func (m *recetasMemoria) GuardarReceta(_ context.Context, r domain.Receta) (domain.Receta, error) {
	if r.PlatoID != "p17" || r.CocinaID != "cerro" {
		return domain.Receta{}, domain.ErrNoEncontrado
	}
	for _, i := range r.Items {
		if i.IngredienteID != "i1" && i.IngredienteID != "i2" {
			return domain.Receta{}, domain.ErrNoEncontrado
		}
	}
	r.Version = m.receta.Version + 1
	m.receta = r
	m.escrituras++
	return r, nil
}
func (m *recetasMemoria) ObtenerReceta(_ context.Context, c, p string) (domain.Receta, error) {
	if m.receta.PlatoID != p || m.receta.CocinaID != c {
		return domain.Receta{}, domain.ErrNoEncontrado
	}
	return m.receta, nil
}
func (m *recetasMemoria) ListarRecetas(_ context.Context, c string) ([]domain.Receta, error) {
	if m.receta.PlatoID != "" && m.receta.CocinaID == c {
		return []domain.Receta{m.receta}, nil
	}
	return []domain.Receta{}, nil
}

func TestRecetasValidacionPermisosYReemplazo(t *testing.T) {
	repo := &recetasMemoria{}
	r := adapter.Nuevo(application.Nuevo(&memoria{}), func(context.Context) error { return nil }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	adapter.RegistrarRecetas(r, application.NuevasRecetas(repo))
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
	valido := `{"items":[{"ingredienteId":"i2","cantidad":2},{"ingredienteId":"i1","cantidad":1}]}`
	for _, tc := range []struct {
		nombre, body, rol, cocina, plato string
		status                           int
	}{
		{"sin identidad", valido, "", "cerro", "p17", 401},
		{"admin", valido, "admin", "cerro", "p17", 403},
		{"cliente", valido, "cliente", "cerro", "p17", 403},
		{"sin cocina", valido, "cocina", "", "p17", 403},
		{"plato ajeno", valido, "cocina", "villa-allende", "p17", 404},
		{"desconocido", valido, "cocina", "cerro", "otro", 404},
		{"vacía", `{"items":[]}`, "cocina", "cerro", "p17", 400},
		{"sin items", `{}`, "cocina", "cerro", "p17", 400},
		{"nula", `{"items":null}`, "cocina", "cerro", "p17", 400},
		{"cero", `{"items":[{"ingredienteId":"i1","cantidad":0}]}`, "cocina", "cerro", "p17", 400},
		{"negativa", `{"items":[{"ingredienteId":"i1","cantidad":-1}]}`, "cocina", "cerro", "p17", 400},
		{"decimal", `{"items":[{"ingredienteId":"i1","cantidad":1.5}]}`, "cocina", "cerro", "p17", 400},
		{"sin cantidad", `{"items":[{"ingredienteId":"i1"}]}`, "cocina", "cerro", "p17", 400},
		{"cantidad nula", `{"items":[{"ingredienteId":"i1","cantidad":null}]}`, "cocina", "cerro", "p17", 400},
		{"sin ingrediente", `{"items":[{"cantidad":1}]}`, "cocina", "cerro", "p17", 400},
		{"duplicado", `{"items":[{"ingredienteId":"i1","cantidad":1},{"ingredienteId":"i1","cantidad":2}]}`, "cocina", "cerro", "p17", 400},
		{"ingrediente ajeno", `{"items":[{"ingredienteId":"ajeno","cantidad":1}]}`, "cocina", "cerro", "p17", 404},
		{"cocina manipulada", `{"cocinaId":"cerro","items":[{"ingredienteId":"i1","cantidad":1}]}`, "cocina", "cerro", "p17", 400},
		{"campo extra en item", `{"items":[{"ingredienteId":"i1","cantidad":1,"cocinaId":"cerro"}]}`, "cocina", "cerro", "p17", 400},
		{"dos objetos", valido + valido, "cocina", "cerro", "p17", 400},
	} {
		t.Run(tc.nombre, func(t *testing.T) {
			rec := hacer("PUT", "/cocina/platos/"+tc.plato+"/receta", tc.body, tc.rol, tc.cocina)
			if rec.Code != tc.status {
				t.Fatalf("%d %s", rec.Code, rec.Body.String())
			}
		})
	}
	if repo.escrituras != 0 {
		t.Fatal("una solicitud inválida modificó la receta")
	}
	rec := hacer("PUT", "/cocina/platos/p17/receta", valido, "cocina", "cerro")
	if rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	var creada domain.Receta
	if err := json.Unmarshal(rec.Body.Bytes(), &creada); err != nil {
		t.Fatal(err)
	}
	if creada.Version != 1 || len(creada.Items) != 2 || creada.Items[0].IngredienteID != "i1" {
		t.Fatal(creada)
	}
	rec = hacer("PUT", "/cocina/platos/p17/receta", `{"items":[{"ingredienteId":"i1","cantidad":9}]}`, "cocina", "cerro")
	if rec.Code != 200 || len(repo.receta.Items) != 1 || repo.receta.Version != 2 || repo.receta.Items[0].Cantidad != 9 {
		t.Fatal("no reemplazó la receta", rec.Body.String())
	}
	if got := hacer("GET", "/cocina/platos/p17/receta", "", "cocina", "cerro"); got.Code != 200 {
		t.Fatal(got.Code)
	}
	if got := hacer("GET", "/cocina/platos/p17/receta", "", "cocina", "villa-allende"); got.Code != 404 {
		t.Fatal("lectura ajena", got.Code)
	}
	if got := hacer("GET", "/cocina/recetas", "", "cocina", "villa-allende"); got.Body.String() != `{"items":[]}` {
		t.Fatal(got.Body.String())
	}
	for _, path := range []string{"/cocina/recetas", "/cocina/platos/p17/receta"} {
		if got := hacer("GET", path, "", "admin", "cerro"); got.Code != 403 {
			t.Fatal("admin en lectura", got.Code)
		}
	}
}
