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

type recetas map[string]bool

func (r recetas) TieneReceta(_ context.Context, _, _, platoID, _ string) (bool, error) {
	return r[platoID], nil
}

func entornoPlatos(t *testing.T, rec recetas) http.Handler {
	t.Helper()
	repo := memoria.Nuevo()
	if _, _, err := seed.Cargar(context.Background(), repo, time.Now()); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return router.Nuevo(log, handlers.Nuevo(services.Nuevo(repo), repo.Ping), handlers.NuevoPlatos(services.NuevoPlatosCocina(repo, repo, rec)))
}

func comoCocina(h http.Handler, cocina, metodo, ruta, cuerpo string) (*httptest.ResponseRecorder, map[string]any) {
	req := httptest.NewRequest(metodo, ruta, strings.NewReader(cuerpo))
	if cocina != "" {
		req.Header.Set("X-Usuario-Id", "k1")
		req.Header.Set("X-Rol", "cocina")
		req.Header.Set("X-Cocina-Id", cocina)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	return w, m
}

func TestCocinaCreaYActivaUnPlato(t *testing.T) {
	rec := recetas{}
	h := entornoPlatos(t, rec)
	_, cocina := get(t, h, "/cocinas/nueva-cordoba")
	rid := cocina["restaurantes"].([]any)[0].(map[string]any)["id"].(string)

	w, p := comoCocina(h, "nueva-cordoba", "POST", "/cocina/restaurantes/"+rid+"/platos",
		`{"nombre":"Mega triple","descripcion":"Tres medallones","categoria":"Hamburguesas","precio":12500,"etiquetas":["Picante"]}`)
	if w.Code != 201 || p["activo"] != false {
		t.Fatalf("crear: %d %s", w.Code, w.Body)
	}
	id := p["id"].(string)
	if w, _ := get(t, h, "/platos/"+id); w.Code != 404 {
		t.Fatalf("desactivado no se publica: %d", w.Code)
	}

	w, m := comoCocina(h, "nueva-cordoba", "PATCH", "/cocina/platos/"+id, `{"activo":true}`)
	if w.Code != 409 || m["code"] != "SIN_RECETA" {
		t.Fatalf("sin receta: %d %s", w.Code, w.Body)
	}
	rec[id] = true
	if w, _ := comoCocina(h, "nueva-cordoba", "PATCH", "/cocina/platos/"+id, `{"activo":true}`); w.Code != 200 {
		t.Fatalf("con receta: %d %s", w.Code, w.Body)
	}
	if w, _ := get(t, h, "/platos/"+id); w.Code != 200 {
		t.Fatalf("activo se publica: %d", w.Code)
	}

	w, m = comoCocina(h, "nueva-cordoba", "GET", "/cocina/platos", "")
	if w.Code != 200 || len(m["items"].([]any)) != 17 {
		t.Fatalf("mis platos (16 del seed + el nuevo): %d %v", w.Code, len(m["items"].([]any)))
	}
}

func TestCocinaRechazos(t *testing.T) {
	h := entornoPlatos(t, recetas{})
	casos := []struct {
		nombre, cocina, metodo, ruta, cuerpo string
		estado                               int
		codigo                               string
	}{
		{"sin identidad", "", "GET", "/cocina/platos", "", 401, "NO_AUTENTICADO"},
		{"plato de otra cocina", "cerro", "PATCH", "/cocina/platos/p1", `{"precio":1}`, 403, "OTRA_COCINA"},
		{"restaurante de otra cocina", "cerro", "POST", "/cocina/restaurantes/r1/platos", `{"nombre":"X","categoria":"Y","precio":1}`, 404, "RESTAURANTE_NO_ENCONTRADO"},
		{"etiqueta inventada", "nueva-cordoba", "PATCH", "/cocina/platos/p1", `{"etiquetas":["Keto"]}`, 400, "DATOS_INVALIDOS"},
		{"manda la cocina", "nueva-cordoba", "PATCH", "/cocina/platos/p1", `{"cocinaId":"cerro"}`, 400, "DATOS_INVALIDOS"},
		{"plato inexistente", "nueva-cordoba", "PATCH", "/cocina/platos/zz", `{"precio":1}`, 404, "PLATO_NO_ENCONTRADO"},
	}
	for _, c := range casos {
		w, m := comoCocina(h, c.cocina, c.metodo, c.ruta, c.cuerpo)
		if w.Code != c.estado || m["code"] != c.codigo {
			t.Errorf("%s: esperaba %d %s, obtuve %d %s", c.nombre, c.estado, c.codigo, w.Code, w.Body)
		}
	}
}
