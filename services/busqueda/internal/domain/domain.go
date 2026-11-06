// Package domain define cómo son una sede (cocina) y un plato dentro de Búsqueda.
// Son copias armadas para buscar: la fuente de verdad es Catálogo (datos comerciales)
// e Inventario (disponibilidad).
//
// Es la capa de más adentro: no importa ningún otro paquete del servicio, y todos los demás
// (handlers, services, repositories) dependen de ella (ADR-002).
package domain

import (
	"errors"
	"time"
)

// Ubicacion es un punto en grados decimales (WGS84).
type Ubicacion struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// Referencia identifica un restaurante o una sede por id y nombre.
type Referencia struct {
	ID     string `json:"id"`
	Nombre string `json:"nombre"`
}

// Sede es una cocina fantasma tal como la publicamos.
//
// Activa no sale en el JSON: una sede inactiva directamente no se publica. Igual se guarda
// en el índice, para que desactivar una cocina en Catálogo pise su copia vieja en Solr.
// Lo mismo pasa con Plato.Activo.
type Sede struct {
	ID               string       `json:"id"`
	Nombre           string       `json:"nombre"`
	Localidad        string       `json:"localidad"`
	Direccion        string       `json:"direccion"`
	Ubicacion        Ubicacion    `json:"ubicacion"`
	TiempoEntregaMin int          `json:"tiempoEntregaMin"`
	TiempoEntregaMax int          `json:"tiempoEntregaMax"`
	CostoEnvio       int          `json:"costoEnvio"`
	Restaurantes     []Referencia `json:"restaurantes"`
	Activa           bool         `json:"-"`
}

// Ref devuelve la referencia corta de la sede.
func (s Sede) Ref() Referencia { return Referencia{ID: s.ID, Nombre: s.Nombre} }

// Plato es un plato de un restaurante de una sede.
type Plato struct {
	ID            string     `json:"id"`
	Nombre        string     `json:"nombre"`
	Descripcion   string     `json:"descripcion,omitempty"`
	Categoria     string     `json:"categoria"`
	Etiquetas     []string   `json:"etiquetas"`
	Precio        int        `json:"precio"`
	Moneda        string     `json:"moneda"`
	FotoURL       string     `json:"fotoUrl,omitempty"`
	Disponible    bool       `json:"disponible"`
	Restaurante   Referencia `json:"restaurante"`
	Sede          Referencia `json:"sede"`
	ActualizadoEn time.Time  `json:"actualizadoEn"`
	Activo        bool       `json:"-"`
}

// Orden de los resultados de la búsqueda de platos.
type Orden string

// Valores de Orden que acepta el parámetro orden del contrato.
const (
	OrdenRelevancia Orden = "relevancia"
	OrdenPrecioAsc  Orden = "precio_asc"
	OrdenPrecioDesc Orden = "precio_desc"
)

// FiltroPlatos son los criterios de búsqueda de platos de una sede.
type FiltroPlatos struct {
	SedeID          string
	Texto           string
	Categoria       string
	Etiqueta        string
	RestauranteID   string
	PrecioMax       int // 0 = sin límite
	SoloDisponibles bool
	Orden           Orden
	Pagina          int
	TamPagina       int
}

// EtiquetasValidas son las etiquetas permitidas por el contrato. Son las mismas que valida
// Catálogo: si se agrega una, hay que cambiarla en los dos servicios y en el contrato.
var EtiquetasValidas = map[string]bool{
	"Vegano":      true,
	"Vegetariano": true,
	"Sin TACC":    true,
	"Picante":     true,
}

// Errores de negocio que la capa HTTP traduce a Problem Details.
var (
	ErrSedeNoEncontrada  = errors.New("sede no encontrada")
	ErrPlatoNoEncontrado = errors.New("plato no encontrado")
	ErrSinCobertura      = errors.New("sin cobertura")
	// ErrNoDisponible envuelve cualquier falla del almacenamiento (Solr caído, timeout).
	// Nunca se responde una lista vacía para disimular una falla: el handler la traduce a
	// 503 con Retry-After (degradación honesta, ADR-010). Una lista vacía diría "no hay
	// platos", que es falso; el 503 dice "no sé", que es la verdad.
	ErrNoDisponible = errors.New("almacenamiento no disponible")
)
