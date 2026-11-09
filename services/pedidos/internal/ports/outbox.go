package ports

import "context"

// Mensaje es un mensaje del outbox listo para publicar en RabbitMQ.
type Mensaje struct {
	ID     string // evento_id: RabbitMQ lo lleva como message_id
	Tipo   string // por ejemplo pedido.estado_cambiado; es también la clave de ruteo
	Cuerpo []byte // el sobre completo, en JSON
}

// OutboxRepo lee los mensajes que todavía no se publicaron. Los escribe PedidosMySQL, en la
// misma transacción que el cambio del pedido.
type OutboxRepo interface {
	// Pendientes devuelve hasta "limite" mensajes sin publicar, en el orden en que se crearon.
	Pendientes(ctx context.Context, limite int) ([]Mensaje, error)
	// MarcarPublicado anota que RabbitMQ confirmó el mensaje.
	MarcarPublicado(ctx context.Context, id string) error
}

// Publicador manda un mensaje a RabbitMQ. Vuelve sin error solo cuando RabbitMQ confirmó que
// lo guardó (publisher confirms): si devuelve error, el mensaje se vuelve a mandar después.
type Publicador interface {
	Publicar(ctx context.Context, m Mensaje) error
}
