package services

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"ghostkitchen/usuarios/internal/domain"
)

// Emisor es quien firma los tokens: va en el campo "iss" y se exige al verificar.
const Emisor = "ghostkitchen-usuarios"

// SecretoMinimo es el largo mínimo de JWT_SECRET: con HS256 una clave corta se puede
// adivinar probando, así que el servicio no arranca con menos de 32 bytes.
const SecretoMinimo = 32

// ErrTokenInvalido es cualquier token que no sirve: mal firmado, vencido, de otro emisor o
// mal armado. No se distingue por qué, para no darle pistas a quien prueba tokens.
var ErrTokenInvalido = errors.New("token inválido o vencido")

// Identidad es lo que el token dice de la persona. Es lo que el gateway pasa a los otros
// servicios en headers (X-Usuario-Id, X-Rol, X-Cocina-Id, X-Localidad).
type Identidad struct {
	UsuarioID string
	Email     string
	Rol       domain.Rol
	CocinaID  string // solo cuenta Cocina
	Localidad string // solo admin
}

// claims es el contenido del JWT. Lleva lo mínimo para autorizar sin consultar la base:
// quién es, su rol y, según el rol, su cocina o su localidad. Nunca la contraseña ni la
// dirección.
type claims struct {
	Email     string `json:"email"`
	Rol       string `json:"rol"`
	CocinaID  string `json:"cocina,omitempty"`
	Localidad string `json:"localidad,omitempty"`
	jwt.RegisteredClaims
}

// Tokens firma y verifica los JWT con HS256 y una clave que viene de JWT_SECRET.
type Tokens struct {
	secreto  []byte
	duracion time.Duration
	ahora    func() time.Time
}

// NuevosTokens valida la configuración. Sin una clave larga, el servicio no arranca.
func NuevosTokens(secreto string, duracion time.Duration) (*Tokens, error) {
	if len(secreto) < SecretoMinimo {
		return nil, errors.New("JWT_SECRET tiene que tener al menos 32 bytes")
	}
	if duracion <= 0 {
		return nil, errors.New("la duración del token tiene que ser positiva")
	}
	return &Tokens{secreto: []byte(secreto), duracion: duracion, ahora: time.Now}, nil
}

// Emitir arma el token de una cuenta. Devuelve también cuándo vence (HU02: "un token que vence").
func (t *Tokens) Emitir(u domain.Usuario) (string, time.Time, error) {
	ahora := t.ahora().UTC().Truncate(time.Second)
	vence := ahora.Add(t.duracion)
	c := claims{
		Email: u.Email, Rol: string(u.Rol),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: Emisor, Subject: u.ID,
			IssuedAt: jwt.NewNumericDate(ahora), ExpiresAt: jwt.NewNumericDate(vence),
		},
	}
	switch u.Rol {
	case domain.RolCocina:
		c.CocinaID = u.CocinaAsignadaID
	case domain.RolAdmin:
		c.Localidad = u.Localidad
	}
	firmado, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(t.secreto)
	return firmado, vence, err
}

// Verificar comprueba firma, emisor y vencimiento, y devuelve la identidad.
//
// Solo acepta HS256: un token con "alg":"none" u otro algoritmo se rechaza, aunque diga
// ser válido. No consulta la base: el token ya trae lo necesario, y así verificar es rápido
// (el gateway espera como máximo 300 ms) y sigue andando aunque MySQL esté lento.
func (t *Tokens) Verificar(token string) (Identidad, error) {
	var c claims
	_, err := jwt.ParseWithClaims(token, &c,
		func(*jwt.Token) (any, error) { return t.secreto, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(Emisor),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithTimeFunc(t.ahora),
	)
	if err != nil {
		return Identidad{}, ErrTokenInvalido
	}
	id := Identidad{UsuarioID: c.Subject, Email: c.Email, Rol: domain.Rol(c.Rol), CocinaID: c.CocinaID, Localidad: c.Localidad}
	// Un token bien firmado igual tiene que tener sentido: rol que existe y los datos de su rol.
	if id.UsuarioID == "" || !id.Rol.Valido() ||
		(id.Rol == domain.RolCocina) != (id.CocinaID != "") ||
		(id.Rol == domain.RolAdmin) != (id.Localidad != "") {
		return Identidad{}, ErrTokenInvalido
	}
	return id, nil
}
