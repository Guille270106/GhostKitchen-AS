package mysql_test

import (
	"context"
	"testing"
	"time"

	adapter "ghostkitchen/pedidos/internal/adapters/mysql"
	"ghostkitchen/pedidos/internal/domain"
	"github.com/google/uuid"
)

func TestMySQLConfirmacionCompleta(t *testing.T) {
	db := conectar(t)
	ctx := context.Background()
	carritos := adapter.CarritosMySQL{DB: db}
	confirmaciones := adapter.ConfirmacionesMySQL{DB: db}
	pedidos := adapter.PedidosMySQL{DB: db}

	// 1. Un cliente con un carrito reservado.
	cliente := domain.Actor{UsuarioID: "u-" + uuid.NewString()[:8], Email: "c@ejemplo.com", Rol: domain.RolCliente}
	vence := time.Now().Add(5 * time.Minute)
	c := domain.Carrito{ID: uuid.NewString(), ClienteID: cliente.UsuarioID, ReservaID: "res-1", VenceEn: &vence, Version: 1}
	_ = c.Agregar(domain.PlatoCatalogo{ID: "p17", CocinaID: "cerro", RestauranteID: "r5", Nombre: "Burger", Precio: 12000, Activo: true}, 2)
	if err := carritos.Guardar(ctx, c, 0); err != nil {
		t.Fatal(err)
	}
	conf, err := domain.NuevaConfirmacion(uuid.NewString(), uuid.NewString(), cliente, "clave-1", c, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	conf.DireccionEntrega = "Av. Vélez Sársfield 1600, 3°B"
	t.Cleanup(func() {
		db.Exec("DELETE FROM confirmacion WHERE cliente_id=?", cliente.UsuarioID)
		db.Exec("DELETE FROM comprobante WHERE pedido_id=?", conf.PedidoID)
		db.Exec("DELETE FROM pedido_historial WHERE pedido_id=?", conf.PedidoID)
		db.Exec("DELETE FROM pedido_item WHERE pedido_id=?", conf.PedidoID)
		db.Exec("DELETE FROM pedido WHERE id=?", conf.PedidoID)
		db.Exec("DELETE FROM carrito_item WHERE cliente_id=?", cliente.UsuarioID)
		db.Exec("DELETE FROM carrito WHERE cliente_id=?", cliente.UsuarioID)
	})

	// 2. Iniciar; la misma clave (o el mismo carrito con otra clave) devuelve la misma.
	if _, existia, err := confirmaciones.Iniciar(ctx, conf); err != nil || existia {
		t.Fatal(err, existia)
	}
	otra := conf
	otra.ID, otra.PedidoID, otra.Clave = uuid.NewString(), uuid.NewString(), "clave-2"
	guardada, existia, err := confirmaciones.Iniciar(ctx, otra)
	if err != nil || !existia || guardada.ID != conf.ID || len(guardada.Items) != 1 {
		t.Fatal("el mismo carrito no se confirma dos veces:", err, existia, guardada.ID)
	}
	if b, err := confirmaciones.BuscarPorClave(ctx, cliente.UsuarioID, "clave-1"); err != nil || b.ID != conf.ID {
		t.Fatal(err)
	}

	// 3. Queda como pendiente hasta que se complete.
	pendientes, _ := confirmaciones.Pendientes(ctx, time.Now().Add(time.Minute))
	encontrada := false
	for _, p := range pendientes {
		encontrada = encontrada || p.ID == conf.ID
	}
	if !encontrada {
		t.Fatal("tiene que figurar como pendiente")
	}

	// 4. Completar: pedido creado, confirmación COMPLETADA y carrito vacío, juntos.
	if err := confirmaciones.MarcarPaso(ctx, conf.ID, domain.PasoCrearPedido); err != nil {
		t.Fatal(err)
	}
	pedido, _ := conf.CrearPedido(time.Now())
	if err := confirmaciones.Completar(ctx, conf, pedido); err != nil {
		t.Fatal(err)
	}
	if err := confirmaciones.Completar(ctx, conf, pedido); err != nil {
		t.Fatal("completar dos veces no hace nada:", err)
	}
	leida, _ := confirmaciones.Obtener(ctx, conf.ID)
	creado, err := pedidos.Obtener(ctx, conf.PedidoID)
	vacio, _ := carritos.Obtener(ctx, cliente.UsuarioID)
	if leida.Estado != domain.ConfirmacionCompletada || err != nil || creado.Total != 24000 || !vacio.Vacio() || vacio.ID == c.ID {
		t.Fatal(leida.Estado, err, creado.Total, vacio)
	}
	// La dirección de entrega se guarda en la confirmación y llega al pedido (con tildes y °).
	if leida.DireccionEntrega != conf.DireccionEntrega || creado.DireccionEntrega != conf.DireccionEntrega {
		t.Fatalf("dirección: confirmación %q, pedido %q", leida.DireccionEntrega, creado.DireccionEntrega)
	}
}
