package solr

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ghostkitchen/busqueda/internal/domain"
)

// Los errores de Solr terminan en el log de la solicitud y en la traza. No pueden llevar
// las coordenadas ni el texto que mandó el consumidor (ADR-011).

func TestErrorDeConexionNoTieneLaURL(t *testing.T) {
	repo := NuevoRepositorio(NuevoCliente("http://127.0.0.1:1/solr", 200*time.Millisecond))
	_, _, err := repo.SedeMasCercana(context.Background(), domain.Ubicacion{Lat: -31.4262, Lng: -64.1883})
	if !errors.Is(err, domain.ErrNoDisponible) {
		t.Fatalf("esperaba ErrNoDisponible, obtuve %v", err)
	}
	for _, prohibido := range []string{"-31.4262", "-64.1883", "pt=", "select?", "127.0.0.1:1/solr/cocinas/select"} {
		if strings.Contains(err.Error(), prohibido) {
			t.Fatalf("el error contiene %q: %v", prohibido, err)
		}
	}
	if !strings.Contains(err.Error(), "cocinas/select") {
		t.Fatalf("el error tiene que decir qué operación falló: %v", err)
	}
}

func TestErrorDeSolrNoTieneElTextoBuscado(t *testing.T) {
	repo, _ := nuevoSolrFalso(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = io.WriteString(w, `{"error":{"msg":"org.apache.solr.search.SyntaxError: Cannot parse 'hamburguesa secreta(': Encountered \"<EOF>\""}}`)
	})
	_, _, err := repo.BuscarPlatos(context.Background(), domain.FiltroPlatos{SedeID: "nueva-cordoba", Texto: "hamburguesa secreta(", Pagina: 1, TamPagina: 10})
	if !errors.Is(err, domain.ErrNoDisponible) {
		t.Fatalf("esperaba ErrNoDisponible, obtuve %v", err)
	}
	if strings.Contains(err.Error(), "hamburguesa") {
		t.Fatalf("el error contiene el texto buscado: %v", err)
	}
	if !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "Cannot parse") {
		t.Fatalf("el error tiene que conservar el código y el motivo: %v", err)
	}
}

// El esquema y las escrituras son trabajo de fondo: aguantan más que el límite de una
// consulta, que tiene que seguir siendo corto para no dejar colgada una solicitud.
func TestEsquemaTieneMasTiempoQueUnaConsulta(t *testing.T) {
	srv := nuevoSolrLento(t, 300*time.Millisecond)
	cli := NuevoCliente(srv, 100*time.Millisecond)
	if err := cli.Esquema(context.Background(), CorePlatos, map[string]any{"add-field": []any{}}); err != nil {
		t.Fatalf("el esquema tenía que esperar más que el límite de consulta: %v", err)
	}
	if _, err := cli.Select(context.Background(), CorePlatos, nil); !errors.Is(err, domain.ErrNoDisponible) {
		t.Fatalf("la consulta tenía que cortar a los 100 ms: %v", err)
	}
}

func nuevoSolrLento(t *testing.T, demora time.Duration) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(demora)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"responseHeader":{"status":0},"response":{"numFound":0,"docs":[]}}`)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/solr"
}
