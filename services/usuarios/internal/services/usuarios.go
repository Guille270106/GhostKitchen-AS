// Package services tiene las reglas de Usuarios. Los handlers lo llaman y él usa el
// repositorio; no sabe nada de HTTP ni de MySQL (handler, servicio, repositorio).
package services

import (
	"context"
	"errors"
	"strings"
	"sync"
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
	repo     repositories.Usuarios
	tokens   *Tokens
	catalogo Catalogo
	costo    int
	// hashFalso se compara cuando el correo no existe, así el login tarda lo mismo con un
	// correo inexistente que con una contraseña mal: por el tiempo no se sabe qué correos
	// tienen cuenta.
	hashFalso       []byte
	hashFalsoUnaVez sync.Once
}

// Nuevo crea el servicio sobre un repositorio y el firmador de tokens.
func Nuevo(repo repositories.Usuarios, tokens *Tokens) *Usuarios {
	return &Usuarios{repo: repo, tokens: tokens, costo: CostoBcrypt}
}

// ConCostoBcrypt cambia el costo. Los tests usan bcrypt.MinCost para correr rápido.
// Se llama al armar el servicio, antes de atender solicitudes.
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
	u := domain.Usuario{
		ID:        uuid.NewString(),
		Nombre:    strings.TrimSpace(d.Nombre),
		Email:     email,
		Direccion: direccion,
		Rol:       domain.RolCliente,
		Activo:    true,
		CreadoEn:  time.Now().UTC().Truncate(time.Second),
	}
	if err := validarAntesDelHash(u, d.Password); err != nil {
		return domain.Usuario{}, err
	}
	return s.guardarConHash(ctx, u, d.Password)
}

// validarAntesDelHash revisa la contraseña y la cuenta antes de calcular el hash, que es lo
// caro: un formulario mal armado no gasta 100 ms de CPU. Para validar la cuenta se usa un
// hash de relleno del largo correcto.
func validarAntesDelHash(u domain.Usuario, password string) error {
	if err := domain.ValidarPassword(password); err != nil {
		return err
	}
	u.PasswordHash = strings.Repeat("x", 60)
	return u.Validar()
}

// guardarConHash calcula el hash bcrypt y guarda la cuenta.
func (s *Usuarios) guardarConHash(ctx context.Context, u domain.Usuario, password string) (domain.Usuario, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.costo)
	if err != nil {
		return domain.Usuario{}, err
	}
	u.PasswordHash = string(hash)
	if err := s.repo.Crear(ctx, u); err != nil {
		return domain.Usuario{}, err
	}
	return u, nil
}

// ErrCredencialesInvalidas es el único error del login con datos mal: no dice si el correo
// no existe o si la contraseña está mal (HU02).
var ErrCredencialesInvalidas = errors.New("correo o contraseña incorrectos")

// Sesion es lo que recibe la persona al entrar: el token, cuándo vence y su cuenta.
type Sesion struct {
	Token   string         `json:"token"`
	Tipo    string         `json:"tipo"`
	VenceEn time.Time      `json:"venceEn"`
	Usuario domain.Usuario `json:"usuario"`
}

// Login comprueba correo y contraseña de cualquier rol y devuelve un token (HU02).
func (s *Usuarios) Login(ctx context.Context, email, password string) (Sesion, error) {
	u, err := s.repo.PorEmail(ctx, domain.NormalizarEmail(email))
	switch {
	case errors.Is(err, domain.ErrNoEncontrado):
		s.compararFalso(password)
		return Sesion{}, ErrCredencialesInvalidas
	case err != nil:
		return Sesion{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil || !u.Activo {
		return Sesion{}, ErrCredencialesInvalidas
	}
	token, vence, err := s.tokens.Emitir(u)
	if err != nil {
		return Sesion{}, err
	}
	return Sesion{Token: token, Tipo: "Bearer", VenceEn: vence, Usuario: u}, nil
}

// Verificar valida un token para el gateway (G9).
func (s *Usuarios) Verificar(token string) (Identidad, error) { return s.tokens.Verificar(token) }

// compararFalso hace el mismo trabajo que una comparación real contra un hash con el mismo
// costo, y descarta el resultado. El hash falso se calcula una sola vez (sync.Once), así dos
// logins que llegan juntos no lo calculan a la vez.
func (s *Usuarios) compararFalso(password string) {
	s.hashFalsoUnaVez.Do(func() {
		s.hashFalso, _ = bcrypt.GenerateFromPassword([]byte("no-es-una-cuenta"), s.costo)
	})
	_ = bcrypt.CompareHashAndPassword(s.hashFalso, []byte(password))
}
