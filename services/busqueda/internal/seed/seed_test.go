package seed

import (
	"context"
	"testing"
	"time"

	"ghostkitchen/busqueda/internal/repositories/memoria"
)

func TestLeerDatosIniciales(t *testing.T) {
	sedes, platos, err := Leer(time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(sedes) != 4 {
		t.Fatalf("esperaba 4 sedes, obtuve %d", len(sedes))
	}
	for _, s := range sedes {
		if len(s.Restaurantes) == 0 || len(s.Restaurantes) > 4 {
			t.Errorf("la sede %s tiene %d restaurantes", s.ID, len(s.Restaurantes))
		}
	}
	if len(platos) != 64 {
		t.Fatalf("esperaba 64 platos, obtuve %d", len(platos))
	}
	ids := map[string]bool{}
	for _, p := range platos {
		if ids[p.ID] {
			t.Errorf("id repetido %s", p.ID)
		}
		ids[p.ID] = true
		if p.Sede.ID == "" || p.Restaurante.Nombre == "" || p.Moneda != "ARS" {
			t.Errorf("plato %s incompleto: %+v", p.ID, p)
		}
	}
}

func TestPlatoDelEjemploDelContrato(t *testing.T) {
	_, platos, err := Leer(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range platos {
		if p.ID == "p5" {
			if p.Nombre != "Salmón roll x 10" || p.Disponible || p.Sede.ID != "nueva-cordoba" || p.Restaurante.ID != "r2" {
				t.Fatalf("p5 no coincide con el ejemplo del contrato: %+v", p)
			}
			return
		}
	}
	t.Fatal("no está p5")
}

func TestCargarEsIdempotente(t *testing.T) {
	repo := memoria.Nuevo()
	for i := 0; i < 2; i++ {
		s, p, err := Cargar(context.Background(), repo, time.Now())
		if err != nil || s != 4 || p != 64 {
			t.Fatalf("carga %d: sedes=%d platos=%d err=%v", i+1, s, p, err)
		}
	}
	sedes, _ := repo.ListarSedes(context.Background())
	if len(sedes) != 4 {
		t.Fatalf("después de cargar dos veces hay %d sedes", len(sedes))
	}
}
