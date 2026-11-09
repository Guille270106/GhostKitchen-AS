package rabbitmq

import (
	"context"
	"errors"
	"time"

	"ghostkitchen/inventario/internal/domain"
	amqp "github.com/rabbitmq/amqp091-go"
)

func ConsumirReferencias(ctx context.Context, host, usuario, clave string, aplicar func(context.Context, domain.Referencia) error) error {
	conn, cerrar, err := conectar(ctx, host, usuario, clave)
	if err != nil {
		return err
	}
	defer cerrar()
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	if err = ch.Qos(1, 0, false); err != nil {
		return err
	}
	entregas, err := ch.Consume("inventario.referencias", "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case entrega, ok := <-entregas:
			if !ok {
				return errors.New("se cerró el consumidor de referencias")
			}
			if err := procesarReferencia(ctx, entrega, aplicar); err != nil {
				return err
			}
		}
	}
}

func procesarReferencia(ctx context.Context, entrega amqp.Delivery, aplicar func(context.Context, domain.Referencia) error) error {
	ref, err := domain.LeerReferencia(entrega.Body, entrega.RoutingKey)
	if err == nil {
		intento, cancelar := context.WithTimeout(ctx, 3*time.Second)
		err = aplicar(intento, ref)
		cancelar()
	}
	if err == nil {
		return entrega.Ack(false)
	}
	if errors.Is(err, domain.ErrEventoInvalido) {
		return entrega.Reject(false)
	}
	reintentos, _ := entrega.Headers["x-delivery-count"].(int64)
	espera := 5 * time.Second
	switch {
	case reintentos == 1:
		espera = 30 * time.Second
	case reintentos >= 2:
		espera = 2 * time.Minute
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(espera):
		return entrega.Nack(false, true)
	}
}
