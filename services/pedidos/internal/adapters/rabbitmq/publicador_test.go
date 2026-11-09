package rabbitmq

import (
	"context"
	"os"
	"testing"
	"time"

	"ghostkitchen/pedidos/internal/ports"
	amqp "github.com/rabbitmq/amqp091-go"
)

// Contra un RabbitMQ real (TEST_RABBITMQ_URL): el mensaje llega a una cola atada al exchange,
// con sus propiedades. Sin la variable, se saltea.
func TestPublicarLlegaALaCola(t *testing.T) {
	url := os.Getenv("TEST_RABBITMQ_URL")
	if url == "" {
		t.Skip("requiere TEST_RABBITMQ_URL")
	}

	// 1. Una cola de prueba atada a pedido.estado_cambiado.
	conn, err := amqp.Dial(url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	canal, _ := conn.Channel()
	if err := canal.ExchangeDeclare(Exchange, "topic", true, false, false, false, nil); err != nil {
		t.Fatal(err)
	}
	cola, _ := canal.QueueDeclare("", false, true, true, false, nil)
	_ = canal.QueueBind(cola.Name, "pedido.estado_cambiado", Exchange, false, nil)

	// 2. Publicamos.
	p := &PublicadorRabbit{URL: url}
	defer p.Cerrar()
	m := ports.Mensaje{ID: "ev-1", Tipo: "pedido.estado_cambiado", Cuerpo: []byte(`{"evento_id":"ev-1"}`)}
	if err := p.Publicar(context.Background(), m); err != nil {
		t.Fatal(err)
	}

	// 3. Llegó, persistente y con el evento_id como message_id.
	time.Sleep(200 * time.Millisecond)
	recibido, ok, err := canal.Get(cola.Name, true)
	if err != nil || !ok || recibido.MessageId != "ev-1" || recibido.DeliveryMode != amqp.Persistent || string(recibido.Body) != `{"evento_id":"ev-1"}` {
		t.Fatal(err, ok, recibido.MessageId)
	}
}

// Sin RabbitMQ, Publicar devuelve error (y el mensaje queda en el outbox), sin colgarse.
func TestSinRabbitMQDevuelveError(t *testing.T) {
	p := &PublicadorRabbit{URL: "amqp://nadie:nada@127.0.0.1:1/"}
	inicio := time.Now()
	if err := p.Publicar(context.Background(), ports.Mensaje{ID: "ev-1", Tipo: "pedido.estado_cambiado", Cuerpo: []byte(`{}`)}); err == nil {
		t.Fatal("tenía que fallar")
	}
	if time.Since(inicio) > 3*time.Second {
		t.Fatal("no tiene que colgarse")
	}
}
