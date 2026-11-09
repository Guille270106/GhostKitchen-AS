package domain

import (
	"errors"
	"testing"
)

var (
	burger = PlatoCatalogo{ID: "p17", CocinaID: "cerro", RestauranteID: "r5", Nombre: "Burger", Precio: 12000, Activo: true}
	pizza  = PlatoCatalogo{ID: "p21", CocinaID: "cerro", RestauranteID: "r6", Nombre: "Pizza", Precio: 9500, Activo: true}
	sushi  = PlatoCatalogo{ID: "p5", CocinaID: "nueva-cordoba", RestauranteID: "r2", Nombre: "Sushi", Precio: 15500, Activo: true}
)

func TestAgregarPlatosDeDistintosRestaurantesDeLaMismaCocina(t *testing.T) {
	c := Carrito{ID: "car-1"}
	if err := c.Agregar(burger, 2); err != nil {
		t.Fatal(err)
	}
	if err := c.Agregar(pizza, 1); err != nil {
		t.Fatal(err)
	}
	if err := c.Agregar(burger, 1); err != nil {
		t.Fatal(err)
	}
	if len(c.Items) != 2 || c.Items[0].Cantidad != 3 || c.Total != 3*12000+9500 || c.CocinaID != "cerro" {
		t.Fatalf("%+v", c)
	}
}

func TestNoSeMezclanCocinas(t *testing.T) {
	c := Carrito{ID: "car-1"}
	_ = c.Agregar(burger, 1)
	if err := c.Agregar(sushi, 1); !errors.Is(err, ErrOtraCocina) {
		t.Fatal(err)
	}
	if len(c.Items) != 1 {
		t.Fatal("el plato de otra cocina no se agrega")
	}
}

func TestElPrecioSeGuardaAlAgregar(t *testing.T) {
	c := Carrito{ID: "car-1"}
	_ = c.Agregar(burger, 1)
	masCara := burger
	masCara.Precio = 99999
	_ = c.Agregar(masCara, 1)
	if c.Items[0].PrecioUnitario != 12000 {
		t.Fatal("el precio no cambia aunque cambie en Catálogo:", c.Items[0].PrecioUnitario)
	}
}

func TestCantidadesInvalidasYPlatoInactivo(t *testing.T) {
	c := Carrito{ID: "car-1"}
	for _, n := range []int64{0, -1, 51} {
		if err := c.Agregar(burger, n); !errors.Is(err, ErrCantidadInvalida) {
			t.Errorf("cantidad %d: %v", n, err)
		}
	}
	inactivo := burger
	inactivo.Activo = false
	if err := c.Agregar(inactivo, 1); !errors.Is(err, ErrPlatoNoEncontrado) {
		t.Error(err)
	}
	_ = c.Agregar(burger, 50)
	if err := c.Agregar(burger, 1); !errors.Is(err, ErrCantidadInvalida) {
		t.Error("no pasa de 50 unidades por plato:", err)
	}
}

func TestCambiarCantidadYQuitar(t *testing.T) {
	c := Carrito{ID: "car-1"}
	_ = c.Agregar(burger, 1)
	_ = c.Agregar(pizza, 1)
	if err := c.CambiarCantidad("p17", 4); err != nil || c.Total != 4*12000+9500 {
		t.Fatal(err, c.Total)
	}
	if err := c.Quitar("p21"); err != nil || len(c.Items) != 1 || c.Total != 48000 {
		t.Fatal(err, c)
	}
	if err := c.Quitar("p99"); !errors.Is(err, ErrNoEstaEnElCarrito) {
		t.Fatal(err)
	}
	if err := c.CambiarCantidad("p99", 1); !errors.Is(err, ErrNoEstaEnElCarrito) {
		t.Fatal(err)
	}
}

func TestReiniciarDejaElCarritoVacioConOtroID(t *testing.T) {
	c := Carrito{ID: "car-1", ReservaID: "res-1"}
	_ = c.Agregar(burger, 1)
	c.Reiniciar("car-2")
	if !c.Vacio() || c.ID != "car-2" || c.ReservaID != "" || c.CocinaID != "" || c.Total != 0 {
		t.Fatalf("%+v", c)
	}
	if err := c.Agregar(sushi, 1); err != nil {
		t.Fatal("vacío se puede elegir otra cocina:", err)
	}
}
