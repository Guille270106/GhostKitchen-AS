package mysql

import (
	"context"
	"database/sql"
	"errors"
	"ghostkitchen/inventario/internal/domain"
	"ghostkitchen/inventario/internal/ports"
)

type Repositorio struct{ db *sql.DB }

var _ ports.Ingredientes = (*Repositorio)(nil)

func Nuevo(db *sql.DB) *Repositorio { return &Repositorio{db: db} }

func (r *Repositorio) Crear(ctx context.Context, i domain.Ingrediente) error {
	// La validación de cocina y la escritura comparten una sola sentencia.
	res, err := r.db.ExecContext(ctx, `INSERT INTO ingrediente (id,cocina_id,nombre,unidad,total,reservado)
 SELECT ?,id,?,?,?,0 FROM cocina_ref WHERE id=? AND activa=1`, i.ID, i.Nombre, i.Unidad, i.Total, i.CocinaID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return domain.ErrNoEncontrado
	}
	return err
}

func (r *Repositorio) Listar(ctx context.Context, cocina string) ([]domain.Ingrediente, error) {
	var existe int
	err := r.db.QueryRowContext(ctx, `SELECT 1 FROM cocina_ref WHERE id=? AND activa=1`, cocina).Scan(&existe)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNoEncontrado
	}
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,cocina_id,nombre,unidad,total,reservado,total-reservado
 FROM ingrediente WHERE cocina_id=? ORDER BY nombre,id`, cocina)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.Ingrediente{}
	for rows.Next() {
		var i domain.Ingrediente
		if err := rows.Scan(&i.ID, &i.CocinaID, &i.Nombre, &i.Unidad, &i.Total, &i.Reservado, &i.Disponible); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

const consultaIngrediente = `SELECT i.id,i.cocina_id,i.nombre,i.unidad,i.total,i.reservado,i.total-i.reservado
 FROM ingrediente i JOIN cocina_ref c ON c.id=i.cocina_id AND c.activa=1
 WHERE i.id=? AND i.cocina_id=?`

func (r *Repositorio) Obtener(ctx context.Context, cocina, id string) (domain.Ingrediente, error) {
	var i domain.Ingrediente
	err := r.db.QueryRowContext(ctx, consultaIngrediente, id, cocina).Scan(&i.ID, &i.CocinaID, &i.Nombre, &i.Unidad, &i.Total, &i.Reservado, &i.Disponible)
	if errors.Is(err, sql.ErrNoRows) {
		err = domain.ErrNoEncontrado
	}
	return i, err
}

func (r *Repositorio) Ready(ctx context.Context) error {
	// Comprueba conectividad y que la migración del servicio está aplicada.
	var n int
	return r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ingrediente i
 JOIN receta r ON r.cocina_id=i.cocina_id JOIN receta_item ri ON ri.plato_id=r.plato_id
 JOIN plato_ref p ON p.id=r.plato_id JOIN reserva rs ON rs.cocina_id=i.cocina_id
 JOIN reserva_plato rp ON rp.reserva_id=rs.id JOIN reserva_item rsi ON rsi.reserva_id=rs.id
 JOIN idempotencia idem ON 1=0 JOIN movimiento_stock m ON 1=0 WHERE 1=0`).Scan(&n)
}
