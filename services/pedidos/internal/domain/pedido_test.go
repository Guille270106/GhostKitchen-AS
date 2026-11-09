package domain

import (
	"errors"
	"testing"
	"time"
)

var (
	ahora   = time.Date(2026, 11, 8, 21, 0, 0, 0, time.UTC)
	cliente = Actor{UsuarioID: "u-cliente", Rol: RolCliente}
	otroCli = Actor{UsuarioID: "u-otro", Rol: RolCliente}
	cocina  = Actor{UsuarioID: "u-cocina", Rol: RolCocina, CocinaID: "cerro"}
	ajena   = Actor{UsuarioID: "u-ajena", Rol: RolCocina, CocinaID: "nueva-cordoba"}
	admin   = Actor{UsuarioID: "u-admin", Rol: RolAdmin}
)

func nuevo(t *testing.T) Pedido {
	t.Helper()
	p, err := NuevoPedido("ped-1", cliente, "cliente@ejemplo.com", "cerro", []Item{
		{PlatoID: "p17", Nombre: "Burger", RestauranteID: "r5", PrecioUnitario: 12000, Cantidad: 2},
		{PlatoID: "p21", Nombre: "Pizza", RestauranteID: "r6", PrecioUnitario: 9500, Cantidad: 1},
	}, ahora)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestNuevoPedidoArrancaRecibidoConTotalEHistorial(t *testing.T) {
	p := nuevo(t)
	if p.Estado != EstadoRecibido || p.Version != 1 || p.Total != 33500 || p.Moneda != "ARS" {
		t.Fatalf("%+v", p)
	}
	if len(p.Historial) != 1 || p.Historial[0].EstadoNuevo != EstadoRecibido || p.Historial[0].EstadoAnterior != "" {
		t.Fatalf("historial inicial: %+v", p.Historial)
	}
}

func TestNuevoPedidoRechazaDatosInvalidos(t *testing.T) {
	item := Item{PlatoID: "p1", Nombre: "Burger", RestauranteID: "r1", PrecioUnitario: 100, Cantidad: 1}
	casos := map[string]struct {
		actor Actor
		email string
		items []Item
		err   error
	}{
		"sin items":         {cliente, "a@b.com", nil, ErrDatosInvalidos},
		"cantidad cero":     {cliente, "a@b.com", []Item{{PlatoID: "p1", Nombre: "x", RestauranteID: "r1", Cantidad: 0}}, ErrDatosInvalidos},
		"precio negativo":   {cliente, "a@b.com", []Item{{PlatoID: "p1", Nombre: "x", RestauranteID: "r1", Cantidad: 1, PrecioUnitario: -1}}, ErrDatosInvalidos},
		"plato repetido":    {cliente, "a@b.com", []Item{item, item}, ErrDatosInvalidos},
		"mail inválido":     {cliente, "sin-arroba", []Item{item}, ErrDatosInvalidos},
		"sin identidad":     {Actor{}, "a@b.com", []Item{item}, ErrSinIdentidad},
		"lo crea la cocina": {cocina, "a@b.com", []Item{item}, ErrSinPermiso},
	}
	for nombre, c := range casos {
		if _, err := NuevoPedido("ped-1", c.actor, c.email, "cerro", c.items, ahora); !errors.Is(err, c.err) {
			t.Errorf("%s: esperaba %v, obtuvo %v", nombre, c.err, err)
		}
	}
}

func TestRecorridoCompletoHastaEntregado(t *testing.T) {
	p := nuevo(t)
	pasos := []struct {
		accion func(*Pedido) (CambioEstado, error)
		estado Estado
	}{
		{func(p *Pedido) (CambioEstado, error) { return p.Aceptar(cocina, ahora) }, EstadoAceptado},
		{func(p *Pedido) (CambioEstado, error) { return p.Avanzar(cocina, ahora) }, EstadoEnPreparacion},
		{func(p *Pedido) (CambioEstado, error) { return p.Avanzar(cocina, ahora) }, EstadoListo},
		{func(p *Pedido) (CambioEstado, error) { return p.Avanzar(cocina, ahora) }, EstadoEnCamino},
		{func(p *Pedido) (CambioEstado, error) { return p.ConfirmarEntrega(cliente, ahora) }, EstadoEntregado},
	}
	for i, paso := range pasos {
		anterior := p.Estado
		c, err := paso.accion(&p)
		if err != nil {
			t.Fatalf("paso %d: %v", i, err)
		}
		if p.Estado != paso.estado || c.EstadoAnterior != anterior || c.EstadoNuevo != paso.estado || c.Version != int64(i+2) || p.Version != c.Version {
			t.Fatalf("paso %d: estado %s, cambio %+v", i, p.Estado, c)
		}
	}
	if len(p.Historial) != 6 || !p.Estado.Final() {
		t.Fatalf("historial %d, final %v", len(p.Historial), p.Estado.Final())
	}
	if _, err := p.Avanzar(cocina, ahora); !errors.Is(err, ErrTransicionInvalida) {
		t.Fatal("un pedido entregado no avanza más:", err)
	}
}

func TestCancelarYRechazarSoloDesdeRecibido(t *testing.T) {
	p := nuevo(t)
	if _, err := p.Cancelar(cliente, ahora); err != nil || p.Estado != EstadoCancelado {
		t.Fatal("el cliente cancela desde RECIBIDO:", err)
	}

	p = nuevo(t)
	if _, err := p.Rechazar(cocina, ahora); err != nil || p.Estado != EstadoCancelado {
		t.Fatal("la cocina rechaza desde RECIBIDO y queda CANCELADO:", err)
	}

	p = nuevo(t)
	if _, err := p.Aceptar(cocina, ahora); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Cancelar(cliente, ahora); !errors.Is(err, ErrTransicionInvalida) {
		t.Fatal("no se cancela después de aceptar:", err)
	}
	if _, err := p.Rechazar(cocina, ahora); !errors.Is(err, ErrTransicionInvalida) {
		t.Fatal("no se rechaza después de aceptar:", err)
	}
	if p.Estado != EstadoAceptado || p.Version != 2 {
		t.Fatal("una acción rechazada no cambia el pedido:", p.Estado, p.Version)
	}
}

func TestTransicionesInvalidas(t *testing.T) {
	p := nuevo(t)
	if _, err := p.Avanzar(cocina, ahora); !errors.Is(err, ErrTransicionInvalida) {
		t.Fatal("RECIBIDO no avanza sin aceptar:", err)
	}
	if _, err := p.ConfirmarEntrega(cliente, ahora); !errors.Is(err, ErrTransicionInvalida) {
		t.Fatal("no se confirma la entrega antes de EN_CAMINO:", err)
	}
	if p.Version != 1 || len(p.Historial) != 1 {
		t.Fatal("las acciones inválidas no dejan rastro")
	}
}

func TestPermisos(t *testing.T) {
	p := nuevo(t)
	if _, err := p.Aceptar(cliente, ahora); !errors.Is(err, ErrSinPermiso) {
		t.Error("un cliente no acepta pedidos:", err)
	}
	if _, err := p.Aceptar(admin, ahora); !errors.Is(err, ErrSinPermiso) {
		t.Error("el admin no maneja pedidos:", err)
	}
	if _, err := p.Aceptar(ajena, ahora); !errors.Is(err, ErrNoEncontrado) {
		t.Error("otra cocina no ve el pedido (404):", err)
	}
	if _, err := p.Cancelar(otroCli, ahora); !errors.Is(err, ErrNoEncontrado) {
		t.Error("otro cliente no ve el pedido (404):", err)
	}
	if _, err := p.Cancelar(cocina, ahora); !errors.Is(err, ErrSinPermiso) {
		t.Error("la cocina no cancela como cliente:", err)
	}
	if _, err := p.Aceptar(Actor{Rol: RolCocina, CocinaID: "cerro"}, ahora); !errors.Is(err, ErrSinIdentidad) {
		t.Error("sin usuario no hay identidad:", err)
	}
	if !p.PuedeVer(cliente) || !p.PuedeVer(cocina) || p.PuedeVer(otroCli) || p.PuedeVer(ajena) || p.PuedeVer(admin) {
		t.Error("PuedeVer: el cliente ve los suyos y la cocina los de su cocina")
	}
}
