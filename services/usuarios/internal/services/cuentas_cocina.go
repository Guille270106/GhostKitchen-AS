package services

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"ghostkitchen/usuarios/internal/clients"
	"ghostkitchen/usuarios/internal/domain"
)

// Catalogo es lo que este servicio necesita de Catálogo. La implementación HTTP está en
// clients; los tests usan una falsa.
type Catalogo interface {
	Cocina(ctx context.Context, id, correlationID string) (clients.Cocina, error)
}

var (
	// ErrSinPermiso: la persona está identificada pero su rol no puede hacer esto.
	ErrSinPermiso = errors.New("solo un admin puede crear cuentas Cocina")
	// ErrOtraLocalidad: la cocina existe pero es de otra localidad que la del admin.
	ErrOtraLocalidad = errors.New("la cocina es de otra localidad")
	// ErrCocinaInvalida: Catálogo dice que la cocina no existe o está inactiva.
	ErrCocinaInvalida = errors.New("la cocina no existe o está inactiva")
	// ErrCatalogoNoDisponible: Catálogo no respondió. Se impide el alta: no se puede
	// comprobar la localidad, y no se toma la falta de respuesta como "no existe".
	ErrCatalogoNoDisponible = errors.New("no se pudo comprobar la cocina en Catálogo")
)

// DatosCuentaCocina es lo que manda el admin para crear la cuenta de una cocina (HU15).
type DatosCuentaCocina struct {
	Nombre   string
	Email    string
	Password string
	CocinaID string
}

// ConCatalogo conecta el cliente de Catálogo. Sin él, crear cuentas Cocina da
// ErrCatalogoNoDisponible.
func (s *Usuarios) ConCatalogo(c Catalogo) *Usuarios { s.catalogo = c; return s }

// CrearCuentaCocina crea una cuenta Cocina para una cocina de la localidad del admin (HU15).
//
// Primero se validan los datos (no cuesta nada), después se le pregunta a Catálogo y recién
// al final se calcula el hash y se guarda. Si Catálogo no responde, no se crea la cuenta.
func (s *Usuarios) CrearCuentaCocina(ctx context.Context, admin Identidad, d DatosCuentaCocina, correlationID string) (domain.Usuario, error) {
	if admin.Rol != domain.RolAdmin || admin.Localidad == "" {
		return domain.Usuario{}, ErrSinPermiso
	}
	cocinaID := strings.TrimSpace(d.CocinaID)
	u := domain.Usuario{
		ID:               uuid.NewString(),
		Nombre:           strings.TrimSpace(d.Nombre),
		Email:            domain.NormalizarEmail(d.Email),
		Rol:              domain.RolCocina,
		CocinaAsignadaID: cocinaID,
		Activo:           true,
		CreadoEn:         time.Now().UTC().Truncate(time.Second),
	}
	if u.Email == "" {
		return domain.Usuario{}, errors.Join(domain.ErrDatosInvalidos, errors.New("el correo no es válido"))
	}
	if cocinaID == "" || len(cocinaID) > 64 {
		return domain.Usuario{}, errors.Join(domain.ErrDatosInvalidos, errors.New("falta la cocina"))
	}
	if err := validarAntesDelHash(u, d.Password); err != nil {
		return domain.Usuario{}, err
	}

	if s.catalogo == nil {
		return domain.Usuario{}, ErrCatalogoNoDisponible
	}
	cocina, err := s.catalogo.Cocina(ctx, cocinaID, correlationID)
	switch {
	case errors.Is(err, clients.ErrCocinaNoExiste):
		return domain.Usuario{}, ErrCocinaInvalida
	case err != nil:
		return domain.Usuario{}, errors.Join(ErrCatalogoNoDisponible, err)
	case !cocina.Activa:
		return domain.Usuario{}, ErrCocinaInvalida
	case !mismaLocalidad(cocina.Localidad, admin.Localidad):
		return domain.Usuario{}, ErrOtraLocalidad
	}

	return s.guardarConHash(ctx, u, d.Password)
}

// mismaLocalidad compara sin espacios de más ni mayúsculas: "Nueva Córdoba" = "nueva córdoba".
func mismaLocalidad(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// AdminInicial es una cuenta de admin que se carga al iniciar el sistema.
type AdminInicial struct{ Nombre, Email, Localidad string }

// SembrarAdmins crea las cuentas de admin que todavía no existen, todas con la misma
// contraseña inicial. Se puede correr en cada arranque: si el correo ya existe, no lo toca
// (tampoco le cambia la contraseña). Devuelve cuántas creó.
func (s *Usuarios) SembrarAdmins(ctx context.Context, password string, admins []AdminInicial) (int, error) {
	creados := 0
	for _, a := range admins {
		u := domain.Usuario{
			ID: uuid.NewString(), Nombre: a.Nombre, Email: domain.NormalizarEmail(a.Email),
			Rol: domain.RolAdmin, Localidad: a.Localidad, Activo: true,
			CreadoEn: time.Now().UTC().Truncate(time.Second),
		}
		if err := validarAntesDelHash(u, password); err != nil {
			return creados, err
		}
		if _, err := s.repo.PorEmail(ctx, u.Email); err == nil {
			continue
		} else if !errors.Is(err, domain.ErrNoEncontrado) {
			return creados, err
		}
		_, err := s.guardarConHash(ctx, u, password)
		if errors.Is(err, domain.ErrEmailRepetido) {
			continue // otra copia del servicio lo creó al mismo tiempo
		}
		if err != nil {
			return creados, err
		}
		creados++
	}
	return creados, nil
}
