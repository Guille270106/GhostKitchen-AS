package domain

import (
	"errors"
	"strings"
	"testing"
)

const hash = "$2a$10$abcdefghijklmnopqrstuuabcdefghijklmnopqrstuvwxyz01234" // 60 caracteres

func TestNormalizarEmail(t *testing.T) {
	casos := map[string]string{
		"  Ana@Mail.COM ":                   "ana@mail.com",
		"ana@mail.com":                      "ana@mail.com",
		"sin-arroba":                        "",
		"Ana <ana@mail.com>":                "", // con nombre no es solo un correo
		"":                                  "",
		strings.Repeat("a", 250) + "@x.com": "",
	}
	for entrada, esperado := range casos {
		if got := NormalizarEmail(entrada); got != esperado {
			t.Errorf("NormalizarEmail(%q) = %q, esperaba %q", entrada, got, esperado)
		}
	}
}

func TestValidarPorRol(t *testing.T) {
	base := Usuario{Nombre: "Ana", Email: "ana@mail.com", PasswordHash: hash}
	casos := []struct {
		nombre  string
		cambiar func(*Usuario)
		valido  bool
	}{
		{"cliente", func(u *Usuario) { u.Rol = RolCliente }, true},
		{"cliente con cocina elegida", func(u *Usuario) { u.Rol = RolCliente; u.CocinaElegidaID = "cerro" }, true},
		{"cocina con su cocina", func(u *Usuario) { u.Rol = RolCocina; u.CocinaAsignadaID = "cerro" }, true},
		{"cocina sin cocina", func(u *Usuario) { u.Rol = RolCocina }, false},
		{"admin con localidad", func(u *Usuario) { u.Rol = RolAdmin; u.Localidad = "Córdoba" }, true},
		{"admin sin localidad", func(u *Usuario) { u.Rol = RolAdmin }, false},
		{"cliente con localidad", func(u *Usuario) { u.Rol = RolCliente; u.Localidad = "Córdoba" }, false},
		{"admin que elige cocina", func(u *Usuario) { u.Rol = RolAdmin; u.Localidad = "Córdoba"; u.CocinaElegidaID = "cerro" }, false},
		{"rol inexistente", func(u *Usuario) { u.Rol = "superadmin" }, false},
		{"sin nombre", func(u *Usuario) { u.Rol = RolCliente; u.Nombre = "  " }, false},
		{"correo sin normalizar", func(u *Usuario) { u.Rol = RolCliente; u.Email = "Ana@Mail.com" }, false},
		{"sin hash", func(u *Usuario) { u.Rol = RolCliente; u.PasswordHash = "" }, false},
	}
	for _, c := range casos {
		u := base
		c.cambiar(&u)
		err := u.Validar()
		if c.valido && err != nil {
			t.Errorf("%s: esperaba válido, obtuve %v", c.nombre, err)
		}
		if !c.valido && !errors.Is(err, ErrDatosInvalidos) {
			t.Errorf("%s: esperaba ErrDatosInvalidos, obtuve %v", c.nombre, err)
		}
	}
}

func TestValidarPassword(t *testing.T) {
	casos := map[string]bool{
		"1234567":               false, // 7: corta
		"12345678":              true,
		"contraseña":            true, // la ñ cuenta como un carácter
		strings.Repeat("a", 72): true,
		strings.Repeat("a", 73): false, // bcrypt solo usa 72 bytes
		strings.Repeat("ñ", 37): false, // 74 bytes
	}
	for p, valido := range casos {
		err := ValidarPassword(p)
		if valido != (err == nil) {
			t.Errorf("ValidarPassword(%d runas, %d bytes): %v", len([]rune(p)), len(p), err)
		}
	}
}
