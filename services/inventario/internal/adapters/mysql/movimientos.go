package mysql

import (
	"context"
	"database/sql"
	"errors"
	"math"

	"ghostkitchen/inventario/internal/domain"
)

func (r *Repositorio) MoverStock(ctx context.Context, cocina, id, usuario string, delta int64) (domain.Ingrediente, error) {
	var i domain.Ingrediente
	if delta == 0 {
		return i, domain.ErrDatosInvalidos
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return i, err
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, consultaIngrediente+" FOR UPDATE OF i FOR SHARE OF c", id, cocina).
		Scan(&i.ID, &i.CocinaID, &i.Nombre, &i.Unidad, &i.Total, &i.Reservado, &i.Disponible)
	if errors.Is(err, sql.ErrNoRows) {
		return i, domain.ErrNoEncontrado
	}
	if err != nil {
		return i, err
	}
	if delta < -i.Disponible {
		return i, domain.ErrSinStock
	}
	if delta > math.MaxInt64-i.Total {
		return i, domain.ErrStockExcedido
	}
	i.Total += delta
	i.Disponible += delta
	if _, err = tx.ExecContext(ctx, `UPDATE ingrediente SET total=? WHERE id=? AND cocina_id=?`, i.Total, id, cocina); err != nil {
		return i, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO movimiento_stock (ingrediente_id,usuario_id,delta) VALUES (?,?,?)`, id, usuario, delta); err != nil {
		return i, err
	}
	if err = actualizarDisponibilidad(ctx, tx, cocina); err != nil {
		return i, err
	}
	return i, tx.Commit()
}
