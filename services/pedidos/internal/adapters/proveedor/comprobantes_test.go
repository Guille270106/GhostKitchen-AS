package proveedor

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/ports"
)

var solicitud = ports.SolicitudComprobante{
	Email:  "cliente@ejemplo.com",
	Lineas: []domain.LineaComprobante{{Descripcion: "Burger x2", Importe: 24000}},
}

func TestEmitirMandaLoQuePideSuContrato(t *testing.T) {
	falso := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var cuerpo map[string]any
		_ = json.NewDecoder(r.Body).Decode(&cuerpo)
		items, _ := cuerpo["items"].([]any)
		if r.Method != "POST" || r.URL.Path != "/v1/comprobantes" || r.Header.Get("X-API-Key") != "clave-1" ||
			r.Header.Get("Idempotency-Key") != "pedido-ped-1" || cuerpo["email"] != "cliente@ejemplo.com" || len(cuerpo) != 2 || len(items) != 1 {
			t.Errorf("solicitud inesperada: %s %s %v", r.Method, r.URL.Path, cuerpo)
		}
		if linea, _ := items[0].(map[string]any); linea["descripcion"] != "Burger x2" || linea["precio"] != 24000.0 {
			t.Errorf("línea: %v", linea)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"6b0c1d2e-3f40-4a5b-8c6d-7e8f90a1b2c3","numero":"B-0001-00000042","estadoEnvio":"PENDIENTE"}`))
	}))
	defer falso.Close()
	c, err := Nuevo(falso.URL, "clave-1").Emitir(context.Background(), "pedido-ped-1", solicitud)
	if err != nil || c.ID != "6b0c1d2e-3f40-4a5b-8c6d-7e8f90a1b2c3" || c.Numero != "B-0001-00000042" {
		t.Fatal(err, c)
	}
}

func TestTraduceSusRespuestas(t *testing.T) {
	casos := []struct {
		estado   int
		esperado error
	}{
		{200, nil}, // misma clave y mismo cuerpo: devuelven la que ya estaba
		{400, domain.ErrProveedorRechazo},
		{409, domain.ErrProveedorRechazo},
		{422, domain.ErrProveedorRechazo},
		{401, domain.ErrProveedorCaido},
		{429, domain.ErrProveedorCaido},
		{503, domain.ErrProveedorCaido},
	}
	for _, c := range casos {
		falso := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(c.estado)
			w.Write([]byte(`{"id":"cmp-1","numero":"B-1","type":"x","title":"t","status":1}`))
		}))
		_, err := Nuevo(falso.URL, "k").Emitir(context.Background(), "pedido-1", solicitud)
		falso.Close()
		if c.esperado == nil && err != nil || c.esperado != nil && !errors.Is(err, c.esperado) {
			t.Errorf("%d: esperaba %v, obtuvo %v", c.estado, c.esperado, err)
		}
	}
}

func TestSinRespuestaATiempoEsCaido(t *testing.T) {
	falso := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(time.Second):
		case <-r.Context().Done():
		}
	}))
	defer falso.Close()
	ctx, cancelar := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancelar()
	if _, err := Nuevo(falso.URL, "k").Emitir(ctx, "pedido-1", solicitud); !errors.Is(err, domain.ErrProveedorCaido) {
		t.Fatal(err)
	}
}

// Después de 10 fallas el circuito se abre: no los llamamos más por 10 s.
func TestCircuitBreakerConElProveedorCaido(t *testing.T) {
	var llamadas atomic.Int32
	falso := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llamadas.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer falso.Close()
	c := Nuevo(falso.URL, "k")
	for i := 0; i < 12; i++ {
		if _, err := c.Emitir(context.Background(), "pedido-1", solicitud); !errors.Is(err, domain.ErrProveedorCaido) {
			t.Fatal(err)
		}
	}
	if llamadas.Load() != 10 {
		t.Fatal("con el circuito abierto no se los llama:", llamadas.Load())
	}
}
