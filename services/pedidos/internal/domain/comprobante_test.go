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
