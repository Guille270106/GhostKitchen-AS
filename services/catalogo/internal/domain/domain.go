// Package domain define qué es una cocina, un restaurante y un plato en Catálogo.
// Catálogo es la fuente de verdad de los datos comerciales: qué se vende y a qué precio.
// No guarda recetas ni stock (eso es de Inventario).
//
// Es la capa de más adentro (ADR-002): no importa ningún otro paquete del servicio. Los tags
// bson dicen cómo se guarda cada campo en MongoDB y los json cómo se publica.
package domain

import (
	"errors"
	"time"
)

// Ubicacion es un punto en grados decimales (WGS84).
type Ubicacion struct {
	Lat float64 `json:"lat" bson:"lat"`
	Lng float64 `json:"lng" bson:"lng"`
}

// Restaurante es una marca dentro de una cocina. No tiene stock ni estados propios.
type Restaurante struct {
	ID     string `json:"id" bson:"id"`
	Nombre string `json:"nombre" bson:"nombre"`
	Activo bool   `json:"activo" bson:"activo"`
}

// MaxRestaurantes por cocina (RN20).
const MaxRestaurantes = 4

// Cocina es una cocina fantasma con sus restaurantes adentro (se leen siempre juntos).
//
// Los restaurantes van embebidos en el documento de la cocina y no en otra colección: son
// pocos (como máximo 4), no existen sin su cocina y siempre se leen junto con ella, así una
// sola lectura trae todo. Además, MongoDB cambia un documento de forma atómica, así que
// cuando existan las escrituras el límite de 4 se podrá controlar en una sola operación,
// sin transacciones (ADR-003, Clase 2).
//
// Version es la versión del documento (los datos iniciales arrancan en 1). Está pensada para
// que una copia, como el índice de Búsqueda, nunca guarde un dato más viejo encima de uno
// más nuevo (ARCHITECTURE 6.3).
type Cocina struct {
	ID               string        `json:"id" bson:"_id"`
	Nombre           string        `json:"nombre" bson:"nombre"`
	Localidad        string        `json:"localidad" bson:"localidad"`
	Direccion        string        `json:"direccion" bson:"direccion"`
	Ubicacion        Ubicacion     `json:"ubicacion" bson:"ubicacion"`
	TiempoEntregaMin int           `json:"tiempoEntregaMin" bson:"tiempoEntregaMin"`
	TiempoEntregaMax int           `json:"tiempoEntregaMax" bson:"tiempoEntregaMax"`
	CostoEnvio       int           `json:"costoEnvio" bson:"costoEnvio"`
	Restaurantes     []Restaurante `json:"restaurantes" bson:"restaurantes"`
	Activa           bool          `json:"activa" bson:"activa"`
	Version          int64         `json:"version" bson:"version"`
	ActualizadaEn    time.Time     `json:"actualizadaEn" bson:"actualizadaEn"`
}

// Plato es lo que se vende. Pertenece a un restaurante de una cocina.
//
// Los platos van en su propia colección (no embebidos en la cocina) porque son muchos, se
// consultan solos (por id) y se leen de a páginas. CocinaID se repite en cada plato para
// poder pedir el menú de una cocina sin pasar por sus restaurantes.
type Plato struct {
	ID            string    `json:"id" bson:"_id"`
	CocinaID      string    `json:"cocinaId" bson:"cocinaId"`
	RestauranteID string    `json:"restauranteId" bson:"restauranteId"`
	Nombre        string    `json:"nombre" bson:"nombre"`
	Descripcion   string    `json:"descripcion,omitempty" bson:"descripcion,omitempty"`
	Categoria     string    `json:"categoria" bson:"categoria"`
	Etiquetas     []string  `json:"etiquetas" bson:"etiquetas"`
	Precio        int       `json:"precio" bson:"precio"`
	Moneda        string    `json:"moneda" bson:"moneda"`
	FotoURL       string    `json:"fotoUrl,omitempty" bson:"fotoUrl,omitempty"`
	Activo        bool      `json:"activo" bson:"activo"`
	Version       int64     `json:"version" bson:"version"`
	ActualizadoEn time.Time `json:"actualizadoEn" bson:"actualizadoEn"`
}

// EtiquetasValidas son las etiquetas permitidas. Búsqueda tiene la misma lista: si se agrega
// una, hay que cambiarla en los dos servicios.
var EtiquetasValidas = map[string]bool{"Vegano": true, "Vegetariano": true, "Sin TACC": true, "Picante": true}

// Pagina es una página de resultados de una lectura paginada.
type Pagina[T any] struct {
	Items      []T `json:"items"`
	Page       int `json:"page"`
	PageSize   int `json:"pageSize"`
	Total      int `json:"total"`
	TotalPages int `json:"totalPages"`
}

// Errores de negocio.
var (
	ErrCocinaNoEncontrada = errors.New("cocina no encontrada")
	ErrPlatoNoEncontrado  = errors.New("plato no encontrado")
	// ErrNoDisponible envuelve cualquier falla de MongoDB (caída, timeout). El handler la
	// traduce a 503: "no sé" no es lo mismo que "no existe" (ADR-010).
	ErrNoDisponible = errors.New("almacenamiento no disponible")
)
