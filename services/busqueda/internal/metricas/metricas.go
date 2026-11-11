// Package metricas publica las métricas del servicio para Prometheus en GET /metrics (ADR-011).
//
// El middleware cuenta cada solicitud y mide cuánto tardó, por método, ruta y código de
// respuesta: con eso Grafana arma el método RED (solicitudes por segundo, % de errores y
// p50/p95/p99 de duración). La ruta es la plantilla de Gin ("/platos/:id") y no la URL real:
// así no se registran ids ni parámetros (que pueden traer texto buscado o coordenadas), y la
// cantidad de series queda acotada.
//
// Los nombres siguen la convención de Prometheus (en inglés), para que los tableros y las
// consultas de Grafana sean las de siempre. El servicio y la copia los agrega Prometheus al
// leer (job e instance).
package metricas

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Ruta donde Prometheus lee las métricas. Solo se puede llegar desde adentro de Docker: el
// gateway no la publica.
const Ruta = "/metrics"

var (
	busquedas = promauto.NewCounter(prometheus.CounterOpts{
		Name: "busqueda_consultas_total",
		Help: "Búsquedas de platos completadas correctamente.",
	})
	sinResultados = promauto.NewCounter(prometheus.CounterOpts{
		Name: "busqueda_sin_resultados_total",
		Help: "Búsquedas de platos completadas con total cero, sin datos del usuario.",
	})
	solicitudes = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Solicitudes HTTP atendidas, por método, ruta y código de respuesta.",
	}, []string{"method", "route", "status"})

	// Los límites de los baldes rodean los objetivos del ADR-011: 300 ms (búsqueda), 500 ms
	// (agregar al carrito) y 3 s (plazo de cualquier pantalla).
	duracion = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "Duración de las solicitudes HTTP, por método y ruta.",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.2, 0.3, 0.5, 1, 2, 3, 5},
	}, []string{"method", "route"})
)

// RegistrarBusqueda cuenta resultados exitosos; una página vacía no implica total cero.
func RegistrarBusqueda(total int) {
	busquedas.Inc()
	if total == 0 {
		sinResultados.Inc()
	}
}

// Middleware cuenta y mide cada solicitud. Va primero (antes de gin.Recovery), así una
// solicitud que termina en panic igual cuenta como 500.
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == Ruta {
			c.Next() // Prometheus leyendo cada 15 s no es tráfico del sistema
			return
		}
		inicio := time.Now()
		c.Next()
		ruta := c.FullPath()
		if ruta == "" {
			ruta = "sin_ruta" // 404: una sola serie para todas las rutas que no existen
		}
		solicitudes.WithLabelValues(c.Request.Method, ruta, strconv.Itoa(c.Writer.Status())).Inc()
		duracion.WithLabelValues(c.Request.Method, ruta).Observe(time.Since(inicio).Seconds())
	}
}

// Handler responde GET /metrics con todas las métricas registradas, incluidas las de Go
// (memoria, goroutines) y las del proceso (CPU, archivos abiertos).
func Handler() gin.HandlerFunc {
	return gin.WrapH(promhttp.Handler())
}
