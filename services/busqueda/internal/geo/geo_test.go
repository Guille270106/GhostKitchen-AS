package geo

import (
	"math"
	"testing"

	"ghostkitchen/busqueda/internal/domain"
)

func TestDistanciaMismoPuntoEsCero(t *testing.T) {
	p := domain.Ubicacion{Lat: -31.4262, Lng: -64.1883}
	if d := DistanciaKm(p, p); d != 0 {
		t.Fatalf("esperaba 0, obtuve %v", d)
	}
}

func TestDistanciaEsSimetrica(t *testing.T) {
	a := domain.Ubicacion{Lat: -31.4262, Lng: -64.1883}
	b := domain.Ubicacion{Lat: -31.2958, Lng: -64.2951}
	if math.Abs(DistanciaKm(a, b)-DistanciaKm(b, a)) > 1e-9 {
		t.Fatal("la distancia no es simétrica")
	}
}

func TestDistanciaConocida(t *testing.T) {
	// Un grado de latitud son unos 111,2 km.
	d := DistanciaKm(domain.Ubicacion{Lat: 0, Lng: 0}, domain.Ubicacion{Lat: 1, Lng: 0})
	if math.Abs(d-111.195) > 0.01 {
		t.Fatalf("esperaba ~111,195 km, obtuve %v", d)
	}
}

func TestRedondear(t *testing.T) {
	casos := []struct {
		v    float64
		dec  int
		want float64
	}{
		{4.449, 1, 4.4}, {4.45, 1, 4.5}, {1.23456, 3, 1.235}, {0, 3, 0},
	}
	for _, c := range casos {
		if got := Redondear(c.v, c.dec); got != c.want {
			t.Errorf("Redondear(%v, %d) = %v, esperaba %v", c.v, c.dec, got, c.want)
		}
	}
}
