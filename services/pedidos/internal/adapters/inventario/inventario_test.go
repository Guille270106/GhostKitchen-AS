package inventario

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/ports"
)

func TestReservarMandaClaveYLeeLaReserva(t *testing.T) {
	falso := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var cuerpo map[string]any
		_ = json.NewDecoder(r.Body).Decode(&cuerpo)
		if r.Method != "POST" || r.URL.Path != "/reservas" || r.Header.Get("Idempotency-Key") != "car-1-1" || cuerpo["carritoId"] != "car-1" {
			t.Errorf("solicitud inesperada: %s %s %v", r.Method, r.URL.Path, cuerpo)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"res-9","estado":"ACTIVA","venceEn":"2026-11-08T22:10:00Z"}`))
	}))
	defer falso.Close()
	id, vence, err := Nuevo(falso.URL).Reservar(context.Background(), "car-1-1", "car-1", "cerro",
		[]ports.ItemReserva{{PlatoID: "p17", Cantidad: 2}})
	if err != nil || id != "res-9" || vence.IsZero() {
		t.Fatal(err, id, vence)
	}
}

func TestTraduceLosErroresDeInventario(t *testing.T) {
	casos := map[string]error{
		"SIN_STOCK":              domain.ErrSinStock,
		"RESERVA_VENCIDA":        domain.ErrCarritoVencido,
		"NO_ENCONTRADO":          domain.ErrPlatoNoDisponible,
		"IDEMPOTENCIA_CONFLICTO": domain.ErrCarritoModificado,
	}
	for codigo, esperado := range casos {
		falso := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(`{"code":"` + codigo + `"}`))
		}))
		err := Nuevo(falso.URL).Ajustar(context.Background(), "k", "res-1", []ports.ItemReserva{{PlatoID: "p17", Cantidad: 1}})
		falso.Close()
		if !errors.Is(err, esperado) {
			t.Errorf("%s: esperaba %v, obtuvo %v", codigo, esperado, err)
		}
	}
}

func TestInventarioCaidoReintentaConLaMismaClave(t *testing.T) {
	var llamadas atomic.Int32
	var claves []string
	falso := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claves = append(claves, r.Header.Get("Idempotency-Key"))
		// El primer intento falla; el segundo sale bien.
		if llamadas.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{"id":"res-1"}`))
	}))
	defer falso.Close()
	if err := Nuevo(falso.URL).Liberar(context.Background(), "res-1-liberar", "res-1"); err != nil {
		t.Fatal(err)
	}
	if len(claves) != 2 || claves[0] != claves[1] {
		t.Fatal("el reintento tiene que usar la misma clave:", claves)
	}
}

func TestInventarioSiempreCaidoDa503(t *testing.T) {
	falso := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer falso.Close()
	if err := Nuevo(falso.URL).Liberar(context.Background(), "k", "res-1"); !errors.Is(err, domain.ErrInventarioCaido) {
		t.Fatal(err)
	}
}

func TestConsumirMandaElPedidoYLaClave(t *testing.T) {
	falso := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var cuerpo map[string]any
		_ = json.NewDecoder(r.Body).Decode(&cuerpo)
		if r.Method != "POST" || r.URL.Path != "/reservas/res-1/consumir" || r.Header.Get("Idempotency-Key") != "conf-1-descontar" || cuerpo["pedidoId"] != "ped-1" {
			t.Errorf("solicitud inesperada: %s %s %v", r.Method, r.URL.Path, cuerpo)
		}
		w.Write([]byte(`{}`))
	}))
	defer falso.Close()
	if err := Nuevo(falso.URL).Consumir(context.Background(), "conf-1-descontar", "res-1", "ped-1"); err != nil {
		t.Fatal(err)
	}
}

// Después de 10 fallas seguidas el circuito se abre: Consumir responde al instante sin llamar.
func TestCircuitBreakerSeAbreConInventarioCaido(t *testing.T) {
	var llamadas atomic.Int32
	falso := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llamadas.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer falso.Close()
	cliente := Nuevo(falso.URL)
	for i := 0; i < 10; i++ {
		_ = cliente.Consumir(context.Background(), "k", "res-1", "ped-1")
	}
	if llamadas.Load() != 10 {
		t.Fatal("las primeras 10 llegan a Inventario:", llamadas.Load())
	}
	if err := cliente.Consumir(context.Background(), "k", "res-1", "ped-1"); !errors.Is(err, domain.ErrInventarioCaido) {
		t.Fatal(err)
	}
	if llamadas.Load() != 10 {
		t.Fatal("con el circuito abierto no se llama a Inventario:", llamadas.Load())
	}
}

// Que la reserva haya vencido es una respuesta del negocio: no abre el circuito.
func TestReservaVencidaNoAbreElCircuito(t *testing.T) {
	falso := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"code":"RESERVA_VENCIDA"}`))
	}))
	defer falso.Close()
	cliente := Nuevo(falso.URL)
	for i := 0; i < 15; i++ {
		if err := cliente.Consumir(context.Background(), "k", "res-1", "ped-1"); !errors.Is(err, domain.ErrCarritoVencido) {
			t.Fatalf("llamada %d: %v", i, err)
		}
	}
}

func TestDevolverMandaLaClaveALaRutaDelPedido(t *testing.T) {
	falso := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var cuerpo map[string]any
		_ = json.NewDecoder(r.Body).Decode(&cuerpo)
		if r.Method != "POST" || r.URL.Path != "/consumos/ped-1/devolver" || r.Header.Get("Idempotency-Key") != "ped-1-devolver" {
			t.Errorf("solicitud inesperada: %s %s %v", r.Method, r.URL.Path, cuerpo)
		}
		w.Write([]byte(`{}`))
	}))
	defer falso.Close()
	if err := Nuevo(falso.URL).Devolver(context.Background(), "ped-1-devolver", "ped-1"); err != nil {
		t.Fatal(err)
	}
}

func TestDevolverConInventarioCaido(t *testing.T) {
	falso := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer falso.Close()
	if err := Nuevo(falso.URL).Devolver(context.Background(), "k", "ped-1"); !errors.Is(err, domain.ErrInventarioCaido) {
		t.Fatal(err)
	}
}

// Si Inventario no tiene un descuento de ese pedido (404), no hay nada que devolver.
func TestDevolverSinConsumoEsNoEncontrado(t *testing.T) {
	for _, cuerpo := range []string{`{"code":"NO_ENCONTRADO"}`, `{"code":"CONSUMO_NO_ENCONTRADO"}`, ``} {
		falso := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(cuerpo))
		}))
		err := Nuevo(falso.URL).Devolver(context.Background(), "k", "ped-1")
		falso.Close()
		if !errors.Is(err, domain.ErrNoEncontrado) {
			t.Errorf("%q: esperaba ErrNoEncontrado, obtuvo %v", cuerpo, err)
		}
	}
}
