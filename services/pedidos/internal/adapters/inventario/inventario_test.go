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
