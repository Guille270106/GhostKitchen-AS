package clients

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func catalogoFalso(t *testing.T, h http.HandlerFunc) (*Catalogo, *atomic.Int32) {
	t.Helper()
	var pedidos atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pedidos.Add(1)
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return NuevoCatalogo(srv.URL), &pedidos
}

func TestCocinaExiste(t *testing.T) {
	c, _ := catalogoFalso(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/cocinas/nueva-cordoba" || r.Header.Get("X-Correlation-Id") != "cid-1" {
			t.Errorf("solicitud inesperada: %s %v", r.URL.Path, r.Header)
		}
		_, _ = w.Write([]byte(`{"id":"nueva-cordoba","localidad":"Nueva Córdoba","activa":true,"restaurantes":[]}`))
	})
	cocina, err := c.Cocina(context.Background(), "nueva-cordoba", "cid-1")
	if err != nil || cocina.Localidad != "Nueva Córdoba" || !cocina.Activa {
		t.Fatalf("%+v %v", cocina, err)
	}
}

func TestCocina404NoSeReintenta(t *testing.T) {
	c, pedidos := catalogoFalso(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	if _, err := c.Cocina(context.Background(), "rio-cuarto", ""); !errors.Is(err, ErrCocinaNoExiste) {
		t.Fatalf("esperaba ErrCocinaNoExiste: %v", err)
	}
	if pedidos.Load() != 1 {
		t.Fatalf("un 404 es una respuesta: no se reintenta (%d pedidos)", pedidos.Load())
	}
}

func TestCocina5xxSeReintentaUnaVez(t *testing.T) {
	c, pedidos := catalogoFalso(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) })
	if _, err := c.Cocina(context.Background(), "cerro", ""); !errors.Is(err, ErrCatalogoNoDisponible) {
		t.Fatalf("esperaba ErrCatalogoNoDisponible: %v", err)
	}
	if pedidos.Load() != 2 {
		t.Fatalf("esperaba 2 intentos, hubo %d", pedidos.Load())
	}
}

func TestCocinaLentaCortaYNoSeTomaComoInexistente(t *testing.T) {
	c, pedidos := catalogoFalso(t, func(w http.ResponseWriter, r *http.Request) { time.Sleep(TimeoutIntento + 200*time.Millisecond) })
	inicio := time.Now()
	_, err := c.Cocina(context.Background(), "cerro", "")
	if !errors.Is(err, ErrCatalogoNoDisponible) || errors.Is(err, ErrCocinaNoExiste) {
		t.Fatalf("sin respuesta no es \"no existe\": %v", err)
	}
	if d := time.Since(inicio); d > 2*time.Second || pedidos.Load() != 2 {
		t.Fatalf("dos intentos de 800 ms tienen que terminar antes de 2 s: %v, %d pedidos", d, pedidos.Load())
	}
}

func TestCatalogoApagado(t *testing.T) {
	c := NuevoCatalogo("http://127.0.0.1:1")
	if _, err := c.Cocina(context.Background(), "cerro", ""); !errors.Is(err, ErrCatalogoNoDisponible) {
		t.Fatalf("esperaba ErrCatalogoNoDisponible: %v", err)
	}
}
