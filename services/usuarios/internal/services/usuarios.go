// Package services tiene las reglas de Usuarios. Los handlers lo llaman y él usa el
// repositorio; no sabe nada de HTTP ni de MySQL (Clase 8: handler, servicio, repositorio).
package services

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"ghostkitchen/usuarios/internal/domain"
	"ghostkitchen/usuarios/internal/repositories"
)

// CostoBcrypt es cuántas vueltas hace bcrypt (2^10). Más es más seguro pero más lento: con
// 10 tarda unos 50 a 100 ms, aceptable para registrarse y entrar sin frenar al servidor.
const CostoBcrypt = 10

// Usuarios agrupa las reglas de las cuentas.
type Usuarios struct {
	repo  repositories.Usuarios
	costo int
}

// Nuevo crea el servicio sobre un repositorio.
func Nuevo(repo repositories.Usuarios) *Usuarios { return &Usuarios{repo: repo, costo: CostoBcrypt} }

// ConCostoBcrypt cambia el costo. Solo para los tests: con bcrypt.MinCost corren rápido.
func (s *Usuarios) ConCostoBcrypt(costo int) *Usuarios { s.costo = costo; return s }

// Listo dice si el servicio puede atender: hoy depende solo de MySQL.
func (s *Usuarios) Listo(ctx context.Context) error { return s.repo.Listo(ctx) }

// DatosRegistro es lo que manda la persona al registrarse (HU01). No pide localidad (RN22).
type DatosRegistro struct {
	Nombre    string
	Email     string
	Password  string
	Direccion string
}

// Registrar crea una cuenta de cliente (HU01). Solo se registran clientes: las cuentas Cocina
// las crea un admin (G8) y los admins se cargan al iniciar el sistema.
//
// Si el correo ya existe devuelve domain.ErrEmailRepetido, sin decir nada de la otra cuenta.
// La contraseña sale de acá solo como hash bcrypt.
func (s *Usuarios) Registrar(ctx context.Context, d DatosRegistro) (domain.Usuario, error) {
	email := domain.NormalizarEmail(d.Email)
	if email == "" {
		return domain.Usuario{}, errors.Join(domain.ErrDatosInvalidos, errors.New("el correo no es válido"))
	}
	direccion := strings.TrimSpace(d.Direccion)
	if direccion == "" {
		return domain.Usuario{}, errors.Join(domain.ErrDatosInvalidos, errors.New("la dirección es obligatoria"))
	}
	if err := domain.ValidarPassword(d.Password); err != nil {
		return domain.Usuario{}, err
	}

	u := domain.Usuario{
		ID:        uuid.NewString(),
		Nombre:    strings.TrimSpace(d.Nombre),
		Email:     email,
		Direccion: direccion,
		Rol:       domain.RolCliente,
		Activo:    true,
		CreadoEn:  time.Now().UTC().Truncate(time.Second),
	}
	// Se valida antes de calcular el hash, que es lo caro: un formulario mal armado no
	// gasta 100 ms de CPU. PasswordHash se completa con un valor del largo correcto.
	u.PasswordHash = strings.Repeat("x", 60)
	if err := u.Validar(); err != nil {
		return domain.Usuario{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(d.Password), s.costo)
	if err != nil {
		return domain.Usuario{}, err
	}
	u.PasswordHash = string(hash)

	if err := s.repo.Crear(ctx, u); err != nil {
		return domain.Usuario{}, err
	}
	return u, nil
}
