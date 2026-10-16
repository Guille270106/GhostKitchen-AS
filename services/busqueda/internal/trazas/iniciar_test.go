package trazas

import (
	"context"
	"testing"
)

// Con un endpoint configurado, Iniciar arma el exportador sin conectarse todavía y sin
// errores de configuración (por ejemplo, versiones de esquema incompatibles).
func TestIniciarConExportador(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1")
	cerrar, err := Iniciar(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, span := Tracer().Start(context.Background(), "prueba")
	if !span.SpanContext().IsSampled() {
		t.Fatal("el span tiene que guardarse")
	}
	span.End()
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	_ = cerrar(ctx) // no hay Jaeger: no importa si no llega
}

func TestIniciarSinExportador(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	if _, err := Iniciar(context.Background()); err != nil {
		t.Fatal(err)
	}
}
