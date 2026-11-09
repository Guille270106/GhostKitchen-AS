package rabbitmq

import (
	"context"
	"errors"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// ErrMensajeInvalido es un mensaje roto o de otro tipo: reintentarlo no lo arregla, así que se
// rechaza sin reencolar y RabbitMQ lo manda a la DLQ de la cola (infra/rabbitmq).
var ErrMensajeInvalido = errors.New("mensaje inválido")

// Consumidor lee una cola de RabbitMQ y le pasa cada mensaje a Manejar (docs/contracts/eventos):
//   - si Manejar sale bien, confirma (ACK) y RabbitMQ lo borra de la cola;
//   - si devuelve ErrMensajeInvalido, lo rechaza sin reencolar: va a la DLQ;
//   - si devuelve otro error (por ejemplo, MySQL no contesta), espera y lo devuelve a la cola
//     para reintentar. La cola manda a la DLQ un mensaje entregado 5 veces sin confirmar.
//
// Lee de a un mensaje por vez (prefetch 1). Si se corta la conexión, vuelve a conectarse a los 5 s.
type Consumidor struct {
	URL         string
	Cola        string
	Manejar     func(ctx context.Context, cuerpo []byte) error
	Log         *slog.Logger
	EsperaError time.Duration // cuánto esperar antes de devolver un mensaje que falló
}

// Escuchar procesa mensajes hasta que se cancele ctx.
func (c *Consumidor) Escuchar(ctx context.Context) {
	for ctx.Err() == nil {
		err := c.escucharUnaVez(ctx)
		if ctx.Err() != nil {
			return
		}
		c.Log.Warn("se cortó la conexión con RabbitMQ; se reintenta en 5 s", "cola", c.Cola, "error", err.Error())
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
		}
	}
}

// escucharUnaVez se conecta, se suscribe a la cola y procesa hasta que se corte la conexión.
func (c *Consumidor) escucharUnaVez(ctx context.Context) error {
	conn, err := amqp.DialConfig(c.URL, amqp.Config{Dial: amqp.DefaultDial(2 * time.Second)})
	if err != nil {
		return err
	}
	defer conn.Close()
	canal, err := conn.Channel()
	if err != nil {
		return err
	}
	if err := canal.Qos(1, 0, false); err != nil {
		return err
	}
	entregas, err := canal.Consume(c.Cola, "pedidos", false, false, false, false, nil)
	if err != nil {
		return err
	}
	c.Log.Info("escuchando la cola", "cola", c.Cola)
	for {
		select {
		case <-ctx.Done():
			return nil
		case m, abierto := <-entregas:
			if !abierto {
				return errors.New("RabbitMQ cerró la entrega de mensajes")
			}
			c.procesar(ctx, m)
		}
	}
}

// procesar maneja un mensaje y le contesta a RabbitMQ según cómo salió.
func (c *Consumidor) procesar(ctx context.Context, m amqp.Delivery) {
	err := c.Manejar(ctx, m.Body)
	switch {
	case err == nil:
		_ = m.Ack(false)
	case errors.Is(err, ErrMensajeInvalido):
		c.Log.Warn("mensaje inválido: va a la DLQ", "cola", c.Cola, "message_id", m.MessageId)
		_ = m.Reject(false)
	default:
		c.Log.Warn("no se pudo procesar el mensaje; vuelve a la cola", "cola", c.Cola, "message_id", m.MessageId, "error", err.Error())
		select {
		case <-time.After(c.EsperaError):
		case <-ctx.Done():
		}
		_ = m.Nack(false, true)
	}
}
