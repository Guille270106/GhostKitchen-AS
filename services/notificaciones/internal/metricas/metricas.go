// Package metricas publica las métricas del worker de mails para Prometheus en GET /metrics
// (ADR-011).
//
// Notificaciones no atiende solicitudes HTTP del sistema: su trabajo son los mensajes. Por eso,
// en lugar de contar solicitudes, cuenta cada mensaje según cómo terminó y mide cuánto tarda
// mandar un mail. Con eso se distingue un worker caído (no aparece en Prometheus) de uno sin
// trabajo (aparece, pero no cuenta mensajes) y de uno con problemas (cuenta reintentos o DLQ).
package metricas

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Ruta donde Prometheus lee las métricas. Solo se llega desde adentro de Docker.
const Ruta = "/metrics"

// Cómo puede terminar un mensaje.
const (
	Enviado   = "enviado"   // se mandó el mail y se marcó ENVIADO
	Duplicado = "duplicado" // ya estaba ENVIADO: el mismo evento llegó otra vez y no se reenvía
	Reintento = "reintento" // falló algo pasajero: vuelve a la cola para otro intento
	DLQ       = "dlq"       // se agotaron los intentos: va a notificaciones.mailer.dlq
	Invalido  = "invalido"  // el mensaje no cumple el formato: va directo a la DLQ
)

var (
	mensajes = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "notificaciones_mensajes_total",
		Help: "Mensajes de pedido.estado_cambiado procesados, según cómo terminaron.",
	}, []string{"resultado"})

	envio = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "notificaciones_envio_duration_seconds",
		Help:    "Cuánto tarda mandar un mail (con o sin éxito).",
		Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10},
	})

	conectado = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "notificaciones_rabbitmq_conectado",
		Help: "1 si el worker está conectado a RabbitMQ y escuchando la cola; 0 si no.",
	})
)

// Mensaje cuenta un mensaje según cómo terminó (Enviado, Duplicado, Reintento, DLQ o Invalido).
func Mensaje(resultado string) { mensajes.WithLabelValues(resultado).Inc() }

// Envio mide cuánto tardó mandar un mail.
func Envio(desde time.Time) { envio.Observe(time.Since(desde).Seconds()) }

// Conectado anota si el worker está escuchando la cola.
func Conectado(si bool) {
	if si {
		conectado.Set(1)
	} else {
		conectado.Set(0)
	}
}

// Handler responde GET /metrics.
func Handler() http.Handler { return promhttp.Handler() }
