package notificaciones

import (
	"encoding/json"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrEvento = errors.New("evento de pedido inválido")

type Pendiente struct {
	EventoID     string    `bson:"_id"`
	PedidoID     string    `bson:"pedido_id"`
	Destinatario string    `bson:"destinatario"`
	EstadoPedido string    `bson:"estado_pedido"`
	Estado       string    `bson:"estado"`
	Intentos     int       `bson:"intentos"`
	CreadoEn     time.Time `bson:"creado_en"`
	// CorrelationID viene en el sobre del evento: sirve para unir el log del mail con los
	// del pedido que lo causó.
	CorrelationID string `bson:"correlation_id,omitempty"`
}

func LeerEvento(cuerpo []byte, ruta string) (Pendiente, error) {
	var evento struct {
		ID          string    `json:"evento_id"`
		Tipo        string    `json:"tipo"`
		Version     int       `json:"version"`
		Pedido      string    `json:"agregado_id"`
		Secuencia   uint64    `json:"secuencia"`
		Fecha       time.Time `json:"ocurrido_en"`
		Correlacion string    `json:"correlation_id"`
		Datos       struct {
			Estado string `json:"estado_nuevo"`
			Email  string `json:"cliente_email"`
		} `json:"datos"`
	}
	if err := json.Unmarshal(cuerpo, &evento); err != nil || evento.Tipo != "pedido.estado_cambiado" ||
		ruta != evento.Tipo || evento.Version != 1 || strings.TrimSpace(evento.Pedido) == "" ||
		evento.Secuencia == 0 || evento.Fecha.IsZero() {
		return Pendiente{}, ErrEvento
	}
	id, err := uuid.Parse(evento.ID)
	if err != nil || len(evento.ID) != 36 {
		return Pendiente{}, ErrEvento
	}
	email, err := mail.ParseAddress(evento.Datos.Email)
	if err != nil || email.Address != evento.Datos.Email {
		return Pendiente{}, ErrEvento
	}
	switch evento.Datos.Estado {
	case "RECIBIDO", "ACEPTADO", "EN_PREPARACION", "LISTO", "EN_CAMINO", "ENTREGADO", "CANCELADO":
	default:
		return Pendiente{}, ErrEvento
	}
	return Pendiente{EventoID: id.String(), PedidoID: evento.Pedido, Destinatario: email.Address,
		EstadoPedido: evento.Datos.Estado, Estado: "PENDIENTE", CreadoEn: time.Now().UTC(),
		CorrelationID: evento.Correlacion}, nil
}
