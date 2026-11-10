package mysql

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"ghostkitchen/pedidos/internal/domain"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

// CarritosMySQL guarda el carrito activo de cada cliente (una fila por cliente).
type CarritosMySQL struct {
	DB *sql.DB
}

// Obtener devuelve el carrito del cliente, o uno vacío con versión 0 si todavía no tiene.
func (repo CarritosMySQL) Obtener(ctx context.Context, clienteID string) (domain.Carrito, error) {
	// 1. La cabecera del carrito.
	c := domain.Carrito{ClienteID: clienteID, Items: []domain.ItemCarrito{}}
	var cocina, reserva sql.NullString
	var vence, vencio sql.NullTime
	err := repo.DB.QueryRowContext(ctx, `SELECT id,cocina_id,reserva_id,vence_en,vencio_en,version FROM carrito WHERE cliente_id=?`,
		clienteID).Scan(&c.ID, &cocina, &reserva, &vence, &vencio, &c.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return c, nil
	}
	if err != nil {
		return domain.Carrito{}, err
	}
	c.CocinaID, c.ReservaID = cocina.String, reserva.String
	if vence.Valid {
		v := vence.Time
		c.VenceEn = &v
	}
	if vencio.Valid {
		v := vencio.Time
		c.VencioEn = &v
	}

	// 2. Sus platos.
	filas, err := repo.DB.QueryContext(ctx, `SELECT plato_id,nombre,restaurante_id,precio_unitario,cantidad
 FROM carrito_item WHERE cliente_id=? ORDER BY agregado_en, plato_id`, clienteID)
	if err != nil {
		return domain.Carrito{}, err
	}
	defer filas.Close()
	for filas.Next() {
		var it domain.ItemCarrito
		if err := filas.Scan(&it.PlatoID, &it.Nombre, &it.RestauranteID, &it.PrecioUnitario, &it.Cantidad); err != nil {
			return domain.Carrito{}, err
		}
		it.Subtotal = it.PrecioUnitario * it.Cantidad
		c.Total += it.Subtotal
		c.Items = append(c.Items, it)
	}
	return c, nil
}

// Guardar escribe el carrito completo solo si sigue en versionAnterior (control optimista).
func (repo CarritosMySQL) Guardar(ctx context.Context, c domain.Carrito, versionAnterior int64) error {
	tx, err := repo.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. La cabecera: si es nuevo se inserta; si no, se actualiza solo si nadie lo cambió.
	ahora := time.Now().UTC()
	if versionAnterior == 0 {
		_, err = tx.ExecContext(ctx, `INSERT INTO carrito (cliente_id,id,cocina_id,reserva_id,vence_en,version,actualizado_en)
 VALUES (?,?,?,?,?,?,?)`, c.ClienteID, c.ID, nulo(c.CocinaID), nulo(c.ReservaID), c.VenceEn, c.Version, ahora)
		// Si dos solicitudes crean el carrito a la vez, la segunda choca con la clave primaria.
		var errMySQL *driver.MySQLError
		if errors.As(err, &errMySQL) && errMySQL.Number == 1062 {
			return domain.ErrCarritoModificado
		}
		if err != nil {
			return err
		}
	} else {
		resultado, err := tx.ExecContext(ctx, `UPDATE carrito SET id=?, cocina_id=?, reserva_id=?, vence_en=?, vencio_en=NULL, version=?, actualizado_en=?
 WHERE cliente_id=? AND version=?`, c.ID, nulo(c.CocinaID), nulo(c.ReservaID), c.VenceEn, c.Version, ahora, c.ClienteID, versionAnterior)
		if err != nil {
			return err
		}
		filas, err := resultado.RowsAffected()
		if err != nil {
			return err
		}
		if filas == 0 {
			return domain.ErrCarritoModificado
		}
	}

	// 2. La clave del cliente (si mandó una), en la misma transacción: si ya estaba, este cambio
	//    ya se hizo y no se guarda nada.
	if c.Operacion != "" {
		_, err := tx.ExecContext(ctx, `INSERT INTO carrito_operacion (cliente_id,clave,creada_en) VALUES (?,?,?)`, c.ClienteID, c.Operacion, ahora)
		var errMySQL *driver.MySQLError
		if errors.As(err, &errMySQL) && errMySQL.Number == 1062 {
			return domain.ErrOperacionRepetida
		}
		if err != nil {
			return err
		}
	}

	// 3. Los platos: borramos los anteriores y guardamos los actuales.
	if _, err := tx.ExecContext(ctx, `DELETE FROM carrito_item WHERE cliente_id=?`, c.ClienteID); err != nil {
		return err
	}
	for i, it := range c.Items {
		_, err := tx.ExecContext(ctx, `INSERT INTO carrito_item (cliente_id,plato_id,nombre,restaurante_id,precio_unitario,cantidad,agregado_en)
 VALUES (?,?,?,?,?,?,?)`, c.ClienteID, it.PlatoID, it.Nombre, it.RestauranteID, it.PrecioUnitario, it.Cantidad, i)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// OperacionAplicada dice si esa Idempotency-Key ya se aplicó al carrito del cliente.
func (repo CarritosMySQL) OperacionAplicada(ctx context.Context, clienteID, clave string) (bool, error) {
	var n int
	err := repo.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM carrito_operacion WHERE cliente_id=? AND clave=?`, clienteID, clave).Scan(&n)
	return n > 0, err
}

// nulo guarda NULL en lugar de un texto vacío.
func nulo(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// MarcarVencido vacía el carrito que todavía tiene esa reserva, le da otro id y anota cuándo
// venció. La condición `reserva_id = ?` va en el mismo UPDATE: si el carrito ya no tiene esa
// reserva (el cliente lo vació, lo confirmó o ya se procesó este aviso), no cambia nada.
func (repo CarritosMySQL) MarcarVencido(ctx context.Context, carritoID, reservaID string) (bool, error) {
	tx, err := repo.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	// 1. Buscamos de quién es el carrito (los platos se guardan por cliente).
	var clienteID string
	err = tx.QueryRowContext(ctx, `SELECT cliente_id FROM carrito WHERE id=? AND reserva_id=? FOR UPDATE`,
		carritoID, reservaID).Scan(&clienteID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	// 2. Lo vaciamos con otro id (la próxima reserva es otra) y anotamos que venció.
	ahora := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE carrito SET id=?, cocina_id=NULL, reserva_id=NULL, vence_en=NULL,
 vencio_en=?, version=version+1, actualizado_en=? WHERE cliente_id=?`, uuid.NewString(), ahora, ahora, clienteID); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM carrito_item WHERE cliente_id=?`, clienteID); err != nil {
		return false, err
	}
	return true, tx.Commit()
}
