// Package mysql implementa ports.Pedidos sobre MySQL (pedidos_db).
package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/ports"
)

// Repositorio guarda pedidos en MySQL.
type Repositorio struct{ db *sql.DB }

var _ ports.Pedidos = (*Repositorio)(nil)

// Nuevo crea el repositorio.
func Nuevo(db *sql.DB) *Repositorio { return &Repositorio{db: db} }

// Crear guarda pedido, items e historial inicial en una sola transacción: o queda todo o nada.
func (r *Repositorio) Crear(ctx context.Context, p domain.Pedido) error {
	return r.enTransaccion(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO pedido
 (id,cliente_id,cliente_email,cocina_id,estado,total,moneda,version,creado_en,actualizado_en)
 VALUES (?,?,?,?,?,?,?,?,?,?)`,
			p.ID, p.ClienteID, p.ClienteEmail, p.CocinaID, p.Estado, p.Total, p.Moneda, p.Version, p.CreadoEn, p.ActualizadoEn); err != nil {
			return err
		}
		for _, it := range p.Items {
			if _, err := tx.ExecContext(ctx, `INSERT INTO pedido_item
 (pedido_id,plato_id,nombre,restaurante_id,restaurante_nombre,precio_unitario,cantidad) VALUES (?,?,?,?,?,?,?)`,
				p.ID, it.PlatoID, it.Nombre, it.RestauranteID, it.RestauranteNombre, it.PrecioUnitario, it.Cantidad); err != nil {
				return err
			}
		}
		for _, c := range p.Historial {
			if err := insertarHistorial(ctx, tx, p.ID, c); err != nil {
				return err
			}
		}
		return nil
	})
}

// Obtener lee el pedido con sus items y su historial ordenado por versión.
func (r *Repositorio) Obtener(ctx context.Context, id string) (domain.Pedido, error) {
	var p domain.Pedido
	err := r.db.QueryRowContext(ctx, `SELECT id,cliente_id,cliente_email,cocina_id,estado,total,moneda,version,creado_en,actualizado_en
 FROM pedido WHERE id=?`, id).Scan(&p.ID, &p.ClienteID, &p.ClienteEmail, &p.CocinaID, &p.Estado, &p.Total, &p.Moneda, &p.Version, &p.CreadoEn, &p.ActualizadoEn)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Pedido{}, domain.ErrNoEncontrado
	}
	if err != nil {
		return domain.Pedido{}, err
	}

	rows, err := r.db.QueryContext(ctx, `SELECT plato_id,nombre,restaurante_id,restaurante_nombre,precio_unitario,cantidad
 FROM pedido_item WHERE pedido_id=? ORDER BY plato_id`, id)
	if err != nil {
		return domain.Pedido{}, err
	}
	defer rows.Close()
	p.Items = []domain.Item{}
	for rows.Next() {
		var it domain.Item
		if err := rows.Scan(&it.PlatoID, &it.Nombre, &it.RestauranteID, &it.RestauranteNombre, &it.PrecioUnitario, &it.Cantidad); err != nil {
			return domain.Pedido{}, err
		}
		p.Items = append(p.Items, it)
	}
	if err := rows.Err(); err != nil {
		return domain.Pedido{}, err
	}

	hist, err := r.db.QueryContext(ctx, `SELECT estado_anterior,estado_nuevo,actor_id,actor_rol,version,ocurrido_en
 FROM pedido_historial WHERE pedido_id=? ORDER BY version`, id)
	if err != nil {
		return domain.Pedido{}, err
	}
	defer hist.Close()
	for hist.Next() {
		var c domain.CambioEstado
		var anterior sql.NullString
		if err := hist.Scan(&anterior, &c.EstadoNuevo, &c.ActorID, &c.ActorRol, &c.Version, &c.OcurridoEn); err != nil {
			return domain.Pedido{}, err
		}
		c.EstadoAnterior = domain.Estado(anterior.String)
		p.Historial = append(p.Historial, c)
	}
	return p, hist.Err()
}

// GuardarCambio escribe el nuevo estado solo si el pedido sigue en la versión anterior.
// La condición `version = ?` va en el mismo UPDATE: la base serializa los dos UPDATE sobre la
// misma fila, así que si dos cambios llegan a la vez, el segundo no encuentra la versión y no
// escribe nada. El historial se guarda en la misma transacción.
func (r *Repositorio) GuardarCambio(ctx context.Context, p domain.Pedido, c domain.CambioEstado) error {
	return r.enTransaccion(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE pedido SET estado=?, version=?, actualizado_en=?
 WHERE id=? AND version=?`, c.EstadoNuevo, c.Version, c.OcurridoEn, p.ID, c.Version-1)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrConflictoVersion
		}
		return insertarHistorial(ctx, tx, p.ID, c)
	})
}

// Ready comprueba la conexión y que la migración de pedidos esté aplicada.
func (r *Repositorio) Ready(ctx context.Context) error {
	var n int
	return r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pedido p
 JOIN pedido_item i ON i.pedido_id=p.id JOIN pedido_historial h ON h.pedido_id=p.id WHERE 1=0`).Scan(&n)
}

func insertarHistorial(ctx context.Context, tx *sql.Tx, pedidoID string, c domain.CambioEstado) error {
	var anterior any
	if c.EstadoAnterior != "" {
		anterior = c.EstadoAnterior
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO pedido_historial
 (pedido_id,version,estado_anterior,estado_nuevo,actor_id,actor_rol,ocurrido_en) VALUES (?,?,?,?,?,?,?)`,
		pedidoID, c.Version, anterior, c.EstadoNuevo, c.ActorID, c.ActorRol, c.OcurridoEn)
	return err
}

func (r *Repositorio) enTransaccion(ctx context.Context, f func(*sql.Tx) error) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = f(tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("confirmar transacción: %w", err)
	}
	return nil
}
