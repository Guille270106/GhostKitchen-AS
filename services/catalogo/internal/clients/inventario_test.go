package clients

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"ghostkitchen/catalogo/internal/domain"
)

func inventarioFalso(t *testing.T, h http.HandlerFunc) (*Inventario, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { n.Add(1); h(w, r) }))
	t.Cleanup(srv.Close)
	return NuevoInventario(srv.URL), &n
}

func TestTieneReceta(t *testing.T) {
	inv, _ := inventarioFalso(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cocina/platos/p-1/receta" || r.Header.Get("X-Rol") != "cocina" || r.Header.Get("X-Cocina-Id") != "nc" || r.Header.Get("X-Usuario-Id") != "k1" {
			t.Errorf("solicitud inesperada: %s %v", r.URL.Path, r.Header)
		}
		_, _ = w.Write([]byte(`{"platoId":"p-1","items":[{"ingredienteId":"i1","cantidad":1}]}`))
	})
	if ok, err := inv.TieneReceta(context.Background(), "k1", "nc", "p-1", ""); !ok || err != nil {
		t.Fatalf("%v %v", ok, err)
	}
}

func TestSinRecetaNoSeReintenta(t *testing.T) {
	inv, n := inventarioFalso(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	if ok, err := inv.TieneReceta(context.Background(), "k1", "nc", "p-1", ""); ok || err != nil || n.Load() != 1 {
		t.Fatalf("404 es \"no tiene receta\": %v %v (%d pedidos)", ok, err, n.Load())
	}
}

func TestInventarioCaidoNoEsSinReceta(t *testing.T) {
	inv, n := inventarioFalso(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) })
	if _, err := inv.TieneReceta(context.Background(), "k1", "nc", "p-1", ""); !errors.Is(err, domain.ErrInventarioNoDisponible) || n.Load() != 2 {
		t.Fatalf("5xx: un reintento y ErrInventarioNoDisponible: %v (%d pedidos)", err, n.Load())
	}
	apagado := NuevoInventario("http://127.0.0.1:1")
	if _, err := apagado.TieneReceta(context.Background(), "k1", "nc", "p-1", ""); !errors.Is(err, domain.ErrInventarioNoDisponible) {
		t.Fatalf("apagado: %v", err)
	}
}
