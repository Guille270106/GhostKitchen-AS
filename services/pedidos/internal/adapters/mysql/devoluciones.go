package mysql

import (
	"context"
	"database/sql"
	"time"

	"ghostkitchen/pedidos/internal/ports"
)

// DevolucionesMySQL sigue las devoluciones de ingredientes pendientes (tabla devolucion).
// Las crea PedidosMySQL.GuardarCambio cuando un pedido pasa a CANCELADO.
type DevolucionesMySQL struct {
	DB *sql.DB
}

// Pendientes devuelve las devoluciones sin hacer, las que hace más tiempo que no se prueban primero.
func (repo DevolucionesMySQL) Pendientes(ctx context.Context, limite int) ([]ports.DevolucionPendiente, error) {
	filas, err := repo.DB.QueryContext(ctx, `SELECT pedido_id,intentos FROM devolucion
 WHERE estado='PENDIENTE' ORDER BY actualizada_en LIMIT ?`, limite)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	lista := []ports.DevolucionPendiente{}
	for filas.Next() {
		var d ports.DevolucionPendiente
		if err := filas.Scan(&d.PedidoID, &d.Intentos); err != nil {
			return nil, err
		}
		lista = append(lista, d)
	}
	return lista, filas.Err()
}

// MarcarHecha anota que Inventario ya devolvió los ingredientes.
func (repo DevolucionesMySQL) MarcarHecha(ctx context.Context, pedidoID string) error {
	_, err := repo.DB.ExecContext(ctx, `UPDATE devolucion SET estado='HECHA', actualizada_en=? WHERE pedido_id=?`,
		time.Now().UTC(), pedidoID)
	return err
}

// SumarIntento anota un intento fallido (y la manda al final de la fila).
func (repo DevolucionesMySQL) SumarIntento(ctx context.Context, pedidoID string) error {
	_, err := repo.DB.ExecContext(ctx, `UPDATE devolucion SET intentos=intentos+1, actualizada_en=? WHERE pedido_id=?`,
		time.Now().UTC(), pedidoID)
	return err
}

// anotarDevolucion crea la devolución pendiente de un pedido cancelado, dentro de la transacción
// del cambio de estado. Solo si el pedido vino de una confirmación (ahí se descontó el stock); si no,
// no hubo descuento y no anota nada.
func anotarDevolucion(ctx context.Context, tx *sql.Tx, pedidoID string, ahora time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO devolucion (pedido_id,estado,intentos,creada_en,actualizada_en)
 SELECT pedido_id, 'PENDIENTE', 0, ?, ? FROM confirmacion WHERE pedido_id=?`, ahora, ahora, pedidoID)
	return err
}
