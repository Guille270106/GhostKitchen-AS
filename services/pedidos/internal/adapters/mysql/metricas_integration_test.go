package mysql_test

import (
	"context"
	"testing"
	"time"

	adapter "ghostkitchen/pedidos/internal/adapters/mysql"
	"ghostkitchen/pedidos/internal/domain"
	"github.com/google/uuid"
)

// Contra MySQL real (TEST_MYSQL=1, testdata/compose.yml): cuenta los pedidos por cocina sin los
// cancelados, los separa por día en hora de Córdoba, arma el ranking y el tiempo de entrega.
func TestMetricasDeCocina(t *testing.T) {
	db := conectar(t)
	ctx := context.Background()
	repo := adapter.PedidosMySQL{DB: db}
	cocina := "metricas-" + uuid.NewString()[:8]
	cliente := domain.Actor{UsuarioID: "u-metricas", Rol: domain.RolCliente}
	cocinaActor := domain.Actor{UsuarioID: "c-metricas", Rol: domain.RolCocina, CocinaID: cocina}

	// Domingo 8/11 a las 23:30 de Córdoba (lunes 9 a las 2:30 en UTC): cuenta para el domingo.
	domingo := time.Date(2026, 11, 9, 2, 30, 0, 0, time.UTC)
	crear := func(cuando time.Time, items ...domain.Item) domain.Pedido {
		p, err := domain.NuevoPedido(uuid.NewString(), cliente, "c@ejemplo.com", cocina, items, cuando)
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.Crear(ctx, p); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			db.Exec("DELETE FROM devolucion WHERE pedido_id=?", p.ID)
			db.Exec("DELETE FROM pedido_historial WHERE pedido_id=?", p.ID)
			db.Exec("DELETE FROM pedido_item WHERE pedido_id=?", p.ID)
			db.Exec("DELETE FROM pedido WHERE id=?", p.ID)
		})
		return p
	}
	burger := domain.Item{PlatoID: "p1", Nombre: "Mega doble", RestauranteID: "r1", RestauranteNombre: "Mostaza", PrecioUnitario: 10000, Cantidad: 2}
	sushi := domain.Item{PlatoID: "p9", Nombre: "Roll", RestauranteID: "r2", RestauranteNombre: "Sushi Club", PrecioUnitario: 5000, Cantidad: 1}

	a := crear(domingo, burger, sushi)        // 25000
	crear(domingo.Add(-48*time.Hour), burger) // viernes: 20000
	c := crear(domingo, sushi)                // se cancela: no cuenta
	// Uno viejo, de antes del período: no cuenta.
	crear(domingo.AddDate(0, 0, -10), burger)

	// a se entrega 30 minutos después; c se cancela.
	guardar := func(p *domain.Pedido, cambio domain.CambioEstado, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.GuardarCambio(ctx, *p, cambio); err != nil {
			t.Fatal(err)
		}
	}
	cambio, err := a.Aceptar(cocinaActor, domingo.Add(2*time.Minute))
	guardar(&a, cambio, err)
	for _, min := range []time.Duration{5, 15, 20} { // en preparación, listo, en camino
		cambio, err = a.Avanzar(cocinaActor, domingo.Add(min*time.Minute))
		guardar(&a, cambio, err)
	}
	cambio, err = a.ConfirmarEntrega(cliente, domingo.Add(30*time.Minute))
	guardar(&a, cambio, err)
	cambio, err = c.Cancelar(cliente, domingo.Add(time.Minute))
	guardar(&c, cambio, err)

	desde := domain.InicioMetricas(domingo)
	conteo, err := adapter.MetricasMySQL{DB: db}.PedidosPorCocina(ctx, []string{cocina, "otra"}, desde)
	if err != nil || conteo[cocina] != 2 || conteo["otra"] != 0 {
		t.Fatalf("%v %v", conteo, err)
	}
	m, porDia, err := adapter.MetricasMySQL{DB: db}.MetricasDeCocina(ctx, cocina, desde)
	if err != nil {
		t.Fatal(err)
	}
	if m.Pedidos != 2 || m.Cancelados != 1 || m.Facturacion != 45000 {
		t.Fatalf("totales: %+v", m)
	}
	if porDia["2026-11-08"] != 1 || porDia["2026-11-06"] != 1 || len(porDia) != 2 {
		t.Fatalf("por día (hora de Córdoba): %v", porDia)
	}
	if len(m.TopPlatos) != 2 || m.TopPlatos[0].PlatoID != "p1" || m.TopPlatos[0].Vendidos != 4 {
		t.Fatalf("ranking: %+v", m.TopPlatos)
	}
	if len(m.PorMarca) != 2 || m.PorMarca[0].Nombre != "Mostaza" || m.PorMarca[0].Pedidos != 2 || m.PorMarca[1].Pedidos != 1 {
		t.Fatalf("por marca: %+v", m.PorMarca)
	}
	if m.MinutosPromedio != 30 {
		t.Fatalf("minutos hasta la entrega: %d", m.MinutosPromedio)
	}
}
