package domain

import (
	"errors"
	"strings"
	"unicode/utf8"
)

var (
	ErrDatosInvalidos = errors.New("datos de ingrediente inválidos")
	ErrNoEncontrado   = errors.New("recurso no encontrado")
	ErrSinPermiso     = errors.New("solo el rol Cocina puede administrar ingredientes")
	ErrSinIdentidad   = errors.New("falta identidad verificada")
)

type Identidad struct{ UsuarioID, Rol, CocinaID string }

func (i Identidad) Validar() error {
	if i.UsuarioID == "" || i.Rol == "" {
		return ErrSinIdentidad
	}
	if i.Rol != "cocina" {
		return ErrSinPermiso
	}
	if i.CocinaID == "" {
		return ErrSinPermiso
	}
	return nil
}

type Ingrediente struct {
	ID         string `json:"id"`
	CocinaID   string `json:"cocinaId"`
	Nombre     string `json:"nombre"`
	Unidad     string `json:"unidad"`
	Total      int64  `json:"total"`
	Reservado  int64  `json:"reservado"`
	Disponible int64  `json:"disponible"`
}

func NuevoIngrediente(id, cocina, nombre, unidad string, inicial int64) (Ingrediente, error) {
	nombre, unidad = strings.TrimSpace(nombre), strings.TrimSpace(unidad)
	if id == "" || cocina == "" || nombre == "" || unidad == "" ||
		utf8.RuneCountInString(nombre) > 120 || utf8.RuneCountInString(unidad) > 30 || inicial < 0 {
		return Ingrediente{}, ErrDatosInvalidos
	}
	return Ingrediente{ID: id, CocinaID: cocina, Nombre: nombre, Unidad: unidad, Total: inicial, Disponible: inicial}, nil
}
