package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSobreSigueElContrato(t *testing.T) {
	p := platoValido()
	p.Version = 3
	ev := EventoPlato(PlatoActualizado, p, "req-1", time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC))
	b, err := ev.Sobre()
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	for _, campo := range []string{"evento_id", "tipo", "version", "ocurrido_en", "correlation_id", "agregado_id", "secuencia", "datos"} {
		if _, ok := m[campo]; !ok {
			t.Errorf("falta %q en el sobre: %s", campo, b)
		}
	}
	if m["tipo"] != "plato.actualizado" || m["agregado_id"] != "p1" || m["secuencia"].(float64) != 3 || m["ocurrido_en"] != "2026-11-09T12:00:00Z" {
		t.Fatalf("sobre inesperado: %s", b)
	}
	datos := m["datos"].(map[string]any)
	if datos["id"] != "p1" || datos["precio"].(float64) != 9500 || datos["cocinaId"] != "nc" {
		t.Fatalf("datos tiene que ser el plato como en /internal/platos: %v", datos)
	}
}

func TestTipoCambio(t *testing.T) {
	if TipoCambio(true, false, CocinaActualizada, CocinaDesactivada) != CocinaDesactivada {
		t.Fatal("activa → inactiva es desactivada")
	}
	if TipoCambio(false, true, CocinaActualizada, CocinaDesactivada) != CocinaActualizada {
		t.Fatal("reactivar es actualizada")
	}
	if TipoCambio(true, true, PlatoActualizado, PlatoDesactivado) != PlatoActualizado {
		t.Fatal("un cambio común es actualizado")
	}
}
