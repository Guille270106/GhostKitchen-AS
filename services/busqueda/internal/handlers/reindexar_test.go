package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ghostkitchen/busqueda/internal/handlers"
	"ghostkitchen/busqueda/internal/indexer"
	"ghostkitchen/busqueda/internal/router"
)

func servidorReindexar(excl *indexer.Exclusiva) http.Handler {
	return router.Nuevo(slog.New(slog.NewJSONHandler(io.Discard, nil)), handlers.NuevoReindexar(excl))
}

func reindexar(srv http.Handler, ctx context.Context) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/internal/reindexar", nil).WithContext(ctx)
	req.Header.Set("X-Correlation-Id", "prueba-reindexar")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func leerProblema(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if ct := w.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("Content-Type %q", ct)
	}
	var p map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReindexarBien(t *testing.T) {
	srv := servidorReindexar(indexer.NuevaExclusiva(func(context.Context) (indexer.Resultado, error) {
		return indexer.Resultado{Sedes: 4, Platos: 64}, nil
	}))
	w := reindexar(srv, context.Background())
	if w.Code != 200 || !json.Valid(w.Body.Bytes()) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestReindexarSinCatalogo(t *testing.T) {
	w := reindexar(servidorReindexar(nil), context.Background())
	if p := leerProblema(t, w); w.Code != 409 || p["code"] != handlers.CodigoSinCatalogo {
		t.Fatalf("%d %v", w.Code, p)
	}
}

func TestReindexarFallaConParcialYRetryAfter(t *testing.T) {
	srv := servidorReindexar(indexer.NuevaExclusiva(func(context.Context) (indexer.Resultado, error) {
		return indexer.Resultado{Sedes: 4, Platos: 100}, errors.New("catálogo respondió 503")
	}))
	w := reindexar(srv, context.Background())
	p := leerProblema(t, w)
	parcial, _ := p["parcial"].(map[string]any)
	if w.Code != 503 || w.Header().Get("Retry-After") == "" || p["code"] != handlers.CodigoNoDisponible ||
		p["correlationId"] != "prueba-reindexar" || parcial["platos"] != float64(100) {
		t.Fatalf("%d %v", w.Code, p)
	}
}

// Mientras corre una (por ejemplo, la del arranque), la manual responde 409 enseguida.
func TestReindexarEnCurso(t *testing.T) {
	libre := make(chan struct{})
	excl := indexer.NuevaExclusiva(func(context.Context) (indexer.Resultado, error) {
		<-libre
		return indexer.Resultado{}, nil
	})
	go func() { _, _ = excl.Esperar(context.Background()) }()
	time.Sleep(20 * time.Millisecond)
	w := reindexar(servidorReindexar(excl), context.Background())
	close(libre)
	if p := leerProblema(t, w); w.Code != 409 || p["code"] != handlers.CodigoReconstruccionEnCurso || w.Header().Get("Retry-After") == "" {
		t.Fatalf("%d %v", w.Code, p)
	}
}

// Si quien la pidió corta la conexión, la reconstrucción sigue: tiene su propio contexto.
func TestReindexarNoSeCortaSiSeVaElCliente(t *testing.T) {
	cortado := make(chan error, 1)
	srv := servidorReindexar(indexer.NuevaExclusiva(func(ctx context.Context) (indexer.Resultado, error) {
		time.Sleep(30 * time.Millisecond)
		cortado <- ctx.Err()
		return indexer.Resultado{}, nil
	}))
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(5 * time.Millisecond); cancel() }()
	reindexar(srv, ctx)
	if err := <-cortado; err != nil {
		t.Fatalf("la reconstrucción se cortó con el cliente: %v", err)
	}
}
