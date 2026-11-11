package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"ghostkitchen/pedidos/internal/domain"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

// ConfirmacionesMySQL guarda la saga de confirmación en MySQL.
type ConfirmacionesMySQL struct {
	DB *sql.DB
}

const columnasConfirmacion = `id,cliente_id,cliente_email,clave,carrito_id,reserva_id,cocina_id,pedido_id,items,estado,paso,motivo,creada_en,costo_envio,direccion_entrega`

// Iniciar guarda la confirmación. Si choca con una clave o un carrito ya usados, devuelve la
// que ya estaba: así un doble clic, o dos solicitudes a la vez, terminan en la misma.
func (repo ConfirmacionesMySQL) Iniciar(ctx context.Context, c domain.Confirmacion) (domain.Confirmacion, bool, error) {
	items, err := json.Marshal(c.Items)
	if err != nil {
		return domain.Confirmacion{}, false, err
	}
	ahora := time.Now().UTC()
	_, err = conexionPedidos(ctx, repo.DB).ExecContext(ctx, `INSERT INTO confirmacion
 (id,cliente_id,cliente_email,clave,carrito_id,reserva_id,cocina_id,pedido_id,items,estado,paso,creada_en,actualizada_en,costo_envio,direccion_entrega)
 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.ID, c.ClienteID, c.ClienteEmail, c.Clave, c.CarritoID, c.ReservaID, c.CocinaID, c.PedidoID, items, c.Estado, c.Paso, ahora, ahora, c.CostoEnvio, nuloSiVacio(c.DireccionEntrega))

	// 1062 = clave duplicada: ya existía una con esa clave o con ese carrito.
	var errMySQL *driver.MySQLError
	if errors.As(err, &errMySQL) && errMySQL.Number == 1062 {
		existente, err := repo.leer(ctx, `WHERE (cliente_id=? AND clave=?) OR carrito_id=?`, c.ClienteID, c.Clave, c.CarritoID)
		return existente, true, err
	}
	if err != nil {
		return domain.Confirmacion{}, false, err
	}
	return c, false, nil
}

// Obtener devuelve una confirmación por id.
func (repo ConfirmacionesMySQL) Obtener(ctx context.Context, id string) (domain.Confirmacion, error) {
	return repo.leer(ctx, `WHERE id=?`, id)
}

// BuscarPorClave devuelve la confirmación de ese cliente con esa clave.
func (repo ConfirmacionesMySQL) BuscarPorClave(ctx context.Context, clienteID, clave string) (domain.Confirmacion, error) {
	return repo.leer(ctx, `WHERE cliente_id=? AND clave=?`, clienteID, clave)
}

// MarcarPaso anota el próximo paso.
func (repo ConfirmacionesMySQL) MarcarPaso(ctx context.Context, id string, paso domain.PasoConfirmacion) error {
	_, err := conexionPedidos(ctx, repo.DB).ExecContext(ctx, `UPDATE confirmacion SET paso=?, actualizada_en=? WHERE id=? AND estado='EN_CURSO'`,
		paso, time.Now().UTC(), id)
	return err
}

// Completar crea el pedido, marca la confirmación COMPLETADA y vacía el carrito, en UNA sola
// transacción: o pasan las tres cosas o ninguna.
func (repo ConfirmacionesMySQL) Completar(ctx context.Context, c domain.Confirmacion, p domain.Pedido) error {
	tx, err := conexionPedidos(ctx, repo.DB).BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Marcamos la confirmación. Si ya estaba completada (por ejemplo, la terminó el proceso
	//    de recuperación al mismo tiempo), no hacemos nada más.
	resultado, err := tx.ExecContext(ctx, `UPDATE confirmacion SET estado='COMPLETADA', actualizada_en=?
 WHERE id=? AND estado='EN_CURSO'`, time.Now().UTC(), c.ID)
	if err != nil {
		return err
	}
	if filas, _ := resultado.RowsAffected(); filas == 0 {
		return nil
	}

	// 2. Creamos el pedido con sus platos y su historial.
	if err := insertarPedido(ctx, tx, p); err != nil {
		return err
	}

	// 2b. Y anotamos su factura como pendiente: la pide otro proceso al proveedor.
	if err := anotarComprobante(ctx, tx, p.ID, time.Now().UTC()); err != nil {
		return err
	}

	// 3. Vaciamos el carrito.
	if err := vaciarCarrito(ctx, tx, c); err != nil {
		return err
	}
	return tx.Commit()
}

// Fallar marca la confirmación FALLIDA y vacía el carrito (la reserva ya no sirve).
func (repo ConfirmacionesMySQL) Fallar(ctx context.Context, c domain.Confirmacion, motivo string) error {
	tx, err := conexionPedidos(ctx, repo.DB).BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	resultado, err := tx.ExecContext(ctx, `UPDATE confirmacion SET estado='FALLIDA', motivo=?, actualizada_en=?
 WHERE id=? AND estado='EN_CURSO'`, motivo, time.Now().UTC(), c.ID)
	if err != nil {
		return err
	}
	if filas, _ := resultado.RowsAffected(); filas == 0 {
		return nil
	}
	if err := vaciarCarrito(ctx, tx, c); err != nil {
		return err
	}
	return tx.Commit()
}

// Pendientes devuelve las confirmaciones EN_CURSO que no cambiaron desde antes de "antes".
func (repo ConfirmacionesMySQL) Pendientes(ctx context.Context, antes time.Time) ([]domain.Confirmacion, error) {
	filas, err := repo.DB.QueryContext(ctx, `SELECT `+columnasConfirmacion+` FROM confirmacion
 WHERE estado='EN_CURSO' AND actualizada_en < ? ORDER BY creada_en LIMIT 50`, antes.UTC())
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	lista := []domain.Confirmacion{}
	for filas.Next() {
		c, err := escanear(filas)
		if err != nil {
			return nil, err
		}
		lista = append(lista, c)
	}
	return lista, nil
}

// vaciarCarrito deja el carrito sin platos y con otro id, solo si sigue siendo el que se confirmó.
func vaciarCarrito(ctx context.Context, tx *sql.Tx, c domain.Confirmacion) error {
	resultado, err := tx.ExecContext(ctx, `UPDATE carrito SET id=?, cocina_id=NULL, reserva_id=NULL, vence_en=NULL,
 version=version+1, actualizado_en=? WHERE cliente_id=? AND id=?`, uuid.NewString(), time.Now().UTC(), c.ClienteID, c.CarritoID)
	if err != nil {
		return err
	}
	if filas, _ := resultado.RowsAffected(); filas == 0 {
		return nil // el cliente ya empezó otro carrito: no lo tocamos
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM carrito_item WHERE cliente_id=?`, c.ClienteID)
	return err
}

// leer busca una confirmación con la condición indicada.
func (repo ConfirmacionesMySQL) leer(ctx context.Context, condicion string, args ...any) (domain.Confirmacion, error) {
	fila := conexionPedidos(ctx, repo.DB).QueryRowContext(ctx, `SELECT `+columnasConfirmacion+` FROM confirmacion `+condicion+` LIMIT 1`, args...)
	c, err := escanear(fila)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Confirmacion{}, domain.ErrNoEncontrado
	}
	return c, err
}

// escanear lee una fila de confirmación (sirve para QueryRow y para Query).
func escanear(fila interface{ Scan(...any) error }) (domain.Confirmacion, error) {
	var c domain.Confirmacion
	var items []byte
	var motivo sql.NullString
	var envio sql.NullInt64
	var direccion sql.NullString
	err := fila.Scan(&c.ID, &c.ClienteID, &c.ClienteEmail, &c.Clave, &c.CarritoID, &c.ReservaID, &c.CocinaID,
		&c.PedidoID, &items, &c.Estado, &c.Paso, &motivo, &c.CreadaEn, &envio, &direccion)
	if err != nil {
		return domain.Confirmacion{}, err
	}
	c.Motivo = motivo.String
	c.DireccionEntrega = direccion.String
	if envio.Valid {
		c.CostoEnvio = &envio.Int64
	}
	if err := json.Unmarshal(items, &c.Items); err != nil {
		return domain.Confirmacion{}, err
	}
	return c, nil
}
