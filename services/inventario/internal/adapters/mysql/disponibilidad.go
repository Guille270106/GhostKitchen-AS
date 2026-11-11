package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"ghostkitchen/inventario/internal/domain"
	"github.com/google/uuid"
)

func (r *Repositorio) ListarDisponibilidad(ctx context.Context) ([]domain.Disponibilidad, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT plato_id,cocina_id,disponible,tiene_receta FROM disponibilidad_actual ORDER BY plato_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.Disponibilidad{}
	for rows.Next() {
		var item domain.Disponibilidad
		if err := rows.Scan(&item.PlatoID, &item.CocinaID, &item.Disponible, &item.TieneReceta); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// En READ COMMITTED, el siguiente escritor que obtenga el bloqueo ve tanto
// su propio cambio como los cambios confirmados por el escritor anterior.
func actualizarDisponibilidad(ctx context.Context, tx *sql.Tx, cocina string) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO disponibilidad_bloqueo (cocina_id) VALUES (?)
 ON DUPLICATE KEY UPDATE cocina_id=cocina_id`, cocina); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT a.plato_id,a.cocina_id,a.disponible,a.tiene_receta,e.secuencia
 FROM disponibilidad_actual a LEFT JOIN disponibilidad_estado e ON e.plato_id=a.plato_id
 WHERE a.cocina_id=? AND (e.plato_id IS NULL OR a.disponible<>e.disponible OR a.tiene_receta<>e.tiene_receta)
 ORDER BY a.plato_id`, cocina)
	if err != nil {
		return err
	}
	defer rows.Close()
	type cambio struct {
		domain.Disponibilidad
		secuencia *uint64
	}
	var cambios []cambio
	for rows.Next() {
		var c cambio
		if err := rows.Scan(&c.PlatoID, &c.CocinaID, &c.Disponible, &c.TieneReceta, &c.secuencia); err != nil {
			return err
		}
		cambios = append(cambios, c)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, c := range cambios {
		secuencia := uint64(1)
		if c.secuencia == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO disponibilidad_estado (plato_id,tiene_receta,disponible,secuencia)
 VALUES (?,?,?,?)`, c.PlatoID, c.TieneReceta, c.Disponible, secuencia)
		} else {
			secuencia = *c.secuencia + 1
			_, err = tx.ExecContext(ctx, `UPDATE disponibilidad_estado SET tiene_receta=?,disponible=?,secuencia=? WHERE plato_id=?`,
				c.TieneReceta, c.Disponible, secuencia, c.PlatoID)
		}
		if err != nil {
			return err
		}
		if err := guardarEvento(ctx, tx, "disponibilidad.cambiada", c.PlatoID, secuencia, time.Now().UTC(), c.Disponibilidad); err != nil {
			return err
		}
	}
	return nil
}

func guardarEvento(ctx context.Context, tx *sql.Tx, tipo, agregado string, secuencia uint64, ahora time.Time, datos any) error {
	id := uuid.NewString()
	cid := correlacionEvento(ctx, id)
	cuerpo, err := json.Marshal(map[string]any{
		"evento_id": id, "tipo": tipo, "version": 1, "ocurrido_en": ahora.UTC(),
		"correlation_id": cid, "agregado_id": agregado, "secuencia": secuencia, "datos": datos,
	})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO outbox (id,tipo,cuerpo) VALUES (?,?,?)`, id, tipo, cuerpo)
	return err
}
