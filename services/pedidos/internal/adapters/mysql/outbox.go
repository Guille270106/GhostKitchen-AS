package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"ghostkitchen/pedidos/internal/adapters/correlacion"
	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/ports"
	"github.com/google/uuid"
)

// OutboxMySQL lee y marca los mensajes del outbox.
type OutboxMySQL struct {
	DB *sql.DB
}

// Pendientes devuelve los mensajes sin publicar, del más viejo al más nuevo.
func (repo OutboxMySQL) Pendientes(ctx context.Context, limite int) ([]ports.Mensaje, error) {
	filas, err := repo.DB.QueryContext(ctx, `SELECT id,tipo,cuerpo FROM outbox
 WHERE publicado_en IS NULL ORDER BY creado_en, id LIMIT ?`, limite)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	lista := []ports.Mensaje{}
	for filas.Next() {
		var m ports.Mensaje
		if err := filas.Scan(&m.ID, &m.Tipo, &m.Cuerpo); err != nil {
			return nil, err
		}
		lista = append(lista, m)
	}
	return lista, filas.Err()
}

// MarcarPublicado anota que RabbitMQ confirmó el mensaje.
func (repo OutboxMySQL) MarcarPublicado(ctx context.Context, id string) error {
	_, err := repo.DB.ExecContext(ctx, `UPDATE outbox SET publicado_en=? WHERE id=?`, time.Now().UTC(), id)
	return err
}

// sobre es la forma común de todos los mensajes (docs/contracts/eventos/sobre.schema.json).
type sobre struct {
	EventoID      string    `json:"evento_id"`
	Tipo          string    `json:"tipo"`
	Version       int       `json:"version"`
	OcurridoEn    time.Time `json:"ocurrido_en"`
	CorrelationID string    `json:"correlation_id"`
	AgregadoID    string    `json:"agregado_id"`
	Secuencia     int64     `json:"secuencia"`
	Datos         any       `json:"datos"`
}

// estadoCambiado son los datos de pedido.estado_cambiado (docs/contracts/eventos/README.md).
type estadoCambiado struct {
	EstadoAnterior domain.Estado `json:"estado_anterior,omitempty"` // no va en el primero (RECIBIDO)
	EstadoNuevo    domain.Estado `json:"estado_nuevo"`
	ClienteEmail   string        `json:"cliente_email"`
}

// anotarEstadoCambiado guarda el mensaje pedido.estado_cambiado en el outbox, dentro de la
// transacción del cambio: o se guardan los dos o ninguno. La secuencia es la versión del pedido,
// así el consumidor puede ignorar un mensaje viejo que llegó tarde.
func anotarEstadoCambiado(ctx context.Context, tx *sql.Tx, p domain.Pedido, c domain.CambioEstado) error {
	m := sobre{
		EventoID:      uuid.NewString(),
		Tipo:          "pedido.estado_cambiado",
		Version:       1,
		OcurridoEn:    c.OcurridoEn.UTC(),
		CorrelationID: correlacion.Desde(ctx),
		AgregadoID:    p.ID,
		Secuencia:     c.Version,
		Datos:         estadoCambiado{EstadoAnterior: c.EstadoAnterior, EstadoNuevo: c.EstadoNuevo, ClienteEmail: p.ClienteEmail},
	}
	cuerpo, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO outbox (id,tipo,cuerpo,creado_en) VALUES (?,?,?,?)`,
		m.EventoID, m.Tipo, cuerpo, c.OcurridoEn.UTC())
	return err
}
