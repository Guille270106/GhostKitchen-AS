package mensajeria

// Test de integración contra un RabbitMQ real. Se saltea sin RABBITMQ_TEST_URL.
//
// Con docker compose levantado (desde la raíz del repo, en Git Bash):
//   docker run --rm --network ghostkitchen-as_default -v "$(pwd -W)/services/busqueda:/src" -w /src \
//     -e RABBITMQ_TEST_URL="amqp://<RABBITMQ_USER>:<RABBITMQ_PASSWORD>@rabbitmq:5672/" golang:1.24-alpine \
//     go test ./internal/mensajeria/ -run Integracion -v

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"ghostkitchen/busqueda/internal/indexer"
	"ghostkitchen/busqueda/internal/repositories/memoria"
)

func TestIntegracionConsumeLaCola(t *testing.T) {
	url := os.Getenv("RABBITMQ_TEST_URL")
	if url == "" {
		t.Skip("sin RABBITMQ_TEST_URL: se saltea el test contra RabbitMQ real")
	}
	conn, err := amqp.Dial(url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	// Exchange real (si ya existe igual, no cambia nada) y una cola temporal armada como
	// busqueda.indexador: quorum, 5 entregas como máximo y DLQ.
	cola, dlq := "test.busqueda.indexador", "test.busqueda.indexador.dlq"
	must(t, ch.ExchangeDeclare("ghostkitchen.events", "topic", true, false, false, false, nil))
	_, _ = ch.QueueDelete(cola, false, false, false)
	_, _ = ch.QueueDelete(dlq, false, false, false)
	must(t, ch.ExchangeDeclare("test.dlx", "direct", false, false, false, false, nil))
	_, err = ch.QueueDeclare(dlq, true, false, false, false, amqp.Table{"x-queue-type": "quorum"})
	must(t, err)
	must(t, ch.QueueBind(dlq, dlq, "test.dlx", false, nil))
	_, err = ch.QueueDeclare(cola, true, false, false, false, amqp.Table{"x-queue-type": "quorum",
		"x-delivery-limit": 5, "x-dead-letter-exchange": "test.dlx", "x-dead-letter-routing-key": dlq})
	must(t, err)
	t.Cleanup(func() {
		_, _ = ch.QueueDelete(cola, false, false, false)
		_, _ = ch.QueueDelete(dlq, false, false, false)
		_ = ch.ExchangeDelete("test.dlx", false, false)
	})
	for _, k := range []string{"cocina.*", "plato.*", "disponibilidad.cambiada"} {
		must(t, ch.QueueBind(cola, k, "ghostkitchen.events", false, nil))
	}

	publicar := func(tipo, agregado string, secuencia int64, datos any) {
		t.Helper()
		d, _ := json.Marshal(datos)
		b, _ := json.Marshal(map[string]any{"evento_id": "0b8e2f8e-1111-4c3a-9a51-2f3c1d2e4f5" + string(rune('0'+secuencia%10)),
			"tipo": tipo, "version": 1, "ocurrido_en": time.Now().UTC(), "correlation_id": "test",
			"agregado_id": agregado, "secuencia": secuencia, "datos": json.RawMessage(d)})
		must(t, ch.PublishWithContext(context.Background(), "ghostkitchen.events", tipo, false, false,
			amqp.Publishing{ContentType: "application/json", DeliveryMode: amqp.Persistent, Body: b}))
	}
	// Llega el plato antes que su cocina, un mensaje roto y después la cocina y el stock.
	publicar("plato.creado", "p1", 1, map[string]any{"id": "p1", "cocinaId": "nc", "restauranteId": "r1",
		"nombre": "Doble", "categoria": "Hamburguesas", "precio": 9000, "activo": true, "version": 1})
	must(t, ch.PublishWithContext(context.Background(), "ghostkitchen.events", "plato.creado", false, false,
		amqp.Publishing{Body: []byte(`{"roto":true}`)}))
	publicar("cocina.creada", "nc", 1, map[string]any{"id": "nc", "nombre": "Cocina NC", "activa": true, "version": 1,
		"ubicacion":    map[string]float64{"lat": -31.4, "lng": -64.2},
		"restaurantes": []map[string]any{{"id": "r1", "nombre": "Mostaza", "activo": true}}})
	publicar("disponibilidad.cambiada", "p1", 3, map[string]any{"plato_id": "p1", "cocina_id": "nc", "disponible": true, "tiene_receta": true})

	repo := memoria.Nuevo()
	cons := NuevoConsumidor(url, indexer.NuevoAplicador(repo), slog.New(slog.NewTextHandler(io.Discard, nil)))
	cons.Cola = cola
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go cons.Correr(ctx)

	limite := time.Now().Add(10 * time.Second)
	for {
		p, ok, _ := repo.ObtenerPlato(context.Background(), "p1")
		if ok && p.Disponible && p.Restaurante.Nombre == "Mostaza" {
			break
		}
		if time.Now().After(limite) {
			t.Fatalf("el plato no quedó al día: %v %+v", ok, p)
		}
		time.Sleep(100 * time.Millisecond)
	}
	// El mensaje roto fue a la DLQ y la cola quedó vacía.
	limite = time.Now().Add(5 * time.Second)
	for {
		q, err := ch.QueueDeclarePassive(dlq, true, false, false, false, amqp.Table{"x-queue-type": "quorum"})
		must(t, err)
		if q.Messages == 1 {
			break
		}
		if time.Now().After(limite) {
			t.Fatalf("la DLQ tiene %d mensajes", q.Messages)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
