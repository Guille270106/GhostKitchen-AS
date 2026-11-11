package mysql

import (
	"context"
	"database/sql"
	"time"

	"ghostkitchen/pedidos/internal/ports"
)

// ComprobantesMySQL sigue las facturas de los pedidos (tabla comprobante).
type ComprobantesMySQL struct {
	DB *sql.DB
}

// Pendientes devuelve los pedidos con la factura sin emitir, los que hace más que no se prueban primero.
func (repo ComprobantesMySQL) Pendientes(ctx context.Context, limite int) ([]string, error) {
	filas, err := repo.DB.QueryContext(ctx, `SELECT pedido_id FROM comprobante
 WHERE estado='PENDIENTE' ORDER BY actualizado_en LIMIT ?`, limite)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	ids := []string{}
	for filas.Next() {
		var id string
		if err := filas.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, filas.Err()
}

// MarcarEmitido guarda el id y el número de la factura que dio el proveedor.
func (repo ComprobantesMySQL) MarcarEmitido(ctx context.Context, pedidoID string, c ports.ComprobanteEmitido) error {
	_, err := repo.DB.ExecContext(ctx, `UPDATE comprobante SET estado='EMITIDO', comprobante_id=?, numero=?, actualizado_en=?
 WHERE pedido_id=?`, c.ID, c.Numero, time.Now().UTC(), pedidoID)
	return err
}

// MarcarRechazado anota que el proveedor la rechazó y por qué.
func (repo ComprobantesMySQL) MarcarRechazado(ctx context.Context, pedidoID, motivo string) error {
	if len(motivo) > 255 {
		motivo = motivo[:255]
	}
	_, err := repo.DB.ExecContext(ctx, `UPDATE comprobante SET estado='RECHAZADO', motivo=?, actualizado_en=? WHERE pedido_id=?`,
		motivo, time.Now().UTC(), pedidoID)
	return err
}

// SumarIntento anota un intento sin respuesta (y la manda al final de la fila).
func (repo ComprobantesMySQL) SumarIntento(ctx context.Context, pedidoID string) error {
	_, err := repo.DB.ExecContext(ctx, `UPDATE comprobante SET intentos=intentos+1, actualizado_en=? WHERE pedido_id=?`,
		time.Now().UTC(), pedidoID)
	return err
}

// anotarComprobante deja la factura del pedido PENDIENTE, dentro de la transacción que crea el
// pedido: si el pedido se crea, su factura queda anotada sí o sí.
func anotarComprobante(ctx context.Context, tx *sql.Tx, pedidoID string, ahora time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO comprobante (pedido_id,estado,intentos,creado_en,actualizado_en)
 VALUES (?,'PENDIENTE',0,?,?)`, pedidoID, ahora, ahora)
	return err
}
