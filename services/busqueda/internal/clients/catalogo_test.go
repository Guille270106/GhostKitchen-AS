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

// clienteRapido arma un cliente de Catálogo contra un servidor de prueba, con esperas cortas
// entre reintentos para que el test no tarde.
func clienteRapido(url string) *Catalogo {
	c := NuevoCatalogo(url)
	c.reintento = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	return c
}

// Un 4xx es definitivo: se pide una sola vez (L22).
func TestUn4xxNoSeReintenta(t *testing.T) {
	for _, estado := range []int{http.StatusBadRequest, http.StatusNotFound} {
		var llamadas atomic.Int32
		falso := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			llamadas.Add(1)
			w.WriteHeader(estado)
		}))
		_, err := clienteRapido(falso.URL).Cocinas(context.Background())
		falso.Close()
		if err == nil || !errors.Is(err, errDefinitivo) || llamadas.Load() != 1 {
			t.Errorf("%d: esperaba un solo intento y un error definitivo, hubo %d intentos (%v)", estado, llamadas.Load(), err)
		}
	}
}

// Lo pasajero (5xx o 429) sí se reintenta: 1 intento + 3 reintentos.
func TestLoPasajeroSeReintenta(t *testing.T) {
	for _, estado := range []int{http.StatusServiceUnavailable, http.StatusTooManyRequests} {
		var llamadas atomic.Int32
		falso := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			llamadas.Add(1)
			w.WriteHeader(estado)
		}))
		_, err := clienteRapido(falso.URL).Cocinas(context.Background())
		falso.Close()
		if err == nil || errors.Is(err, errDefinitivo) || llamadas.Load() != 4 {
			t.Errorf("%d: esperaba 4 intentos, hubo %d (%v)", estado, llamadas.Load(), err)
		}
	}
}

// Si un reintento sale bien, se usa esa respuesta.
func TestUnaFallaPasajeraYDespuesResponde(t *testing.T) {
	var llamadas atomic.Int32
	falso := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if llamadas.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{"items":[{"id":"cerro","nombre":"Cocina Cerro"}]}`))
	}))
	defer falso.Close()
	cocinas, err := clienteRapido(falso.URL).Cocinas(context.Background())
	if err != nil || len(cocinas) != 1 || llamadas.Load() != 2 {
		t.Fatal(err, cocinas, llamadas.Load())
	}
}
