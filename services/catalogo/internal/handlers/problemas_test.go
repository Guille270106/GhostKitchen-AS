package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// Los errores de Catálogo tienen el mismo formato que los de Búsqueda, Pedidos e Inventario
// (L22): "type" con el código en minúsculas y guiones, y "title" en castellano.
func TestFormatoDeErrorIgualAlDeLosDemasServicios(t *testing.T) {
	gin.SetMode(gin.TestMode)
	casos := []struct {
		estado       int
		codigo       string
		tipo, titulo string
	}{
		{404, "PLATO_NO_ENCONTRADO", "https://ghostkitchen.dev/errores/plato-no-encontrado", "Plato no encontrado"},
		{409, "SIN_RECETA", "https://ghostkitchen.dev/errores/sin-receta", "El plato no tiene receta"},
		{503, "SERVICIO_NO_DISPONIBLE", "https://ghostkitchen.dev/errores/servicio-no-disponible", "Servicio no disponible"},
		{404, "CODIGO_NUEVO", "https://ghostkitchen.dev/errores/codigo-nuevo", "No encontrado"}, // sin título propio
	}
	for _, c := range casos {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		responderProblema(ctx, c.estado, c.codigo, "detalle")
		var p map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &p)
		if w.Code != c.estado || p["type"] != c.tipo || p["title"] != c.titulo || p["code"] != c.codigo ||
			w.Header().Get("Content-Type") != "application/problem+json" {
			t.Errorf("%s: %d %v", c.codigo, w.Code, p)
		}
	}
}
