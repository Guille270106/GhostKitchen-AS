package application

import (
	"context"

	"ghostkitchen/pedidos/internal/ports"
)

// OutboxService publica en RabbitMQ los mensajes guardados en el outbox (ADR-005). main lo
// llama cada 1 s.
type OutboxService struct {
	Repo       ports.OutboxRepo
	Publicador ports.Publicador
}

// PublicarPendientes manda los mensajes en el orden en que se crearon y marca cada uno recién
// cuando RabbitMQ lo confirmó. Si uno falla, para ahí (así no se adelanta un estado posterior
// del mismo pedido) y se reintenta en la próxima vuelta. Si se cae justo después de publicar y
// antes de marcar, el mensaje se manda dos veces: por eso lleva evento_id y el consumidor
// descarta los repetidos.
func (s OutboxService) PublicarPendientes(ctx context.Context) (publicados int, err error) {
	mensajes, err := s.Repo.Pendientes(ctx, 50)
	if err != nil {
		return 0, err
	}
	for _, m := range mensajes {
		if err := s.Publicador.Publicar(ctx, m); err != nil {
			return publicados, err
		}
		if err := s.Repo.MarcarPublicado(ctx, m.ID); err != nil {
			return publicados, err
		}
		publicados++
	}
	return publicados, nil
}
