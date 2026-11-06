package trazas_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"ghostkitchen/busqueda/internal/clients"
	"ghostkitchen/busqueda/internal/domain"
	"ghostkitchen/busqueda/internal/repositories/solr"
	"ghostkitchen/busqueda/internal/router"
)

// grabar instala un TracerProvider que guarda los spans en memoria.
func grabar(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	anterior := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTracerProvider(anterior) })
	return rec
}

type rutaDePrueba struct{}

func (rutaDePrueba) Registrar(r gin.IRoutes) {
	r.GET("/v1/platos/:platoId", func(c *gin.Context) { c.Status(204) })
	r.GET("/health", func(c *gin.Context) { c.Status(200) })
}

func TestSolicitudContinuaLaTrazaDelGatewayYLaEscribeEnElLog(t *testing.T) {
	rec := grabar(t)
	var logs bytes.Buffer
	srv := router.Nuevo(slog.New(slog.NewJSONHandler(&logs, nil)), rutaDePrueba{})

	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	req := httptest.NewRequest("GET", "/v1/platos/p1", nil)
	req.Header.Set("traceparent", "00-"+traceID+"-00f067aa0ba902b7-01") // lo manda NGINX
	req.Header.Set("X-Correlation-Id", "demo-1")
	srv.ServeHTTP(httptest.NewRecorder(), req)

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("esperaba 1 span, hubo %d", len(spans))
	}
	s := spans[0]
	if s.SpanContext().TraceID().String() != traceID {
		t.Fatalf("no continuó la traza del gateway: %s", s.SpanContext().TraceID())
	}
	if s.Name() != "/v1/platos/:platoId" {
		t.Fatalf("el span tiene que llamarse como la plantilla de la ruta, no %q", s.Name())
	}
	var tieneCorrelacion bool
	for _, a := range s.Attributes() {
		if a.Key == "ghostkitchen.correlation_id" && a.Value.AsString() == "demo-1" {
			tieneCorrelacion = true
		}
	}
	if !tieneCorrelacion {
		t.Fatal("el span tiene que llevar el X-Correlation-Id")
	}

	var linea map[string]any
	if err := json.Unmarshal(logs.Bytes(), &linea); err != nil {
		t.Fatalf("log no es JSON: %s", logs.String())
	}
	if linea["trace_id"] != traceID || linea["correlation_id"] != "demo-1" || linea["span_id"] == "" {
		t.Fatalf("el log tiene que tener trace_id, span_id y correlation_id: %v", linea)
	}
}

func TestNoSeTrazanLosChequeosDeSalud(t *testing.T) {
	rec := grabar(t)
	srv := router.Nuevo(slog.New(slog.NewJSONHandler(io.Discard, nil)), rutaDePrueba{})
	srv.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/health", nil))
	if n := len(rec.Ended()); n != 0 {
		t.Fatalf("/health generó %d spans", n)
	}
}

func TestSpanDeSolrSinParametros(t *testing.T) {
	rec := grabar(t)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"response":{"numFound":0,"docs":[]}}`)
	}))
	defer fake.Close()
	repo := solr.NuevoRepositorio(solr.NuevoCliente(fake.URL+"/solr", time.Second))
	_, _, _ = repo.SedeMasCercana(context.Background(), domain.Ubicacion{Lat: -31.4262, Lng: -64.1883})

	spans := rec.Ended()
	if len(spans) != 1 || spans[0].Name() != "solr cocinas/select" {
		t.Fatalf("spans: %v", spans)
	}
	for _, a := range spans[0].Attributes() {
		if strings.Contains(a.Value.Emit(), "31.4262") {
			t.Fatalf("el span registró coordenadas en %s", a.Key)
		}
	}
}

func TestLaLlamadaACatalogoPropagaLaTraza(t *testing.T) {
	rec := grabar(t)
	var traceparent string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceparent = r.Header.Get("traceparent")
		_, _ = io.WriteString(w, `{"items":[]}`)
	}))
	defer fake.Close()

	ctx, padre := otel.Tracer("test").Start(context.Background(), "reconstruir")
	_, err := clients.NuevoCatalogo(fake.URL).Cocinas(ctx)
	padre.End()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(traceparent, padre.SpanContext().TraceID().String()) {
		t.Fatalf("Catálogo no recibió la traza: traceparent=%q", traceparent)
	}
	var nombres []string
	for _, s := range rec.Ended() {
		nombres = append(nombres, s.Name())
	}
	if !strings.Contains(strings.Join(nombres, ","), "catalogo GET /internal/cocinas") {
		t.Fatalf("falta el span de la llamada a Catálogo: %v", nombres)
	}
}
