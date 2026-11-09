// Package rabbitmq es el adaptador de salida hacia RabbitMQ. Implementa ports.Publicador.
package rabbitmq

import (
	"context"
	"errors"
	"sync"
	"time"

	"ghostkitchen/pedidos/internal/ports"
	amqp "github.com/rabbitmq/amqp091-go"
)

// Exchange es donde se publican todos los eventos (infra/rabbitmq/definiciones.json).
const Exchange = "ghostkitchen.events"

// PublicadorRabbit publica en RabbitMQ con confirmación del broker (publisher confirms).
// Si RabbitMQ no está, Publicar devuelve error y en la próxima llamada vuelve a conectarse:
// Pedidos sigue funcionando y los mensajes esperan en el outbox.
type PublicadorRabbit struct {
	URL string // amqp://usuario:clave@rabbitmq:5672/

	mu    sync.Mutex
	conn  *amqp.Connection
	canal *amqp.Channel
}

// Publicar manda el mensaje y espera que RabbitMQ confirme que lo guardó (hasta 2 s).
func (p *PublicadorRabbit) Publicar(ctx context.Context, m ports.Mensaje) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 1. Nos conectamos si hace falta (la primera vez o después de un error).
	if err := p.conectar(); err != nil {
		return err
	}

	// 2. Publicamos: persistente, JSON, y con el evento_id como message_id.
	conPlazo, cancelar := context.WithTimeout(ctx, 2*time.Second)
	defer cancelar()
	confirmacion, err := p.canal.PublishWithDeferredConfirmWithContext(conPlazo, Exchange, m.Tipo, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    m.ID,
		Timestamp:    time.Now().UTC(),
		Body:         m.Cuerpo,
	})
	if err != nil {
		p.cerrar()
		return err
	}

	// 3. Esperamos la confirmación del broker. Sin ella, el mensaje queda pendiente en el outbox.
	confirmado, err := confirmacion.WaitContext(conPlazo)
	if err != nil {
		p.cerrar()
		return err
	}
	if !confirmado {
		return errors.New("RabbitMQ no aceptó el mensaje")
	}
	return nil
}

// conectar abre la conexión y un canal en modo confirmación, si no están abiertos.
func (p *PublicadorRabbit) conectar() error {
	if p.canal != nil && !p.canal.IsClosed() {
		return nil
	}
	p.cerrar()
	conn, err := amqp.DialConfig(p.URL, amqp.Config{Dial: amqp.DefaultDial(2 * time.Second)})
	if err != nil {
		return err
	}
	canal, err := conn.Channel()
	if err != nil {
		conn.Close()
		return err
	}
	if err := canal.Confirm(false); err != nil {
		conn.Close()
		return err
	}
	p.conn, p.canal = conn, canal
	return nil
}

// cerrar descarta la conexión; la próxima publicación abre una nueva.
func (p *PublicadorRabbit) cerrar() {
	if p.conn != nil {
		p.conn.Close()
	}
	p.conn, p.canal = nil, nil
}

// Cerrar cierra la conexión al apagar.
func (p *PublicadorRabbit) Cerrar() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cerrar()
}
