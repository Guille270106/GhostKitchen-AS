package catalogo

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"ghostkitchen/pedidos/internal/adapters/correlacion"
	"ghostkitchen/pedidos/internal/domain"
)

// Las cocinas de una localidad: solo las de esa localidad, activas o no.
func TestCocinasDeLocalidad(t *testing.T) {
	falso := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/cocinas" || r.Header.Get("X-Correlation-Id") != "prueba-1" {
			t.Errorf("solicitud inesperada: %s %q", r.URL.Path, r.Header.Get("X-Correlation-Id"))
		}
		w.Write([]byte(`{"items":[
			{"id":"cerro","nombre":"Cocina Cerro","localidad":"Cerro de las Rosas","activa":true},
			{"id":"nueva-cordoba","nombre":"Cocina Nueva Córdoba","localidad":"Nueva Córdoba"},
			{"id":"cerro-2","nombre":"Cocina Cerro 2","localidad":"Cerro de las Rosas","activa":false}]}`))
	}))
	defer falso.Close()
	ctx := correlacion.Con(context.Background(), "prueba-1")

	cocinas, err := Nuevo(falso.URL).CocinasDeLocalidad(ctx, "Cerro de las Rosas")
	if err != nil || len(cocinas) != 2 || cocinas[0].ID != "cerro" || cocinas[1].ID != "cerro-2" || cocinas[0].Nombre != "Cocina Cerro" {
		t.Fatal(err, cocinas)
	}
	// Una localidad sin cocinas devuelve la lista vacía, no un error.
	if cocinas, err := Nuevo(falso.URL).CocinasDeLocalidad(ctx, "Villa Belgrano"); err != nil || len(cocinas) != 0 {
		t.Fatal(err, cocinas)
	}
}

// Catálogo caído o con una respuesta que no se entiende: se reintenta una vez y se avisa
// que no está disponible (503), nunca una lista vacía inventada.
func TestCocinasConCatalogoCaido(t *testing.T) {
	for nombre, responder := range map[string]func(http.ResponseWriter){
		"503":       func(w http.ResponseWriter) { w.WriteHeader(http.StatusServiceUnavailable) },
		"JSON roto": func(w http.ResponseWriter) { w.Write([]byte(`{"items": [`)) },
	} {
		var llamadas atomic.Int32
		falso := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			llamadas.Add(1)
			responder(w)
		}))
		_, err := Nuevo(falso.URL).CocinasDeLocalidad(context.Background(), "Cerro de las Rosas")
		falso.Close()
		if !errors.Is(err, domain.ErrCatalogoCaido) || llamadas.Load() != 2 {
			t.Errorf("%s: esperaba ErrCatalogoCaido con un reintento, obtuvo %v en %d llamadas", nombre, err, llamadas.Load())
		}
	}
}

// Sin conexión con Catálogo: también es "no disponible".
func TestCocinasSinConexion(t *testing.T) {
	falso := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := falso.URL
	falso.Close() // ya no escucha nadie en esa dirección
	if _, err := Nuevo(url).CocinasDeLocalidad(context.Background(), "Cerro de las Rosas"); !errors.Is(err, domain.ErrCatalogoCaido) {
		t.Fatal(err)
	}
}

// Un plato con un JSON que no se entiende es "Catálogo no disponible", no un plato vacío.
func TestPlatoConRespuestaRota(t *testing.T) {
	falso := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`no es json`))
	}))
	defer falso.Close()
	if _, err := Nuevo(falso.URL).ObtenerPlato(context.Background(), "p17"); !errors.Is(err, domain.ErrCatalogoCaido) {
		t.Fatal(err)
	}
}
