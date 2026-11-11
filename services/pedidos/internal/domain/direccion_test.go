package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNormalizarDireccion(t *testing.T) {
	buenas := map[string]string{
		"Obispo Trejo 242":                   "Obispo Trejo 242",
		"  Av. Vélez   Sarsfield 1600, 3°B ": "Av. Vélez Sarsfield 1600, 3°B",
		"Bv. San Juan 50\nTimbre roto":       "Bv. San Juan 50 Timbre roto",
		strings.Repeat("ñ", MaxDireccion):    strings.Repeat("ñ", MaxDireccion),
	}
	for entrada, quiero := range buenas {
		got, err := NormalizarDireccion(entrada)
		if err != nil || got != quiero {
			t.Errorf("%q: %q, %v; esperaba %q", entrada, got, err, quiero)
		}
	}
	malas := []string{"", "   ", "\n\t", strings.Repeat("a", MaxDireccion+1), "Calle\x00 1", "Calle \x1b[31m1", string([]byte{0xff, 'a'})}
	for _, entrada := range malas {
		if _, err := NormalizarDireccion(entrada); !errors.Is(err, ErrDireccionInvalida) {
			t.Errorf("%q: esperaba ErrDireccionInvalida, fue %v", entrada, err)
		}
	}
}

func TestElPedidoLlevaLaDireccionDeLaConfirmacion(t *testing.T) {
	conf, err := NuevaConfirmacion("conf-1", "ped-1", clienteConMail, "clave-1", carritoListo(t), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	conf.DireccionEntrega = "Obispo Trejo 242"
	p, err := conf.CrearPedido(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if p.DireccionEntrega != "Obispo Trejo 242" {
		t.Fatalf("dirección del pedido: %q", p.DireccionEntrega)
	}
}
