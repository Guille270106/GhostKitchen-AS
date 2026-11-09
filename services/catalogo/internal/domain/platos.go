package domain

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// Errores de las escrituras de la cuenta Cocina sobre sus platos (HU17).
var (
	ErrOtraCocina = errors.New("el plato o el restaurante es de otra cocina")
	// ErrSinReceta: un plato sin receta no se publica (RN18). Las recetas son de Inventario.
	ErrSinReceta = errors.New("el plato no tiene receta")
	// ErrInventarioNoDisponible: no se pudo preguntar a Inventario si hay receta. No se toma
	// como "no tiene receta": se responde 503 y se puede reintentar.
	ErrInventarioNoDisponible = errors.New("inventario no disponible")
)

// Precio máximo de un plato, en pesos. Frena un error de tipeo (un cero de más).
const PrecioMaximo = 10_000_000

// CambiosPlato son los datos que la cocina puede cambiar de un plato. Un campo nil no se toca.
// El restaurante y la cocina no se cambian: un plato no se muda de marca.
type CambiosPlato struct {
	Nombre      *string   `json:"nombre"`
	Descripcion *string   `json:"descripcion"`
	Categoria   *string   `json:"categoria"`
	Etiquetas   *[]string `json:"etiquetas"`
	Precio      *int      `json:"precio"`
	FotoURL     *string   `json:"fotoUrl"`
	Activo      *bool     `json:"activo"`
}

// Aplicar cambia los campos indicados y valida cómo queda el plato.
func (p *Plato) Aplicar(c CambiosPlato) error {
	if c.Nombre != nil {
		p.Nombre = strings.TrimSpace(*c.Nombre)
	}
	if c.Descripcion != nil {
		p.Descripcion = strings.TrimSpace(*c.Descripcion)
	}
	if c.Categoria != nil {
		p.Categoria = strings.TrimSpace(*c.Categoria)
	}
	if c.Etiquetas != nil {
		p.Etiquetas = append([]string{}, *c.Etiquetas...)
	}
	if c.Precio != nil {
		p.Precio = *c.Precio
	}
	if c.FotoURL != nil {
		p.FotoURL = strings.TrimSpace(*c.FotoURL)
	}
	if c.Activo != nil {
		p.Activo = *c.Activo
	}
	return p.Validar()
}

// Validar revisa los datos del plato (HU17): nombre, categoría, precio y etiquetas permitidas.
func (p Plato) Validar() error {
	switch {
	case p.Nombre == "" || utf8.RuneCountInString(p.Nombre) > 80:
		return invalido("el nombre es obligatorio y tiene hasta 80 caracteres")
	case utf8.RuneCountInString(p.Descripcion) > 300:
		return invalido("la descripción tiene hasta 300 caracteres")
	case p.Categoria == "" || utf8.RuneCountInString(p.Categoria) > 40:
		return invalido("la categoría es obligatoria y tiene hasta 40 caracteres")
	case p.Precio <= 0 || p.Precio > PrecioMaximo:
		return invalido("el precio tiene que ser mayor a 0")
	case p.FotoURL != "" && !strings.HasPrefix(p.FotoURL, "/") && !strings.HasPrefix(p.FotoURL, "https://"):
		return invalido("la foto tiene que ser una ruta (/images/...) o una dirección https")
	}
	vistas := map[string]bool{}
	for _, e := range p.Etiquetas {
		if !EtiquetasValidas[e] {
			return invalido("etiqueta desconocida: las permitidas son Vegano, Vegetariano, Sin TACC y Picante")
		}
		if vistas[e] {
			return invalido("etiqueta repetida")
		}
		vistas[e] = true
	}
	return nil
}
