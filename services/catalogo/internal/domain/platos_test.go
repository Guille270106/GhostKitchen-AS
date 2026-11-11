package domain

import (
	"errors"
	"testing"
)

func platoValido() Plato {
	return Plato{ID: "p1", CocinaID: "nc", RestauranteID: "r1", Nombre: "Mega doble", Categoria: "Hamburguesas", Precio: 9500, Moneda: "ARS", Etiquetas: []string{}}
}

func TestValidarPlato(t *testing.T) {
	casos := map[string]struct {
		cambios CambiosPlato
		valido  bool
	}{
		"precio nuevo":          {CambiosPlato{Precio: ptr(10500)}, true},
		"etiquetas válidas":     {CambiosPlato{Etiquetas: &[]string{"Vegano", "Sin TACC"}}, true},
		"foto local":            {CambiosPlato{FotoURL: ptr("/images/platos/p1.webp")}, true},
		"sin nombre":            {CambiosPlato{Nombre: ptr(" ")}, false},
		"sin categoría":         {CambiosPlato{Categoria: ptr("")}, false},
		"precio 0":              {CambiosPlato{Precio: ptr(0)}, false},
		"precio negativo":       {CambiosPlato{Precio: ptr(-10)}, false},
		"etiqueta inventada":    {CambiosPlato{Etiquetas: &[]string{"Keto"}}, false},
		"etiqueta repetida":     {CambiosPlato{Etiquetas: &[]string{"Vegano", "Vegano"}}, false},
		"foto http (no segura)": {CambiosPlato{FotoURL: ptr("http://x.com/a.jpg")}, false},
	}
	for nombre, c := range casos {
		p := platoValido()
		err := p.Aplicar(c.cambios)
		if c.valido != (err == nil) || (!c.valido && !errors.Is(err, ErrDatosInvalidos)) {
			t.Errorf("%s: %v", nombre, err)
		}
	}
}
