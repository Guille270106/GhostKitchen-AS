// Package services tiene las reglas de Usuarios. Los handlers lo llaman y él usa el
// repositorio; no sabe nada de HTTP ni de MySQL (Clase 8: handler, servicio, repositorio).
//
// G5 deja la base: el registro (G6), el login (G7) y las cuentas Cocina (G8) se agregan acá.
package services

import (
	"context"

	"ghostkitchen/usuarios/internal/repositories"
)

// Usuarios agrupa las reglas de las cuentas.
type Usuarios struct {
	repo repositories.Usuarios
}

// Nuevo crea el servicio sobre un repositorio.
func Nuevo(repo repositories.Usuarios) *Usuarios { return &Usuarios{repo: repo} }

// Listo dice si el servicio puede atender: hoy depende solo de MySQL.
func (s *Usuarios) Listo(ctx context.Context) error { return s.repo.Listo(ctx) }
