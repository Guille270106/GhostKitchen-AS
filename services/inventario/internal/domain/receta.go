package domain

import (
	"errors"
	"sort"
)

var ErrRecetaInvalida = errors.New("la receta requiere ingredientes distintos y cantidades enteras mayores a cero")

type RecetaItem struct {
	IngredienteID string `json:"ingredienteId"`
	Cantidad      int64  `json:"cantidad"`
}

type Receta struct {
	PlatoID  string       `json:"platoId"`
	CocinaID string       `json:"cocinaId"`
	Version  uint64       `json:"version"`
	Items    []RecetaItem `json:"items"`
}

func NuevaReceta(plato, cocina string, items []RecetaItem) (Receta, error) {
	if plato == "" || cocina == "" || len(items) == 0 {
		return Receta{}, ErrRecetaInvalida
	}
	vistos := make(map[string]bool, len(items))
	for _, i := range items {
		if i.IngredienteID == "" || i.Cantidad <= 0 || vistos[i.IngredienteID] {
			return Receta{}, ErrRecetaInvalida
		}
		vistos[i.IngredienteID] = true
	}
	copia := append([]RecetaItem(nil), items...)
	sort.Slice(copia, func(i, j int) bool { return copia[i].IngredienteID < copia[j].IngredienteID })
	return Receta{PlatoID: plato, CocinaID: cocina, Items: copia}, nil
}
