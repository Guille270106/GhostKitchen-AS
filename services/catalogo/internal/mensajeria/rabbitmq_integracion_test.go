package mensajeria

// Test de integración contra un RabbitMQ real. Se saltea sin RABBITMQ_TEST_URL.
//
// Con docker compose levantado (desde la raíz del repo, en Git Bash):
//   docker run --rm --network ghostkitchen-as_default -v "$(pwd -W)/services/catalogo:/src" -w /src \
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

	"ghostkitchen/catalogo/internal/domain"
	"ghostkitchen/catalogo/internal/repositories/memoria"
)

func TestIntegracionPublicaEnRabbitMQ(t *testing.T) {
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
	// El exchange es el de infra/rabbitmq (si ya existe con estos datos, no cambia nada). La
	// cola es temporal, solo para este test, y escucha los mismos tipos que Búsqueda.
	if err := ch.ExchangeDeclare(Exchange, "topic", true, false, false, false, nil); err != nil {
		t.Fatal(err)
	}
	q, err := ch.QueueDeclare("", false, true, true, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, clave := range []string{"cocina.*", "plato.*"} {
		if err := ch.QueueBind(q.Name, clave, Exchange, false, nil); err != nil {
			t.Fatal(err)
		}
	}
	mensajes, err := ch.Consume(q.Name, "", true, true, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	repo := memoria.Nuevo()
	c := domain.Cocina{ID: "guemes", Nombre: "Güemes", Localidad: "Nueva Córdoba", Activa: true, Version: 1}
	evCocina := domain.EventoCocina(domain.CocinaCreada, c, "req-integracion", time.Now())
	_ = repo.CrearCocina(context.Background(), c, evCocina)
	p := domain.Plato{ID: "p-x", CocinaID: "guemes", Nombre: "Mega", Categoria: "Hamburguesas", Precio: 100, Version: 1}
	evPlato := domain.EventoPlato(domain.PlatoCreado, p, "req-integracion", time.Now())
	_ = repo.CrearPlato(context.Background(), p, evPlato)

	pub := NuevoPublicador(repo, func() (Canal, error) { return Conectar(url) }, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go pub.Correr(ctx)

	for i, esperado := range []domain.Evento{evCocina, evPlato} {
		select {
		case m := <-mensajes:
			if m.RoutingKey != esperado.Tipo || m.MessageId != esperado.EventoID || m.ContentType != "application/json" || m.DeliveryMode != amqp.Persistent {
				t.Fatalf("mensaje %d: clave %q, id %q, tipo %q, modo %d", i, m.RoutingKey, m.MessageId, m.ContentType, m.DeliveryMode)
			}
			var sobre map[string]any
			if err := json.Unmarshal(m.Body, &sobre); err != nil || sobre["evento_id"] != esperado.EventoID || sobre["agregado_id"] != esperado.AgregadoID || sobre["correlation_id"] != "req-integracion" {
				t.Fatalf("sobre %d: %s", i, m.Body)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("no llegó el mensaje %d", i)
		}
	}
	// Recién después de la confirmación del broker quedan marcados.
	limite := time.Now().Add(2 * time.Second)
	for {
		if pend, _ := repo.Pendientes(context.Background(), 10); len(pend) == 0 {
			break
		}
		if time.Now().After(limite) {
			t.Fatal("los eventos confirmados tienen que quedar marcados como publicados")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
