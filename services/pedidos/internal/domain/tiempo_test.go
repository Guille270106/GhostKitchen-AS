package domain

import (
	"testing"
	"time"
)

func TestSegundosRestantesDeLaReserva(t *testing.T) {
	ahora := time.Date(2026, 11, 9, 20, 0, 0, 0, time.UTC)
	vence := ahora.Add(10 * time.Minute)
	c := Carrito{ID: "car-1", VenceEn: &vence}
	_ = c.Agregar(burger, 1)

	casos := []struct {
		nombre   string
		ahora    time.Time
		segundos int64
		vencido  bool
	}{
		{"recién reservado", ahora, 600, false},
		{"pasaron 4 minutos y medio", ahora.Add(4*time.Minute + 30*time.Second), 330, false},
		{"queda menos de un segundo", vence.Add(-400 * time.Millisecond), 1, false},
		{"justo al vencer", vence, 0, true},
		{"ya vencido", vence.Add(time.Minute), 0, true},
	}
	for _, caso := range casos {
		c.ActualizarTiempo(caso.ahora)
		if c.SegundosRestantes != caso.segundos || c.Vencido != caso.vencido {
			t.Errorf("%s: esperaba %d s y vencido=%v, obtuvo %d s y vencido=%v",
				caso.nombre, caso.segundos, caso.vencido, c.SegundosRestantes, c.Vencido)
		}
	}
}

func TestCarritoSinReservaNoTieneTiempo(t *testing.T) {
	c := Carrito{}
	c.ActualizarTiempo(time.Now())
	if c.SegundosRestantes != 0 || c.Vencido {
		t.Fatal("un carrito vacío no tiene reserva ni vence:", c.SegundosRestantes, c.Vencido)
	}
}
