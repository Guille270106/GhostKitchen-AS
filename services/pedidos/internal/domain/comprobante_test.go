package domain

import (
	"testing"
	"time"
)

func TestLineasDeLaFacturaConElPrecioDelPedido(t *testing.T) {
	cliente := Actor{UsuarioID: "u-1", Rol: RolCliente}
	p, err := NuevoPedido("ped-1", cliente, "c@ejemplo.com", "cerro", []Item{
		{PlatoID: "p17", Nombre: "Burger", RestauranteID: "r5", PrecioUnitario: 12000, Cantidad: 2},
		{PlatoID: "p21", Nombre: "Pizza", RestauranteID: "r6", PrecioUnitario: 9500, Cantidad: 1},
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	lineas := p.LineasDeComprobante()
	if len(lineas) != 2 || lineas[0].Descripcion != "Burger x2" || lineas[0].Importe != 24000 || lineas[1].Importe != 9500 {
		t.Fatalf("%+v", lineas)
	}
}

// La factura incluye el envío cobrado (si quedó guardado y no es gratis), y su total coincide
// con lo que pagó el cliente.
func TestLaFacturaIncluyeElEnvio(t *testing.T) {
	cliente := Actor{UsuarioID: "u-1", Rol: RolCliente}
	p, err := NuevoPedido("ped-1", cliente, "ana@example.com", "cerro",
		[]Item{{PlatoID: "p17", Nombre: "Burger", RestauranteID: "r5", PrecioUnitario: 6000, Cantidad: 2}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	envio := int64(1300)
	p.FijarEnvio(&envio)
	lineas := p.LineasDeComprobante()
	var total int64
	for _, l := range lineas {
		total += l.Importe
	}
	if len(lineas) != 2 || lineas[1].Descripcion != "Envío" || total != *p.TotalConEnvio {
		t.Fatalf("líneas %+v, total %d", lineas, total)
	}

	gratis := int64(0)
	p.FijarEnvio(&gratis)
	if len(p.LineasDeComprobante()) != 1 {
		t.Fatal("un envío gratis no va como línea")
	}
	p.FijarEnvio(nil)
	if len(p.LineasDeComprobante()) != 1 {
		t.Fatal("sin envío registrado, solo los platos")
	}
}
