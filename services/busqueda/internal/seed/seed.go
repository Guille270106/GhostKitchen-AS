// Package seed carga en el índice las 4 cocinas y sus platos de ejemplo.
//
// Hasta que Catálogo e Inventario publiquen sus mensajes (cocina.*, plato.*,
// disponibilidad.cambiada), Búsqueda arranca con estos datos para que la API
// publicada funcione de punta a punta. Los nombres de restaurantes son reales
// (trabajo académico); platos, precios y disponibilidad son inventados.
package seed

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"time"

	"ghostkitchen/busqueda/internal/domain"
	"ghostkitchen/busqueda/internal/repositories"
)

//go:embed datos/*.json
var datos embed.FS

type cocinaSeed struct {
	ID               string              `json:"id"`
	Nombre           string              `json:"nombre"`
	Localidad        string              `json:"localidad"`
	Direccion        string              `json:"direccion"`
	Ubicacion        domain.Ubicacion    `json:"ubicacion"`
	TiempoEntregaMin int                 `json:"tiempoEntregaMin"`
	TiempoEntregaMax int                 `json:"tiempoEntregaMax"`
	CostoEnvio       int                 `json:"costoEnvio"`
	Restaurantes     []domain.Referencia `json:"restaurantes"`
	Activa           bool                `json:"activa"`
}

type platoSeed struct {
	ID            string   `json:"id"`
	RestauranteID string   `json:"restauranteId"`
	Nombre        string   `json:"nombre"`
	Descripcion   string   `json:"descripcion"`
	Categoria     string   `json:"categoria"`
	Etiquetas     []string `json:"etiquetas"`
	Precio        int      `json:"precio"`
	Disponible    bool     `json:"disponible"`
	Activo        bool     `json:"activo"`
}

// Leer arma las sedes y platos de ejemplo. ahora es la fecha de actualización de los platos.
func Leer(ahora time.Time) ([]domain.Sede, []domain.Plato, error) {
	var cs []cocinaSeed
	if err := leerJSON("datos/cocinas.json", &cs); err != nil {
		return nil, nil, err
	}
	var ps []platoSeed
	if err := leerJSON("datos/platos.json", &ps); err != nil {
		return nil, nil, err
	}

	type ubicacionRest struct{ rest, sede domain.Referencia }
	porRestaurante := map[string]ubicacionRest{}
	sedes := make([]domain.Sede, 0, len(cs))
	for _, c := range cs {
		s := domain.Sede{
			ID: c.ID, Nombre: c.Nombre, Localidad: c.Localidad, Direccion: c.Direccion,
			Ubicacion: c.Ubicacion, TiempoEntregaMin: c.TiempoEntregaMin,
			TiempoEntregaMax: c.TiempoEntregaMax, CostoEnvio: c.CostoEnvio,
			Restaurantes: c.Restaurantes, Activa: c.Activa,
		}
		if len(s.Restaurantes) > 4 {
			return nil, nil, fmt.Errorf("seed: la cocina %s tiene más de 4 restaurantes (RN20)", s.ID)
		}
		for _, r := range s.Restaurantes {
			porRestaurante[r.ID] = ubicacionRest{rest: r, sede: s.Ref()}
		}
		sedes = append(sedes, s)
	}

	platos := make([]domain.Plato, 0, len(ps))
	for _, p := range ps {
		u, ok := porRestaurante[p.RestauranteID]
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
		platos = append(platos, domain.Plato{
			ID: p.ID, Nombre: p.Nombre, Descripcion: p.Descripcion, Categoria: p.Categoria,
			Etiquetas: etq, Precio: p.Precio, Moneda: "ARS", Disponible: p.Disponible,
			Activo: p.Activo, Restaurante: u.rest, Sede: u.sede, ActualizadoEn: ahora.UTC(),
		})
	}
	return sedes, platos, nil
}

// Cargar prepara el esquema y guarda (agrega o reemplaza por id) las sedes y platos de ejemplo.
// Es idempotente: se puede ejecutar en cada arranque.
func Cargar(ctx context.Context, idx repositories.Indexador, ahora time.Time) (int, int, error) {
	if err := idx.PrepararEsquema(ctx); err != nil {
		return 0, 0, fmt.Errorf("preparar esquema: %w", err)
	}
	sedes, platos, err := Leer(ahora)
	if err != nil {
		return 0, 0, err
	}
	if err := idx.GuardarSedes(ctx, sedes); err != nil {
		return 0, 0, fmt.Errorf("guardar sedes: %w", err)
	}
	if err := idx.GuardarPlatos(ctx, platos); err != nil {
		return 0, 0, fmt.Errorf("guardar platos: %w", err)
	}
	return len(sedes), len(platos), nil
}

// DisponibilidadInicial devuelve, para cada plato de ejemplo, si se puede pedir.
// Es provisional: la disponibilidad real es de Inventario (mensaje disponibilidad.cambiada).
// Un plato que no está en la tabla figura como no disponible (todavía no tiene receta).
func DisponibilidadInicial() (func(platoID string) bool, error) {
	var ps []platoSeed
	if err := leerJSON("datos/platos.json", &ps); err != nil {
		return nil, err
	}
	tabla := make(map[string]bool, len(ps))
	for _, p := range ps {
		tabla[p.ID] = p.Disponible
	}
	return func(id string) bool { return tabla[id] }, nil
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
