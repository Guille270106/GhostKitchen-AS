package domain

import (
	"math"
	"testing"
)

func TestTotalesReservaDetectaOverflow(t *testing.T) {
	for _, lotes := range [][]LoteReserva{
		{{Unidades: 2, Ingredientes: []RecetaItem{{IngredienteID: "i", Cantidad: math.MaxInt64}}}},
		{{Unidades: 1, Ingredientes: []RecetaItem{{IngredienteID: "i", Cantidad: math.MaxInt64}}}, {Unidades: 1, Ingredientes: []RecetaItem{{IngredienteID: "i", Cantidad: 1}}}},
	} {
		if _, err := Totales([]PlatoReservado{{Lotes: lotes}}); err != ErrReservaInvalida {
			t.Fatal("no detectó overflow", err)
		}
	}
}

func TestReducirConservaRecetasApartadas(t *testing.T) {
	lotes := []LoteReserva{
		{Unidades: 2, RecetaVersion: 1, Ingredientes: []RecetaItem{{IngredienteID: "i1", Cantidad: 2}}},
		{Unidades: 3, RecetaVersion: 2, Ingredientes: []RecetaItem{{IngredienteID: "i2", Cantidad: 4}}},
	}
	reducidos := Reducir(lotes, 4)
	if len(reducidos) != 1 || reducidos[0].Unidades != 1 || reducidos[0].RecetaVersion != 1 {
		t.Fatal(reducidos)
	}
	if lotes[0].Unidades != 2 || lotes[1].Unidades != 3 {
		t.Fatal("mutó los lotes originales")
	}
}
