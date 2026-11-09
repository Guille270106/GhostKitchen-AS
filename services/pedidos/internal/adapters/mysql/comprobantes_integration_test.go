package mysql_test

import (
	"context"
	"testing"
	"time"

	adapter "ghostkitchen/pedidos/internal/adapters/mysql"
	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/ports"
	"github.com/google/uuid"
)

// Completar la confirmación deja la factura PENDIENTE en la misma transacción; el detalle del
// pedido muestra cómo va, y al emitirla figura con su número.
func TestMySQLLaFacturaQuedaPendienteYDespuesEmitida(t *testing.T) {
	db := conectar(t)
	ctx := context.Background()
	confirmaciones := adapter.ConfirmacionesMySQL{DB: db}
	pedidos := adapter.PedidosMySQL{DB: db}
	comprobantes := adapter.ComprobantesMySQL{DB: db}

	// 1. Un pedido creado por una confirmación.
	cliente := domain.Actor{UsuarioID: "u-" + uuid.NewString()[:8], Email: "c@ejemplo.com", Rol: domain.RolCliente}
	vence := time.Now().Add(5 * time.Minute)
	c := domain.Carrito{ID: uuid.NewString(), ClienteID: cliente.UsuarioID, ReservaID: "res-1", VenceEn: &vence, Version: 1}
	_ = c.Agregar(domain.PlatoCatalogo{ID: "p17", CocinaID: "cerro", RestauranteID: "r5", Nombre: "Burger", Precio: 12000, Activo: true}, 1)
	conf, err := domain.NuevaConfirmacion(uuid.NewString(), uuid.NewString(), cliente, "clave-1", c, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM comprobante WHERE pedido_id=?", conf.PedidoID)
		db.Exec("DELETE FROM confirmacion WHERE cliente_id=?", cliente.UsuarioID)
		db.Exec("DELETE FROM pedido_historial WHERE pedido_id=?", conf.PedidoID)
		db.Exec("DELETE FROM pedido_item WHERE pedido_id=?", conf.PedidoID)
		db.Exec("DELETE FROM pedido WHERE id=?", conf.PedidoID)
	})
	if _, _, err := confirmaciones.Iniciar(ctx, conf); err != nil {
		t.Fatal(err)
	}
	pedido, _ := conf.CrearPedido(time.Now())
	if err := confirmaciones.Completar(ctx, conf, pedido); err != nil {
		t.Fatal(err)
	}

	// 2. La factura quedó pendiente y el detalle del pedido lo muestra.
	if !contiene(t, comprobantes, conf.PedidoID) {
		t.Fatal("la factura tiene que quedar pendiente")
	}
	leido, err := pedidos.Obtener(ctx, conf.PedidoID)
	if err != nil || leido.Comprobante == nil || leido.Comprobante.Estado != domain.ComprobantePendiente {
		t.Fatal(err, leido.Comprobante)
	}

	// 3. Un intento sin respuesta la deja pendiente; emitida, ya no figura y muestra el número.
	if err := comprobantes.SumarIntento(ctx, conf.PedidoID); err != nil || !contiene(t, comprobantes, conf.PedidoID) {
		t.Fatal("sigue pendiente después de un intento:", err)
	}
	if err := comprobantes.MarcarEmitido(ctx, conf.PedidoID, ports.ComprobanteEmitido{ID: "cmp-1", Numero: "B-0001-00000042"}); err != nil {
		t.Fatal(err)
	}
	if contiene(t, comprobantes, conf.PedidoID) {
		t.Fatal("emitida no tiene que figurar como pendiente")
	}
	leido, _ = pedidos.Obtener(ctx, conf.PedidoID)
	if leido.Comprobante == nil || leido.Comprobante.Estado != domain.ComprobanteEmitido || leido.Comprobante.Numero != "B-0001-00000042" {
		t.Fatal(leido.Comprobante)
	}
}

func contiene(t *testing.T, repo adapter.ComprobantesMySQL, pedidoID string) bool {
	t.Helper()
	ids, err := repo.Pendientes(context.Background(), 100000)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if id == pedidoID {
			return true
		}
	}
	return false
}
