package catalogo

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"ghostkitchen/pedidos/internal/adapters/correlacion"
	"ghostkitchen/pedidos/internal/domain"
)

func TestObtenerPlato(t *testing.T) {
	// Un Catálogo falso: p17 existe, p64 está desactivado (404).
	falso := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Correlation-Id") != "prueba-1" {
			t.Error("no se pasó el correlation id")
		}
		if r.URL.Path == "/platos/p17" {
			w.Write([]byte(`{"id":"p17","cocinaId":"cerro","restauranteId":"r5","nombre":"Burger","precio":12000,"activo":true}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer falso.Close()
	c := Nuevo(falso.URL)
	ctx := correlacion.Con(context.Background(), "prueba-1")

	plato, err := c.ObtenerPlato(ctx, "p17")
	if err != nil || plato.Precio != 12000 || plato.CocinaID != "cerro" || !plato.Activo {
		t.Fatal(err, plato)
	}
	if _, err := c.ObtenerPlato(ctx, "p64"); !errors.Is(err, domain.ErrPlatoNoEncontrado) {
		t.Fatal(err)
	}
}

func TestCatalogoCaidoReintentaUnaVezY503(t *testing.T) {
	var llamadas atomic.Int32
	falso := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llamadas.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer falso.Close()
	if _, err := Nuevo(falso.URL).ObtenerPlato(context.Background(), "p17"); !errors.Is(err, domain.ErrCatalogoCaido) {
		t.Fatal(err)
	}
	if llamadas.Load() != 2 {
		t.Fatal("tiene que reintentar una sola vez:", llamadas.Load())
	}
}

func TestCatalogoLentoCortaPorTimeout(t *testing.T) {
	lento := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(700 * time.Millisecond)
	}))
	defer lento.Close()
	inicio := time.Now()
	if _, err := Nuevo(lento.URL).ObtenerPlato(context.Background(), "p17"); !errors.Is(err, domain.ErrCatalogoCaido) {
		t.Fatal(err)
	}
	if time.Since(inicio) > 1500*time.Millisecond {
		t.Fatal("dos intentos de 500 ms no pueden tardar tanto:", time.Since(inicio))
	}
}
