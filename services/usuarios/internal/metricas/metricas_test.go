package metricas

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// Cuenta por plantilla de ruta (sin el id real), con el código de respuesta, también cuando
// el handler entra en panic; y no cuenta las lecturas de /metrics.
func TestMiddlewareCuentaPorRuta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware(), gin.Recovery())
	r.GET(Ruta, Handler())
	r.GET("/prueba/:id", func(c *gin.Context) { c.Status(http.StatusTeapot) })
	r.GET("/rompe", func(c *gin.Context) { panic("x") })

	for _, ruta := range []string{"/prueba/123", "/prueba/456", "/rompe", "/no-existe", Ruta} {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, ruta, nil))
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, Ruta, nil))
	cuerpo := w.Body.String()

	for _, quiero := range []string{
		`http_requests_total{method="GET",route="/prueba/:id",status="418"} 2`,
		`http_requests_total{method="GET",route="/rompe",status="500"} 1`,
		`http_requests_total{method="GET",route="sin_ruta",status="404"} 1`,
		`http_request_duration_seconds_count{method="GET",route="/prueba/:id"} 2`,
		`go_goroutines`,
	} {
		if !strings.Contains(cuerpo, quiero) {
			t.Errorf("falta %s", quiero)
		}
	}
	for _, no := range []string{"123", "456", `route="/metrics"`} {
		if strings.Contains(cuerpo, `route="`+no) || strings.Contains(cuerpo, no+`"`) {
			t.Errorf("no tendría que aparecer %s", no)
		}
	}
}
