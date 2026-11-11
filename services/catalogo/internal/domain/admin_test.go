package domain

import (
	"errors"
	"testing"
)

func ptr[T any](v T) *T { return &v }

func cocinaValida() Cocina {
	return Cocina{ID: "nc", Nombre: "Cocina NC", Localidad: "Nueva Córdoba", Direccion: "Obispo Trejo 1050",
		Ubicacion: Ubicacion{Lat: -31.42, Lng: -64.18}, TiempoEntregaMin: 25, TiempoEntregaMax: 35, CostoEnvio: 1200, Activa: true}
}

func TestAplicarYValidar(t *testing.T) {
	casos := map[string]struct {
		cambios CambiosCocina
		valido  bool
	}{
		"nombre nuevo":               {CambiosCocina{Nombre: ptr("  Cocina Centro  ")}, true},
		"desactivar":                 {CambiosCocina{Activa: ptr(false)}, true},
		"sin nombre":                 {CambiosCocina{Nombre: ptr("  ")}, false},
		"sin dirección":              {CambiosCocina{Direccion: ptr("")}, false},
		"ubicación 0,0 (sin mandar)": {CambiosCocina{Ubicacion: &Ubicacion{}}, false},
		"latitud imposible":          {CambiosCocina{Ubicacion: &Ubicacion{Lat: 95, Lng: -64}}, false},
		"mínimo mayor al máximo":     {CambiosCocina{TiempoEntregaMin: ptr(50)}, false},
		"envío negativo":             {CambiosCocina{CostoEnvio: ptr(-1)}, false},
	}
	for nombre, c := range casos {
		cocina := cocinaValida()
		err := cocina.Aplicar(c.cambios)
		if c.valido != (err == nil) || (!c.valido && !errors.Is(err, ErrDatosInvalidos)) {
			t.Errorf("%s: %v", nombre, err)
		}
	}
	c := cocinaValida()
	_ = c.Aplicar(CambiosCocina{Nombre: ptr("  Cocina Centro  ")})
	if c.Nombre != "Cocina Centro" || c.Direccion != "Obispo Trejo 1050" {
		t.Fatalf("solo cambia lo que viene, sin espacios de más: %+v", c)
	}
}

func TestCuatroRestaurantesSinRepetir(t *testing.T) {
	c := cocinaValida()
	for i, n := range []string{"Mostaza", "Café Martínez", "Sushi Pop", "La Cabrera"} {
		if err := c.AgregarRestaurante(Restaurante{ID: string(rune('a' + i)), Nombre: n, Activo: true}); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.AgregarRestaurante(Restaurante{ID: "e", Nombre: "Otro"}); !errors.Is(err, ErrCocinaLlena) {
		t.Fatalf("el quinto tiene que rechazarse: %v", err)
	}

	c2 := cocinaValida()
	_ = c2.AgregarRestaurante(Restaurante{ID: "a", Nombre: "Café Martínez"})
	if err := c2.AgregarRestaurante(Restaurante{ID: "b", Nombre: "  cafe   MARTINEZ "}); !errors.Is(err, ErrRestauranteRepetido) {
		t.Fatalf("el mismo nombre con otras mayúsculas y sin acentos es repetido: %v", err)
	}
	if err := c2.AgregarRestaurante(Restaurante{ID: "c", Nombre: ""}); !errors.Is(err, ErrDatosInvalidos) {
		t.Fatalf("sin nombre: %v", err)
	}
}

func TestDesactivadoSigueOcupandoLugarYNombre(t *testing.T) {
	c := cocinaValida()
	_ = c.AgregarRestaurante(Restaurante{ID: "a", Nombre: "Mostaza", Activo: true})
	if _, err := c.CambiarRestaurante("a", CambiosRestaurante{Activo: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if err := c.AgregarRestaurante(Restaurante{ID: "b", Nombre: "Mostaza"}); !errors.Is(err, ErrRestauranteRepetido) {
		t.Fatalf("un restaurante desactivado no se borra: su nombre sigue ocupado: %v", err)
	}
}

func TestCambiarRestaurante(t *testing.T) {
	c := cocinaValida()
	_ = c.AgregarRestaurante(Restaurante{ID: "a", Nombre: "Mostaza", Activo: true})
	_ = c.AgregarRestaurante(Restaurante{ID: "b", Nombre: "Sushi Pop", Activo: true})
	if r, err := c.CambiarRestaurante("a", CambiosRestaurante{Nombre: ptr("Mostaza Express")}); err != nil || r.Nombre != "Mostaza Express" {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err := c.CambiarRestaurante("a", CambiosRestaurante{Nombre: ptr("sushi pop")}); !errors.Is(err, ErrRestauranteRepetido) {
		t.Fatalf("renombrar a uno que ya existe: %v", err)
	}
	if _, err := c.CambiarRestaurante("a", CambiosRestaurante{Nombre: ptr("MOSTAZA EXPRESS")}); err != nil {
		t.Fatalf("renombrarse a sí mismo con otras mayúsculas está bien: %v", err)
	}
	if _, err := c.CambiarRestaurante("zz", CambiosRestaurante{Activo: ptr(false)}); !errors.Is(err, ErrRestauranteNoEncontrado) {
		t.Fatalf("restaurante inexistente: %v", err)
	}
}

func TestIDDesdeNombre(t *testing.T) {
	casos := map[string]string{
		"Cocina Alta Córdoba":  "alta-cordoba",
		"  Cocina   Güemes!! ": "guemes",
		"Villa Belgrano 2":     "villa-belgrano-2",
		"Cocina":               "cocina", // solo se saca "cocina " si hay algo después
		"¡¡¡":                  "",
	}
	for nombre, id := range casos {
		if got := IDDesdeNombre(nombre); got != id {
			t.Errorf("IDDesdeNombre(%q) = %q, esperaba %q", nombre, got, id)
		}
	}
}
