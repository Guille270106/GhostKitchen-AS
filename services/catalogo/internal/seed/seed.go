// Package seed carga las 4 cocinas, sus restaurantes y los platos de ejemplo la primera vez
// que arranca Catálogo. Mientras no existan las pantallas de administración, son los datos
// con los que trabaja todo el sistema. Los nombres de restaurantes son reales (trabajo
// académico); platos y precios son inventados.
package seed

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"time"

	"ghostkitchen/catalogo/internal/domain"
	"ghostkitchen/catalogo/internal/repositories"
)

//go:embed datos/*.json
var datos embed.FS

type platoSeed struct {
	ID            string   `json:"id"`
	RestauranteID string   `json:"restauranteId"`
	Nombre        string   `json:"nombre"`
	Descripcion   string   `json:"descripcion"`
	Categoria     string   `json:"categoria"`
	Etiquetas     []string `json:"etiquetas"`
	Precio        int      `json:"precio"`
	Activo        bool     `json:"activo"`
}

// Leer arma las cocinas y platos de ejemplo y valida las reglas básicas (RN18–RN20).
func Leer(ahora time.Time) ([]domain.Cocina, []domain.Plato, error) {
	var cocinas []domain.Cocina
	if err := leerJSON("datos/cocinas.json", &cocinas); err != nil {
		return nil, nil, err
	}
	var ps []platoSeed
	if err := leerJSON("datos/platos.json", &ps); err != nil {
		return nil, nil, err
	}

	cocinaDe := map[string]string{} // restaurante -> cocina
	for i := range cocinas {
		c := &cocinas[i]
		if len(c.Restaurantes) > domain.MaxRestaurantes {
			return nil, nil, fmt.Errorf("seed: la cocina %s tiene más de %d restaurantes (RN20)", c.ID, domain.MaxRestaurantes)
		}
		for _, r := range c.Restaurantes {
			if _, repetido := cocinaDe[r.ID]; repetido {
				return nil, nil, fmt.Errorf("seed: el restaurante %s está repetido", r.ID)
			}
			cocinaDe[r.ID] = c.ID
		}
		c.Version, c.ActualizadaEn = 1, ahora.UTC()
	}

	platos := make([]domain.Plato, 0, len(ps))
	for _, p := range ps {
		cocinaID, ok := cocinaDe[p.RestauranteID]
		if !ok {
			return nil, nil, fmt.Errorf("seed: el plato %s usa el restaurante desconocido %s", p.ID, p.RestauranteID)
		}
		etq := p.Etiquetas
		if etq == nil {
			etq = []string{}
		}
		for _, e := range etq {
			if !domain.EtiquetasValidas[e] {
				return nil, nil, fmt.Errorf("seed: el plato %s tiene la etiqueta inválida %q", p.ID, e)
			}
		}
		if p.Precio < 0 {
			return nil, nil, fmt.Errorf("seed: el plato %s tiene precio negativo", p.ID)
		}
		platos = append(platos, domain.Plato{
			ID: p.ID, CocinaID: cocinaID, RestauranteID: p.RestauranteID, Nombre: p.Nombre,
			Descripcion: p.Descripcion, Categoria: p.Categoria, Etiquetas: etq, Precio: p.Precio,
			Moneda: "ARS", Activo: p.Activo, Version: 1, ActualizadoEn: ahora.UTC(),
		})
	}
	return cocinas, platos, nil
}

// Cargar inserta los datos de ejemplo que falten. No pisa lo que ya está en la base:
// si alguien cambió un precio en MongoDB, el cambio se conserva al reiniciar.
func Cargar(ctx context.Context, repo repositories.Repositorio, ahora time.Time) (int, int, error) {
	cocinas, platos, err := Leer(ahora)
	if err != nil {
		return 0, 0, err
	}
	if err := repo.InsertarIniciales(ctx, cocinas, platos); err != nil {
		return 0, 0, fmt.Errorf("cargar datos iniciales: %w", err)
	}
	return len(cocinas), len(platos), nil
}

func leerJSON(ruta string, destino any) error {
	b, err := datos.ReadFile(ruta)
	if err != nil {
		return fmt.Errorf("seed: leer %s: %w", ruta, err)
	}
	if err := json.Unmarshal(b, destino); err != nil {
		return fmt.Errorf("seed: %s inválido: %w", ruta, err)
	}
	return nil
}
