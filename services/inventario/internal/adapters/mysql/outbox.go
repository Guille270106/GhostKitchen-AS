package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"ghostkitchen/inventario/internal/domain"
	"github.com/google/uuid"
)

const eventoReservaVencida = "reserva.vencida"

func (r *Repositorio) VencerReservas(ctx context.Context) error {
	for range 100 {
		procesada, err := r.vencerSiguiente(ctx)
		if err != nil || !procesada {
			return err
		}
	}
	return nil
}

func (r *Repositorio) vencerSiguiente(ctx context.Context) (bool, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(ctx, `SELECT id FROM reserva
 WHERE estado='ACTIVA' AND vence_en<=UTC_TIMESTAMP(6)
 ORDER BY vence_en,id LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	res, ahora, err := leerReserva(ctx, tx, id, true)
	if err != nil {
		return false, err
	}
	if _, err = r.vencerReserva(ctx, tx, res, ahora); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (r *Repositorio) vencerReserva(ctx context.Context, tx *sql.Tx, res domain.Reserva, ahora time.Time) (domain.Reserva, error) {
	res, err := r.aplicarItems(ctx, tx, res, nil, "VENCIDA")
	if err != nil {
		return res, err
	}
	id := uuid.NewString()
	cid := domain.Correlacion(ctx)
	if cid == "" {
		cid = id
	}
	cuerpo, err := json.Marshal(map[string]any{
		"evento_id": id, "tipo": eventoReservaVencida, "version": 1,
		"ocurrido_en": ahora.UTC(), "correlation_id": cid,
		"agregado_id": res.CarritoID, "secuencia": 1,
		"datos": map[string]string{"reserva_id": res.ID, "carrito_id": res.CarritoID, "cocina_id": res.CocinaID},
	})
	if err != nil {
		return res, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO outbox (id,tipo,cuerpo) VALUES (?,?,?)`, id, eventoReservaVencida, cuerpo)
	return res, err
}

// Un fallo conserva la fila. Si el commit falla después del ACK, el próximo
// intento conserva el mismo evento_id para que el consumidor pueda deduplicar.
func (r *Repositorio) PublicarSiguiente(ctx context.Context, publicar func(context.Context, string, string, []byte) error) (bool, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var id, tipo string
	var cuerpo []byte
	err = tx.QueryRowContext(ctx, `SELECT id,tipo,cuerpo FROM outbox
 ORDER BY creado_en,id LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&id, &tipo, &cuerpo)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err = publicar(ctx, id, tipo, cuerpo); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM outbox WHERE id=?`, id); err != nil {
		return false, err
	}
	return true, tx.Commit()
}
