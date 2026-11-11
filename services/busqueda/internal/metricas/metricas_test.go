package metricas

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

func TestRegistrarBusqueda(t *testing.T) {
	leer := func(nombre string) float64 {
		t.Helper()
		familias, err := prometheus.DefaultGatherer.Gather()
		if err != nil {
			t.Fatal(err)
		}
		for _, familia := range familias {
			if familia.GetName() == nombre {
				if len(familia.Metric) != 1 || len(familia.Metric[0].Label) != 0 {
					t.Fatal("la métrica no debe tener etiquetas")
				}
				return familia.Metric[0].GetCounter().GetValue()
			}
		}
		t.Fatalf("falta %s", nombre)
		return 0
	}
	total, vacias := leer("busqueda_consultas_total"), leer("busqueda_sin_resultados_total")
	RegistrarBusqueda(0)
	RegistrarBusqueda(10)
	if leer("busqueda_consultas_total")-total != 2 || leer("busqueda_sin_resultados_total")-vacias != 1 {
		t.Fatal("conteo incorrecto")
	}
}

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
