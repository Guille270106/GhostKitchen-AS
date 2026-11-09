// Package mensajeria publica en RabbitMQ los eventos que Catálogo guardó en su outbox (G13).
//
// El publicador lee los eventos pendientes de la base, los manda al exchange
// ghostkitchen.events y los marca como enviados recién cuando RabbitMQ confirma que los
// recibió (publisher confirms). Si RabbitMQ está caído, los eventos esperan en el outbox y
// salen cuando vuelve: un cambio guardado nunca pierde su mensaje (ADR-005).
package mensajeria

import (
	"context"
	"errors"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"ghostkitchen/catalogo/internal/domain"
)

// Exchange donde publica Catálogo (infra/rabbitmq). La clave de ruteo es el tipo del evento.
const Exchange = "ghostkitchen.events"

// RabbitMQ es una conexión con un canal en modo confirmación.
type RabbitMQ struct {
	conn *amqp.Connection
	ch   *amqp.Channel
}

// Conectar abre la conexión y un canal con publisher confirms: cada mensaje publicado espera
// el "recibido" (ack) del broker. La URL lleva usuario y contraseña: nunca se registra.
func Conectar(url string) (*RabbitMQ, error) {
	conn, err := amqp.DialConfig(url, amqp.Config{Dial: amqp.DefaultDial(5 * time.Second), Properties: amqp.Table{"connection_name": "catalogo-outbox"}})
	if err != nil {
		return nil, errors.New("no se pudo conectar con RabbitMQ")
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, errors.New("no se pudo abrir un canal en RabbitMQ")
	}
	if err := ch.Confirm(false); err != nil {
		_ = conn.Close()
		return nil, errors.New("RabbitMQ no aceptó el modo confirmación")
	}
	return &RabbitMQ{conn: conn, ch: ch}, nil
}

// Publicar manda un evento y espera la confirmación del broker (hasta 5 s). Solo devuelve nil
// si RabbitMQ lo confirmó: recién ahí se puede marcar como enviado.
//
// Propiedades AMQP del contrato (docs/contracts/eventos): JSON, persistente (sobrevive a un
// reinicio de RabbitMQ) y message_id igual al evento_id, que el consumidor usa para no
// procesar dos veces el mismo mensaje.
func (r *RabbitMQ) Publicar(ctx context.Context, p domain.Pendiente) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conf, err := r.ch.PublishWithDeferredConfirmWithContext(ctx, Exchange, p.Tipo, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    p.EventoID,
		Type:         p.Tipo,
		Timestamp:    time.Now().UTC(),
		AppId:        "catalogo",
		Body:         p.Sobre,
	})
	if err != nil {
		return fmt.Errorf("publicar %s: %w", p.Tipo, err)
	}
	ok, err := conf.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("esperar la confirmación de %s: %w", p.Tipo, err)
	}
	if !ok {
		return fmt.Errorf("RabbitMQ rechazó %s (nack)", p.Tipo)
	}
	return nil
}

// Cerrar cierra el canal y la conexión.
func (r *RabbitMQ) Cerrar() {
	_ = r.ch.Close()
	_ = r.conn.Close()
}
