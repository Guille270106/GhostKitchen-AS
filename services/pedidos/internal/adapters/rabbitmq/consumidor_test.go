package rabbitmq

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Contra un RabbitMQ real (TEST_RABBITMQ_URL): un mensaje bueno se confirma, uno roto se rechaza
// (va a la DLQ) y uno que falla una vez vuelve a la cola y se procesa en el segundo intento.
func TestConsumidorConfirmaRechazaYReintenta(t *testing.T) {
	url := os.Getenv("TEST_RABBITMQ_URL")
	if url == "" {
		t.Skip("requiere TEST_RABBITMQ_URL")
	}

	// 1. Una cola de prueba con su DLQ, como las de infra/rabbitmq.
	conn, err := amqp.Dial(url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	canal, _ := conn.Channel()
	cola := "prueba.pedidos." + time.Now().Format("150405.000")
	_, _ = canal.QueueDeclare(cola+".dlq", false, false, false, false, nil)
	defer canal.QueueDelete(cola+".dlq", false, false, false)
	defer canal.QueueDelete(cola, false, false, false)
	_, err = canal.QueueDeclare(cola, false, false, false, false, amqp.Table{"x-dead-letter-exchange": "", "x-dead-letter-routing-key": cola + ".dlq"})
	if err != nil {
		t.Fatal(err)
	}
	for _, cuerpo := range []string{"bueno", "roto", "falla-una-vez"} {
		_ = canal.Publish("", cola, false, false, amqp.Publishing{Body: []byte(cuerpo)})
	}

	// 2. El consumidor, con un manejador que registra lo que recibe.
	var mu sync.Mutex
	recibidos := []string{}
	fallo := false
	listo := make(chan struct{})
	c := &Consumidor{URL: url, Cola: cola, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), EsperaError: 100 * time.Millisecond,
		Manejar: func(_ context.Context, cuerpo []byte) error {
			mu.Lock()
			defer mu.Unlock()
			recibidos = append(recibidos, string(cuerpo))
			switch {
			case string(cuerpo) == "roto":
				return ErrMensajeInvalido
			case string(cuerpo) == "falla-una-vez" && !fallo:
				fallo = true
				return errors.New("mysql caído")
			}
			if string(cuerpo) == "falla-una-vez" {
				close(listo)
			}
			return nil
		}}
	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()
	go c.Escuchar(ctx)

	select {
	case <-listo:
	case <-time.After(10 * time.Second):
		t.Fatal("no terminó de procesar:", recibidos)
	}
	cancelar()
	time.Sleep(300 * time.Millisecond)

	// 3. La cola quedó vacía y el roto está en la DLQ.
	principal, _ := canal.QueueDeclarePassive(cola, false, false, false, false, amqp.Table{"x-dead-letter-exchange": "", "x-dead-letter-routing-key": cola + ".dlq"})
	dlq, _ := canal.QueueDeclarePassive(cola+".dlq", false, false, false, false, nil)
	if principal.Messages != 0 || dlq.Messages != 1 {
		t.Fatal("cola", principal.Messages, "dlq", dlq.Messages, recibidos)
	}
	if len(recibidos) != 4 {
		t.Fatal("bueno, roto, falla-una-vez dos veces:", recibidos)
	}
}
