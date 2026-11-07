// Package domain define qué es un usuario de GhostKitchen, sus roles y los errores del servicio.
package domain

import (
	"errors"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"
)

// Rol es el tipo de cuenta. Decide qué puede hacer la persona y qué datos tiene la cuenta.
type Rol string

const (
	// RolCliente pide comida. Puede elegir y cambiar su cocina.
	RolCliente Rol = "cliente"
	// RolCocina administra los platos, pedidos e inventario de una sola cocina.
	RolCocina Rol = "cocina"
	// RolAdmin administra cocinas, restaurantes y cuentas de su localidad. No se registra solo.
	RolAdmin Rol = "admin"
)

// Valido dice si el rol es uno de los tres que existen.
func (r Rol) Valido() bool { return r == RolCliente || r == RolCocina || r == RolAdmin }

// Usuario es una cuenta. PasswordHash es el hash bcrypt: la contraseña nunca se guarda ni
// se devuelve, por eso el campo no sale en JSON.
type Usuario struct {
	ID               string    `json:"id"`
	Nombre           string    `json:"nombre"`
	Email            string    `json:"email"`
	PasswordHash     string    `json:"-"`
	Direccion        string    `json:"direccion,omitempty"`
	Rol              Rol       `json:"rol"`
	CocinaElegidaID  string    `json:"cocinaElegidaId,omitempty"`  // cliente
	CocinaAsignadaID string    `json:"cocinaAsignadaId,omitempty"` // cuenta Cocina
	Localidad        string    `json:"localidad,omitempty"`        // admin
	Activo           bool      `json:"activo"`
	CreadoEn         time.Time `json:"creadoEn"`
}

// Errores del servicio. Los handlers los traducen a códigos HTTP; nunca muestran el error de
// la base, que puede tener datos de la consulta.
var (
	ErrDatosInvalidos = errors.New("datos de usuario inválidos")
	ErrEmailRepetido  = errors.New("ya existe una cuenta con ese correo")
	ErrNoEncontrado   = errors.New("usuario no encontrado")
)

// Largo permitido de la contraseña. El máximo es el de bcrypt: solo usa los primeros 72
// bytes, así que una contraseña más larga se rechaza en lugar de recortarla sin avisar.
const (
	PasswordMin = 8
	PasswordMax = 72
)

// ValidarPassword revisa el largo. No se guarda ni se registra en ningún lado: solo su hash.
func ValidarPassword(p string) error {
	if utf8.RuneCountInString(p) < PasswordMin || len(p) > PasswordMax {
		return errors.Join(ErrDatosInvalidos, errors.New("la contraseña tiene que tener entre 8 caracteres y 72 bytes"))
	}
	return nil
}

// NormalizarEmail quita espacios y pasa a minúsculas, así "Ana@Mail.com" y "ana@mail.com"
// son la misma cuenta. Devuelve "" si no es un correo.
func NormalizarEmail(email string) string {
	e := strings.ToLower(strings.TrimSpace(email))
	if len(e) > 254 {
		return ""
	}
	dir, err := mail.ParseAddress(e)
	if err != nil || dir.Address != e || dir.Name != "" {
		return ""
	}
	return e
}

// Validar revisa que la cuenta tenga los datos que corresponden a su rol, lo mismo que piden
// los CHECK de la tabla. Se valida antes de llegar a la base para responder un 400 claro.
func (u Usuario) Validar() error {
	nombre := strings.TrimSpace(u.Nombre)
	switch {
	case nombre == "" || utf8.RuneCountInString(nombre) > 120:
		return errors.Join(ErrDatosInvalidos, errors.New("el nombre es obligatorio y tiene hasta 120 caracteres"))
	case NormalizarEmail(u.Email) != u.Email:
		return errors.Join(ErrDatosInvalidos, errors.New("el correo no es válido"))
	case len(u.PasswordHash) != 60:
		return errors.Join(ErrDatosInvalidos, errors.New("falta el hash de la contraseña"))
	case !u.Rol.Valido():
		return errors.Join(ErrDatosInvalidos, errors.New("el rol no existe"))
	case utf8.RuneCountInString(u.Direccion) > 255:
		return errors.Join(ErrDatosInvalidos, errors.New("la dirección tiene hasta 255 caracteres"))
	case (u.Rol == RolCocina) != (u.CocinaAsignadaID != ""):
		return errors.Join(ErrDatosInvalidos, errors.New("solo una cuenta Cocina tiene cocina asignada, y la necesita"))
	case (u.Rol == RolAdmin) != (u.Localidad != ""):
		return errors.Join(ErrDatosInvalidos, errors.New("solo un admin tiene localidad, y la necesita"))
	case u.Rol != RolCliente && u.CocinaElegidaID != "":
		return errors.Join(ErrDatosInvalidos, errors.New("solo un cliente elige cocina"))
	}
	return nil
}
