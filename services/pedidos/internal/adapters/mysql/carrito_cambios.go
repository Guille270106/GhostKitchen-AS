package mysql

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/ports"
)

type conexionCarritoKey struct{}

type consultasCarrito interface {
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func conexionPedidos(ctx context.Context, db *sql.DB) consultasCarrito {
	if conn, ok := ctx.Value(conexionCarritoKey{}).(*sql.Conn); ok {
		return conn
	}
	return db
}

func (repo CarritosMySQL) ConBloqueo(ctx context.Context, clienteID string, hacer func(context.Context) error) error {
	conn, err := repo.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	huella := sha256.Sum256([]byte(clienteID))
	nombre := fmt.Sprintf("pedidos-carrito-%x", huella[:20])
	var tomado sql.NullInt64
	err = conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 3)", nombre).Scan(&tomado)
	if err != nil {
		// Si se perdió la respuesta, la sesión puede haber adquirido el lock.
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		return err
	}
	if tomado.Int64 != 1 {
		return domain.ErrCarritoModificado
	}
	defer func() {
		limpieza, cancelar := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancelar()
		var liberado sql.NullInt64
		if err := conn.QueryRowContext(limpieza, "SELECT RELEASE_LOCK(?)", nombre).Scan(&liberado); err != nil || liberado.Int64 != 1 {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	return hacer(context.WithValue(ctx, conexionCarritoKey{}, conn))
}

func (repo CarritosMySQL) IniciarCambio(ctx context.Context, clienteID string, op ports.CambioCarrito) error {
	db := conexionPedidos(ctx, repo.DB)
	var confirmando bool
	err := db.QueryRowContext(ctx, `SELECT EXISTS(
 SELECT 1 FROM confirmacion c JOIN carrito a ON a.id=c.carrito_id
 WHERE a.cliente_id=? AND c.estado='EN_CURSO')`, clienteID).Scan(&confirmando)
	if err != nil {
		return err
	}
	if confirmando {
		return domain.ErrCarritoModificado
	}
	datos, err := json.Marshal(op)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO carrito_operacion
 (cliente_id, clave, interna, estado, datos, creada_en) VALUES (?, ?, TRUE, 'incierta', ?, ?)`,
		clienteID, op.ID, string(datos), time.Now().UTC())
	return err
}

func (repo CarritosMySQL) CambioPendiente(ctx context.Context, clienteID string) (*ports.CambioCarrito, error) {
	var datos []byte
	var estado ports.EstadoCambio
	err := conexionPedidos(ctx, repo.DB).QueryRowContext(ctx, `SELECT datos, estado FROM carrito_operacion
 WHERE cliente_id=? AND interna=TRUE AND estado IN ('incierta','por_liberar')`, clienteID).Scan(&datos, &estado)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var op ports.CambioCarrito
	if err := json.Unmarshal(datos, &op); err != nil {
		return nil, err
	}
	op.Destino.ClienteID, op.Estado = clienteID, estado
	return &op, nil
}

func (repo CarritosMySQL) TerminarCambio(ctx context.Context, clienteID, id string, estado ports.EstadoCambio) error {
	_, err := conexionPedidos(ctx, repo.DB).ExecContext(ctx, `UPDATE carrito_operacion SET estado=?, datos=NULL
 WHERE cliente_id=? AND clave=? AND interna=TRUE AND estado IN ('incierta','por_liberar')`,
		estado, clienteID, id)
	return err
}

func (repo CarritosMySQL) ClientesConCambiosPendientes(ctx context.Context) ([]string, error) {
	filas, err := repo.DB.QueryContext(ctx, `SELECT DISTINCT cliente_id FROM carrito_operacion
 WHERE interna=TRUE AND estado IN ('incierta','por_liberar')`)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	var clientes []string
	for filas.Next() {
		var clienteID string
		if err := filas.Scan(&clienteID); err != nil {
			return nil, err
		}
		clientes = append(clientes, clienteID)
	}
	return clientes, filas.Err()
}

func (repo CarritosMySQL) ReintentarCompensacion(ctx context.Context, op ports.CambioCarrito) error {
	datos, err := json.Marshal(op)
	if err != nil {
		return err
	}
	_, err = conexionPedidos(ctx, repo.DB).ExecContext(ctx, `UPDATE carrito_operacion SET datos=?
 WHERE cliente_id=? AND clave=? AND interna=TRUE AND estado='incierta'`,
		string(datos), op.Destino.ClienteID, op.ID)
	return err
}
