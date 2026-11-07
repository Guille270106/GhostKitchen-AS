package services

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"ghostkitchen/usuarios/internal/domain"
)

func tokensFijos(t *testing.T, ahora time.Time) *Tokens {
	t.Helper()
	tk, err := NuevosTokens(secretoDePrueba, 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	tk.ahora = func() time.Time { return ahora }
	return tk
}

func TestSecretoCortoNoArranca(t *testing.T) {
	if _, err := NuevosTokens("corto", time.Hour); err == nil {
		t.Fatal("con un JWT_SECRET de menos de 32 bytes no tiene que arrancar")
	}
	if _, err := NuevosTokens(secretoDePrueba, 0); err == nil {
		t.Fatal("con duración 0 no tiene que arrancar")
	}
}

func TestEmitirYVerificarCadaRol(t *testing.T) {
	ahora := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	tk := tokensFijos(t, ahora)
	casos := []domain.Usuario{
		{ID: "u1", Email: "ana@mail.com", Rol: domain.RolCliente, CocinaElegidaID: "cerro"},
		{ID: "u2", Email: "cocina@gk.com", Rol: domain.RolCocina, CocinaAsignadaID: "nueva-cordoba"},
		{ID: "u3", Email: "admin@gk.com", Rol: domain.RolAdmin, Localidad: "Nueva Córdoba"},
	}
	for _, u := range casos {
		token, vence, err := tk.Emitir(u)
		if err != nil {
			t.Fatal(err)
		}
		if !vence.Equal(ahora.Add(2 * time.Hour)) {
			t.Fatalf("vence %v, esperaba 2 h después", vence)
		}
		id, err := tk.Verificar(token)
		if err != nil {
			t.Fatalf("%s: %v", u.Rol, err)
		}
		if id.UsuarioID != u.ID || id.Rol != u.Rol || id.Email != u.Email || id.CocinaID != u.CocinaAsignadaID || id.Localidad != u.Localidad {
			t.Fatalf("%s: identidad inesperada %+v", u.Rol, id)
		}
	}
}

func TestElTokenNoLlevaDatosDeMas(t *testing.T) {
	tk := tokensFijos(t, time.Now())
	u := domain.Usuario{ID: "u1", Email: "ana@mail.com", Rol: domain.RolCliente, PasswordHash: "$2a$10$secreto", Direccion: "Obispo Trejo 323"}
	token, _, _ := tk.Emitir(u)
	carga, _ := base64.RawURLEncoding.DecodeString(strings.Split(token, ".")[1])
	for _, prohibido := range []string{"secreto", "Obispo", "password", "direccion"} {
		if strings.Contains(string(carga), prohibido) {
			t.Fatalf("el token no puede llevar %q: %s", prohibido, carga)
		}
	}
}

func TestTokensQueNoSirven(t *testing.T) {
	ahora := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	tk := tokensFijos(t, ahora)
	bueno, _, _ := tk.Emitir(domain.Usuario{ID: "u1", Email: "a@b.com", Rol: domain.RolCliente})

	vencido := tokensFijos(t, ahora.Add(3*time.Hour)) // pasaron más de 2 h
	otraClave, _ := NuevosTokens("otra-clave-distinta-de-32-bytes!!!", time.Hour)

	firmar := func(metodo jwt.SigningMethod, clave any, c claims) string {
		s, err := jwt.NewWithClaims(metodo, c).SignedString(clave)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	base := jwt.RegisteredClaims{Issuer: Emisor, Subject: "u1", IssuedAt: jwt.NewNumericDate(ahora), ExpiresAt: jwt.NewNumericDate(ahora.Add(time.Hour))}
	sinVencimiento := base
	sinVencimiento.ExpiresAt = nil
	otroEmisor := base
	otroEmisor.Issuer = "alguien"

	casos := map[string]func() error{
		"vencido":          func() error { _, err := vencido.Verificar(bueno); return err },
		"firmado con otra": func() error { _, err := otraClave.Verificar(bueno); return err },
		"alterado":         func() error { _, err := tk.Verificar(bueno[:len(bueno)-2] + "xx"); return err },
		"alg none": func() error {
			_, err := tk.Verificar(firmar(jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, claims{Rol: "admin", Localidad: "X", RegisteredClaims: base}))
			return err
		},
		"sin vencimiento": func() error {
			_, err := tk.Verificar(firmar(jwt.SigningMethodHS256, tk.secreto, claims{Rol: "cliente", RegisteredClaims: sinVencimiento}))
			return err
		},
		"otro emisor": func() error {
			_, err := tk.Verificar(firmar(jwt.SigningMethodHS256, tk.secreto, claims{Rol: "cliente", RegisteredClaims: otroEmisor}))
			return err
		},
		"rol inventado": func() error {
			_, err := tk.Verificar(firmar(jwt.SigningMethodHS256, tk.secreto, claims{Rol: "root", RegisteredClaims: base}))
			return err
		},
		"cocina sin cocina": func() error {
			_, err := tk.Verificar(firmar(jwt.SigningMethodHS256, tk.secreto, claims{Rol: "cocina", RegisteredClaims: base}))
			return err
		},
		"basura": func() error { _, err := tk.Verificar("no.es.token"); return err },
	}
	for nombre, verificar := range casos {
		if err := verificar(); !errors.Is(err, ErrTokenInvalido) {
			t.Errorf("%s: esperaba ErrTokenInvalido, obtuve %v", nombre, err)
		}
	}
}
