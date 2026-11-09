package rabbitmq

import (
	"context"
	"errors"
	"net"
	"net/url"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func NuevoPublicador(host, usuario, clave string) func(context.Context, string, string, []byte) error {
	direccion := url.URL{Scheme: "amqp", Host: net.JoinHostPort(host, "5672"), User: url.UserPassword(usuario, clave), Path: "/"}
	return func(ctx context.Context, id, tipo string, cuerpo []byte) error {
		var detener func() bool
		conn, err := amqp.DialConfig(direccion.String(), amqp.Config{
			Dial: func(network, addr string) (net.Conn, error) {
				dialer := net.Dialer{Timeout: 2 * time.Second}
				socket, err := dialer.DialContext(ctx, network, addr)
				if err != nil {
					return nil, err
				}
				detener = context.AfterFunc(ctx, func() { socket.Close() })
				return socket, nil
			},
		})
		if detener != nil {
			defer detener()
		}
		if err != nil {
			return errors.New("RabbitMQ no disponible")
		}
		defer conn.Close()
		ch, err := conn.Channel()
		if err != nil {
			return err
		}
		if err = ch.Confirm(false); err != nil {
			return err
		}
		devoluciones := ch.NotifyReturn(make(chan amqp.Return, 1))
		confirmaciones := ch.NotifyPublish(make(chan amqp.Confirmation, 1))
		if err = ch.PublishWithContext(ctx, "ghostkitchen.events", tipo, true, false, amqp.Publishing{
			ContentType: "application/json", DeliveryMode: amqp.Persistent, MessageId: id, Body: cuerpo,
		}); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-devoluciones:
			return errors.New("evento sin destino en RabbitMQ")
		case ack := <-confirmaciones:
			if !ack.Ack {
				return errors.New("RabbitMQ no confirmó el evento")
			}
			// Para un envío mandatory sin ruta, RabbitMQ devuelve el mensaje antes del ACK.
			select {
			case <-devoluciones:
				return errors.New("evento sin destino en RabbitMQ")
			default:
				return nil
			}
		}
	}
}
