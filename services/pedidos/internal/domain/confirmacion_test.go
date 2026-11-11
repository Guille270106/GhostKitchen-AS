package domain

import (
	"errors"
	"testing"
	"time"
)

func carritoListo(t *testing.T) Carrito {
	t.Helper()
	vence := time.Now().Add(5 * time.Minute)
	c := Carrito{ID: "car-1", ReservaID: "res-1", VenceEn: &vence}
	if err := c.Agregar(burger, 2); err != nil {
		t.Fatal(err)
	}
	if err := c.Agregar(pizza, 1); err != nil {
		t.Fatal(err)
	}
	return c
}

var clienteConMail = Actor{UsuarioID: "u-cliente", Email: "cliente@ejemplo.com", Rol: RolCliente}

func TestNuevaConfirmacionCopiaElCarrito(t *testing.T) {
	conf, err := NuevaConfirmacion("conf-1", "ped-1", clienteConMail, "clave-1", carritoListo(t), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if conf.Estado != ConfirmacionEnCurso || conf.Paso != PasoDescontar || conf.ReservaID != "res-1" ||
		conf.CocinaID != "cerro" || len(conf.Items) != 2 || conf.ClienteEmail != "cliente@ejemplo.com" {
		t.Fatalf("%+v", conf)
	}

	// Con esa copia se arma el pedido en RECIBIDO, con el total y el mail del cliente.
	pedido, err := conf.CrearPedido(time.Now())
	if err != nil || pedido.ID != "ped-1" || pedido.Estado != EstadoRecibido || pedido.Total != 2*12000+9500 || pedido.ClienteEmail != "cliente@ejemplo.com" {
		t.Fatal(err, pedido)
	}
}

func TestNoSeConfirmaSinPlatosVencidoOSinClave(t *testing.T) {
	vacio := Carrito{ID: "car-1"}
	if _, err := NuevaConfirmacion("c", "p", clienteConMail, "k", vacio, time.Now()); !errors.Is(err, ErrCarritoVacio) {
		t.Error("carrito vacío:", err)
	}

	vencido := carritoListo(t)
	paso := time.Now().Add(-time.Second)
	vencido.VenceEn = &paso
	if _, err := NuevaConfirmacion("c", "p", clienteConMail, "k", vencido, time.Now()); !errors.Is(err, ErrCarritoVencido) {
		t.Error("reserva vencida:", err)
	}

	for _, clave := range []string{"", "con espacios", string(make([]byte, 129))} {
		if _, err := NuevaConfirmacion("c", "p", clienteConMail, clave, carritoListo(t), time.Now()); !errors.Is(err, ErrClaveInvalida) {
			t.Errorf("clave %q: %v", clave, err)
		}
	}

	cocina := Actor{UsuarioID: "u-c", Rol: RolCocina, CocinaID: "cerro"}
	if _, err := NuevaConfirmacion("c", "p", cocina, "k", carritoListo(t), time.Now()); !errors.Is(err, ErrSinPermiso) {
		t.Error("solo el cliente confirma:", err)
	}
}
