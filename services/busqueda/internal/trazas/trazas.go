// Package trazas configura OpenTelemetry: cada solicitud genera una traza que sigue su
// recorrido por el gateway, Búsqueda, Solr y Catálogo, y se ve en Jaeger.
package trazas

import (
	"context"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// Servicio es el nombre con el que Búsqueda aparece en Jaeger.
const Servicio = "busqueda"

// Tracer es el que usan los paquetes del servicio para crear sus propios spans.
func Tracer() trace.Tracer { return otel.Tracer("ghostkitchen/" + Servicio) }

// Iniciar configura la propagación W3C (traceparent) y, si está definida
// OTEL_EXPORTER_OTLP_ENDPOINT (ej. http://jaeger:4318), el envío de spans por OTLP/HTTP.
// Sin esa variable las trazas igual se propagan entre servicios, pero no se exportan.
// Devuelve la función que hay que llamar al apagar para mandar los spans pendientes.
func Iniciar(ctx context.Context) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))

	if strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")) == "" {
		return func(context.Context) error { return nil }, nil
	}
	exp, err := otlptracehttp.New(ctx) // lee OTEL_EXPORTER_OTLP_ENDPOINT
	if err != nil {
		return nil, err
	}
	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL, semconv.ServiceName(Servicio)))
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		// En desarrollo guardamos todas las trazas, también las que el gateway no muestreó
		// (las solicitudes con parámetros, ver gateway/nginx.conf). En producción se usaría
		// un porcentaje (TraceIDRatioBased).
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

// Middleware crea el span de servidor de cada solicitud HTTP (continúa la traza del gateway
// si viene un traceparent). No traza /health ni /ready para no llenar Jaeger de chequeos.
// El span lleva la ruta como plantilla (ej. /v1/sedes/:sedeId/platos), sin parámetros.
func Middleware() gin.HandlerFunc {
	return otelgin.Middleware(Servicio, otelgin.WithFilter(func(r *http.Request) bool {
		return r.URL.Path != "/health" && r.URL.Path != "/ready"
	}))
}

// MarcarCorrelacion agrega el X-Correlation-Id al span actual, para buscar una traza en
// Jaeger a partir del id que aparece en los logs o que nos pasa el otro grupo.
func MarcarCorrelacion(ctx context.Context, id string) {
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("ghostkitchen.correlation_id", id))
}

// IDs devuelve el trace_id y span_id del contexto, para escribirlos en los logs.
func IDs(ctx context.Context) (traceID, spanID string) {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return "", ""
	}
	return sc.TraceID().String(), sc.SpanID().String()
}
