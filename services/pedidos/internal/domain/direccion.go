package domain

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxDireccion es el largo máximo de la dirección de entrega, igual que en Usuarios.
const MaxDireccion = 255

var ErrDireccionInvalida = errors.New("la dirección de entrega es obligatoria y tiene hasta 255 caracteres")

// NormalizarDireccion deja la dirección de entrega lista para guardar: sin espacios de más
// (ni al principio, ni al final, ni repetidos) y sin caracteres de control. La exige no vacía:
// sin dirección la cocina no sabe adónde mandar el pedido.
func NormalizarDireccion(texto string) (string, error) {
	if !utf8.ValidString(texto) {
		return "", ErrDireccionInvalida
	}
	for _, r := range texto {
		if unicode.IsControl(r) && r != '\t' && r != '\n' && r != '\r' {
			return "", ErrDireccionInvalida
		}
	}
	limpia := strings.Join(strings.Fields(texto), " ")
	if limpia == "" || utf8.RuneCountInString(limpia) > MaxDireccion {
		return "", ErrDireccionInvalida
	}
	return limpia, nil
}
