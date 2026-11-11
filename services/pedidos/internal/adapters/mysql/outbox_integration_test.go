package mysql_test

import (
	"context"
	"encoding/json"
	"testing"

	adapter "ghostkitchen/pedidos/internal/adapters/mysql"
	"ghostkitchen/pedidos/internal/application"
	"ghostkitchen/pedidos/internal/domain"
)

// Crear el pedido y cada cambio de estado dejan su pedido.estado_cambiado en el outbox, con el
// sobre común y la versión del pedido como secuencia.
func TestMySQLCadaCambioDejaSuMensajeEnElOutbox(t *testing.T) {
	db := conectar(t)
	ctx := context.Background()
	pedidos := adapter.PedidosMySQL{DB: db}
	outbox := adapter.OutboxMySQL{DB: db}

	// 1. Se crea (RECIBIDO) y la cocina lo acepta.
	p := crearPedido(t, db, pedidos)
	t.Cleanup(func() { db.Exec("DELETE FROM outbox WHERE JSON_UNQUOTE(JSON_EXTRACT(cuerpo,'$.agregado_id'))=?", p.ID) })
	cocina := domain.Actor{UsuarioID: "u-cocina", Rol: domain.RolCocina, CocinaID: "cerro"}
	if _, err := (application.PedidosService{Repo: pedidos}).CambiarEstado(ctx, cocina, p.ID, application.Aceptar); err != nil {
		t.Fatal(err)
	}

	// 2. Los dos mensajes están pendientes, en orden.
	pendientes, err := outbox.Pendientes(ctx, 100000)
	if err != nil {
		t.Fatal(err)
	}
	type mensaje struct {
		EventoID   string `json:"evento_id"`
		Tipo       string `json:"tipo"`
		Version    int    `json:"version"`
		AgregadoID string `json:"agregado_id"`
		Secuencia  int64  `json:"secuencia"`
		Datos      struct {
			EstadoAnterior string `json:"estado_anterior"`
			EstadoNuevo    string `json:"estado_nuevo"`
			ClienteEmail   string `json:"cliente_email"`
		} `json:"datos"`
	}
	var delPedido []mensaje
	var ids []string
	for _, m := range pendientes {
		var leido mensaje
		if err := json.Unmarshal(m.Cuerpo, &leido); err != nil {
			t.Fatal(err)
		}
		if leido.AgregadoID == p.ID {
			delPedido = append(delPedido, leido)
			ids = append(ids, m.ID)
			if m.Tipo != "pedido.estado_cambiado" || m.ID != leido.EventoID {
				t.Fatal("tipo e id del mensaje:", m.Tipo, m.ID)
			}
		}
	}
	if len(delPedido) != 2 {
		t.Fatal("un mensaje por cambio:", len(delPedido))
	}
	primero, segundo := delPedido[0], delPedido[1]
	if primero.Secuencia != 1 || primero.Datos.EstadoNuevo != "RECIBIDO" || primero.Datos.EstadoAnterior != "" || primero.Version != 1 ||
		segundo.Secuencia != 2 || segundo.Datos.EstadoAnterior != "RECIBIDO" || segundo.Datos.EstadoNuevo != "ACEPTADO" ||
		segundo.Datos.ClienteEmail != "cliente@ejemplo.com" {
		t.Fatalf("%+v %+v", primero, segundo)
	}

	// 3. Marcados como publicados, ya no están pendientes.
	for _, id := range ids {
		if err := outbox.MarcarPublicado(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	pendientes, _ = outbox.Pendientes(ctx, 100000)
	for _, m := range pendientes {
		if m.ID == ids[0] || m.ID == ids[1] {
			t.Fatal("ya publicado no tiene que figurar como pendiente")
		}
	}
}
