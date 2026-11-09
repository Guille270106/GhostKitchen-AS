package mysql_test

import (
	"context"
	"testing"
	"time"

	adapter "ghostkitchen/pedidos/internal/adapters/mysql"
	"ghostkitchen/pedidos/internal/application"
	"ghostkitchen/pedidos/internal/domain"
	"github.com/google/uuid"
)

// Cancelar un pedido confirmado anota la devolución de ingredientes en la misma transacción;
// después se marca hecha y ya no figura como pendiente.
func TestMySQLCancelarAnotaLaDevolucion(t *testing.T) {
	db := conectar(t)
	ctx := context.Background()
	confirmaciones := adapter.ConfirmacionesMySQL{DB: db}
	pedidos := adapter.PedidosMySQL{DB: db}
	devoluciones := adapter.DevolucionesMySQL{DB: db}
	svc := application.PedidosService{Repo: pedidos}

	// 1. Un pedido creado por una confirmación (con su reserva).
	cliente := domain.Actor{UsuarioID: "u-" + uuid.NewString()[:8], Email: "c@ejemplo.com", Rol: domain.RolCliente}
	vence := time.Now().Add(5 * time.Minute)
	reserva := "res-" + uuid.NewString()[:8] // la reserva del carrito (para crear la confirmación)
	c := domain.Carrito{ID: uuid.NewString(), ClienteID: cliente.UsuarioID, ReservaID: reserva, VenceEn: &vence, Version: 1}
	_ = c.Agregar(domain.PlatoCatalogo{ID: "p17", CocinaID: "cerro", RestauranteID: "r5", Nombre: "Burger", Precio: 12000, Activo: true}, 1)
	conf, err := domain.NuevaConfirmacion(uuid.NewString(), uuid.NewString(), cliente, "clave-1", c, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM devolucion WHERE pedido_id=?", conf.PedidoID)
		db.Exec("DELETE FROM confirmacion WHERE cliente_id=?", cliente.UsuarioID)
		db.Exec("DELETE FROM pedido_historial WHERE pedido_id=?", conf.PedidoID)
		db.Exec("DELETE FROM pedido_item WHERE pedido_id=?", conf.PedidoID)
		db.Exec("DELETE FROM pedido WHERE id=?", conf.PedidoID)
		db.Exec("DELETE FROM carrito_item WHERE cliente_id=?", cliente.UsuarioID)
		db.Exec("DELETE FROM carrito WHERE cliente_id=?", cliente.UsuarioID)
	})
	if _, _, err := confirmaciones.Iniciar(ctx, conf); err != nil {
		t.Fatal(err)
	}
	pedido, _ := conf.CrearPedido(time.Now())
	if err := confirmaciones.Completar(ctx, conf, pedido); err != nil {
		t.Fatal(err)
	}

	// 2. El cliente lo cancela: queda la devolución pendiente.
	if _, err := svc.CambiarEstado(ctx, cliente, conf.PedidoID, application.Cancelar); err != nil {
		t.Fatal(err)
	}
	if !estaPendiente(t, devoluciones, conf.PedidoID) {
		t.Fatal("la devolución tiene que quedar pendiente")
	}

	// 3. Un intento fallido la deja pendiente; marcarla hecha la saca de la lista.
	if err := devoluciones.SumarIntento(ctx, conf.PedidoID); err != nil {
		t.Fatal(err)
	}
	if !estaPendiente(t, devoluciones, conf.PedidoID) {
		t.Fatal("después de un intento fallido sigue pendiente")
	}
	if err := devoluciones.MarcarHecha(ctx, conf.PedidoID); err != nil {
		t.Fatal(err)
	}
	if estaPendiente(t, devoluciones, conf.PedidoID) {
		t.Fatal("ya hecha no tiene que figurar como pendiente")
	}
}

func estaPendiente(t *testing.T, repo adapter.DevolucionesMySQL, pedidoID string) bool {
	t.Helper()
	pendientes, err := repo.Pendientes(context.Background(), 1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range pendientes {
		if d.PedidoID == pedidoID {
			return true
		}
	}
	return false
}
