package mysql

import (
	"context"
	"database/sql"
	"errors"

	"ghostkitchen/inventario/internal/domain"
	driver "github.com/go-sql-driver/mysql"
)

func (r *Repositorio) AplicarReferencia(ctx context.Context, ref domain.Referencia) error {
	ctx = domain.ConCorrelacion(ctx, ref.Correlacion)
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `INSERT INTO mensaje_procesado (evento_id) VALUES (?)`, ref.EventoID)
	var duplicado *driver.MySQLError
	if errors.As(err, &duplicado) && duplicado.Number == 1062 {
		return nil
	}
	if err != nil {
		return err
	}

	if ref.EsCocina {
		_, err = tx.ExecContext(ctx, `INSERT INTO cocina_ref (id,activa,version) VALUES (?,?,?)
 ON DUPLICATE KEY UPDATE activa=IF(version<?,?,activa),version=GREATEST(version,?)`,
			ref.ID, ref.Activa, ref.Version, ref.Version, ref.Activa, ref.Version)
	} else {
		err = guardarPlatoRef(ctx, tx, ref)
	}
	if err != nil {
		return err
	}
	if err = actualizarDisponibilidad(ctx, tx, ref.CocinaID); err != nil {
		return err
	}
	return tx.Commit()
}

func guardarPlatoRef(ctx context.Context, tx *sql.Tx, ref domain.Referencia) error {
	// Si el plato llega primero, la cocina queda inactiva hasta recibir su evento.
	if _, err := tx.ExecContext(ctx, `INSERT INTO cocina_ref (id,activa) VALUES (?,0)
 ON DUPLICATE KEY UPDATE id=id`, ref.CocinaID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO plato_ref (id,cocina_id,activo,version) VALUES (?,?,?,?)
 ON DUPLICATE KEY UPDATE activo=IF(cocina_id=? AND version<?,?,activo),
 version=IF(cocina_id=?,GREATEST(version,?),version)`,
		ref.ID, ref.CocinaID, ref.Activa, ref.Version, ref.CocinaID, ref.Version, ref.Activa, ref.CocinaID, ref.Version); err != nil {
		return err
	}
	var cocina string
	var version uint64
	err := tx.QueryRowContext(ctx, `SELECT cocina_id,version FROM plato_ref WHERE id=?`, ref.ID).Scan(&cocina, &version)
	if err == nil && cocina != ref.CocinaID && version < ref.Version {
		return domain.ErrEventoInvalido
	}
	return err
}
