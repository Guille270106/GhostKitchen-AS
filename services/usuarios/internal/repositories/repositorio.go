// Package repositories define cómo el servicio guarda y busca usuarios, sin decir dónde.
// La implementación con MySQL está en repositories/mysql; los tests usan una en memoria.
package repositories

import (
	"context"

	"ghostkitchen/usuarios/internal/domain"
)

// Usuarios es lo que la capa de servicios necesita de la base.
type Usuarios interface {
	// Crear guarda una cuenta nueva. Si el correo ya existe devuelve domain.ErrEmailRepetido.
	Crear(ctx context.Context, u domain.Usuario) error
	// PorEmail busca una cuenta por su correo ya normalizado. Si no existe, domain.ErrNoEncontrado.
	PorEmail(ctx context.Context, email string) (domain.Usuario, error)
	// PorID busca una cuenta por su id. Si no existe, domain.ErrNoEncontrado.
	PorID(ctx context.Context, id string) (domain.Usuario, error)
	// Listo dice si la base responde y la tabla de usuarios existe (para /ready).
	Listo(ctx context.Context) error
}
