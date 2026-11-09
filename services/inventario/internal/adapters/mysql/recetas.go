package mysql

import (
	"context"
	"database/sql"
	"errors"

	"ghostkitchen/inventario/internal/domain"
	"ghostkitchen/inventario/internal/ports"
)

var _ ports.Recetas = (*Repositorio)(nil)

func (r *Repositorio) GuardarReceta(ctx context.Context, receta domain.Receta) (domain.Receta, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Receta{}, err
	}
	defer tx.Rollback()
	var existe int
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM plato_ref p JOIN cocina_ref c ON c.id=p.cocina_id
 WHERE p.id=? AND p.cocina_id=? AND p.activo=1 AND c.activa=1 FOR UPDATE`, receta.PlatoID, receta.CocinaID).Scan(&existe)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Receta{}, domain.ErrNoEncontrado
	}
	if err != nil {
		return domain.Receta{}, err
	}
	// El INSERT/UPDATE bloquea la cabecera hasta commit; no quedan recetas a medio reemplazar.
	_, err = tx.ExecContext(ctx, `INSERT INTO receta (plato_id,cocina_id) VALUES (?,?)
 ON DUPLICATE KEY UPDATE version=version+1`, receta.PlatoID, receta.CocinaID)
	if err != nil {
		return domain.Receta{}, err
	}
	// El dominio ordena IDs para mantener el orden de bloqueo de ingredientes.
	for _, item := range receta.Items {
		err = tx.QueryRowContext(ctx, `SELECT 1 FROM ingrediente WHERE id=? AND cocina_id=? FOR SHARE`, item.IngredienteID, receta.CocinaID).Scan(&existe)
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Receta{}, domain.ErrNoEncontrado
		}
		if err != nil {
			return domain.Receta{}, err
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM receta_item WHERE plato_id=?`, receta.PlatoID); err != nil {
		return domain.Receta{}, err
	}
	for _, item := range receta.Items {
		if _, err = tx.ExecContext(ctx, `INSERT INTO receta_item (plato_id,ingrediente_id,cocina_id,cantidad) VALUES (?,?,?,?)`, receta.PlatoID, item.IngredienteID, receta.CocinaID, item.Cantidad); err != nil {
			return domain.Receta{}, err
		}
	}
	if err = tx.QueryRowContext(ctx, `SELECT version FROM receta WHERE plato_id=?`, receta.PlatoID).Scan(&receta.Version); err != nil {
		return domain.Receta{}, err
	}
	if err = tx.Commit(); err != nil {
		return domain.Receta{}, err
	}
	return receta, nil
}

const seleccionarRecetas = `SELECT r.plato_id,r.cocina_id,r.version,i.ingrediente_id,i.cantidad
 FROM receta r JOIN receta_item i ON i.plato_id=r.plato_id
 JOIN plato_ref p ON p.id=r.plato_id AND p.activo=1
 JOIN cocina_ref c ON c.id=r.cocina_id AND c.activa=1
 WHERE r.cocina_id=?`

func leerRecetas(rows *sql.Rows) ([]domain.Receta, error) {
	defer rows.Close()
	recetas := []domain.Receta{}
	for rows.Next() {
		var receta domain.Receta
		var item domain.RecetaItem
		if err := rows.Scan(&receta.PlatoID, &receta.CocinaID, &receta.Version, &item.IngredienteID, &item.Cantidad); err != nil {
			return nil, err
		}
		if len(recetas) == 0 || recetas[len(recetas)-1].PlatoID != receta.PlatoID {
			recetas = append(recetas, receta)
		}
		ultimo := &recetas[len(recetas)-1]
		ultimo.Items = append(ultimo.Items, item)
	}
	return recetas, rows.Err()
}

func (r *Repositorio) ObtenerReceta(ctx context.Context, cocina, plato string) (domain.Receta, error) {
	rows, err := r.db.QueryContext(ctx, seleccionarRecetas+` AND r.plato_id=? ORDER BY i.ingrediente_id`, cocina, plato)
	if err != nil {
		return domain.Receta{}, err
	}
	recetas, err := leerRecetas(rows)
	if err != nil {
		return domain.Receta{}, err
	}
	if len(recetas) == 0 {
		return domain.Receta{}, domain.ErrNoEncontrado
	}
	return recetas[0], nil
}

func (r *Repositorio) ListarRecetas(ctx context.Context, cocina string) ([]domain.Receta, error) {
	var existe int
	err := r.db.QueryRowContext(ctx, `SELECT 1 FROM cocina_ref WHERE id=? AND activa=1`, cocina).Scan(&existe)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNoEncontrado
	}
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, seleccionarRecetas+` ORDER BY r.plato_id,i.ingrediente_id`, cocina)
	if err != nil {
		return nil, err
	}
	return leerRecetas(rows)
}
