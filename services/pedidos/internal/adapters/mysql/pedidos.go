// Package mysql tiene los adaptadores de salida hacia MySQL (pedidos_db).
// Implementan los puertos ports.PedidosRepo y ports.CarritosRepo.
package mysql

import (
	"context"
	"database/sql"
	"errors"

	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/ports"
)

// PedidosMySQL guarda pedidos en MySQL.
type PedidosMySQL struct {
	DB *sql.DB
}

// Crear guarda pedido, items e historial inicial en una sola transacción: o queda todo o nada.
func (repo PedidosMySQL) Crear(ctx context.Context, p domain.Pedido) error {
	tx, err := repo.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	// Si algo falla antes del Commit, se deshace todo. Después del Commit no hace nada.
	defer tx.Rollback()

	if err := insertarPedido(ctx, tx, p); err != nil {
		return err
	}
	return tx.Commit()
}

// insertarPedido guarda pedido, platos e historial dentro de una transacción que ya está abierta.
// La usan Crear y también Completar (la confirmación), que guarda más cosas en la misma transacción.
func insertarPedido(ctx context.Context, tx *sql.Tx, p domain.Pedido) error {
	// 1. El pedido.
	_, err := tx.ExecContext(ctx, `INSERT INTO pedido
 (id,cliente_id,cliente_email,cocina_id,estado,total,moneda,version,creado_en,actualizado_en)
 VALUES (?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.ClienteID, p.ClienteEmail, p.CocinaID, p.Estado, p.Total, p.Moneda, p.Version, p.CreadoEn, p.ActualizadoEn)
	if err != nil {
		return err
	}

	// 2. Sus platos.
	for _, it := range p.Items {
		_, err = tx.ExecContext(ctx, `INSERT INTO pedido_item
 (pedido_id,plato_id,nombre,restaurante_id,restaurante_nombre,precio_unitario,cantidad) VALUES (?,?,?,?,?,?,?)`,
			p.ID, it.PlatoID, it.Nombre, it.RestauranteID, it.RestauranteNombre, it.PrecioUnitario, it.Cantidad)
		if err != nil {
			return err
		}
	}

	// 3. La primera entrada del historial (RECIBIDO).
	for _, c := range p.Historial {
		if err := insertarHistorial(ctx, tx, p.ID, c); err != nil {
			return err
		}
	}
	return nil
}

// Obtener lee el pedido con sus items y su historial ordenado por versión.
func (repo PedidosMySQL) Obtener(ctx context.Context, id string) (domain.Pedido, error) {
	// 1. El pedido.
	var p domain.Pedido
	err := repo.DB.QueryRowContext(ctx, `SELECT id,cliente_id,cliente_email,cocina_id,estado,total,moneda,version,creado_en,actualizado_en
 FROM pedido WHERE id=?`, id).Scan(&p.ID, &p.ClienteID, &p.ClienteEmail, &p.CocinaID, &p.Estado, &p.Total, &p.Moneda, &p.Version, &p.CreadoEn, &p.ActualizadoEn)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Pedido{}, domain.ErrNoEncontrado
	}
	if err != nil {
		return domain.Pedido{}, err
	}

	// 2. Sus platos.
	if p.Items, err = repo.leerItems(ctx, id); err != nil {
		return domain.Pedido{}, err
	}

	// 3. Su historial.
	historial, err := repo.DB.QueryContext(ctx, `SELECT estado_anterior,estado_nuevo,actor_id,actor_rol,version,ocurrido_en
 FROM pedido_historial WHERE pedido_id=? ORDER BY version`, id)
	if err != nil {
		return domain.Pedido{}, err
	}
	defer historial.Close()
	for historial.Next() {
		var c domain.CambioEstado
		var anterior sql.NullString // el primer cambio no tiene estado anterior
		if err := historial.Scan(&anterior, &c.EstadoNuevo, &c.ActorID, &c.ActorRol, &c.Version, &c.OcurridoEn); err != nil {
			return domain.Pedido{}, err
		}
		c.EstadoAnterior = domain.Estado(anterior.String)
		p.Historial = append(p.Historial, c)
	}
	return p, nil
}

// leerItems devuelve los platos de un pedido.
func (repo PedidosMySQL) leerItems(ctx context.Context, pedidoID string) ([]domain.Item, error) {
	filas, err := repo.DB.QueryContext(ctx, `SELECT plato_id,nombre,restaurante_id,restaurante_nombre,precio_unitario,cantidad
 FROM pedido_item WHERE pedido_id=? ORDER BY plato_id`, pedidoID)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	items := []domain.Item{}
	for filas.Next() {
		var it domain.Item
		if err := filas.Scan(&it.PlatoID, &it.Nombre, &it.RestauranteID, &it.RestauranteNombre, &it.PrecioUnitario, &it.Cantidad); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, filas.Err()
}

// Listar devuelve una página de pedidos de un cliente o de una cocina, del más nuevo al más
// viejo, con sus platos (la cocina los necesita para prepararlos). No trae el historial: para
// eso está el detalle.
func (repo PedidosMySQL) Listar(ctx context.Context, f ports.FiltroPedidos) ([]domain.Pedido, error) {
	// 1. Armamos la consulta según el filtro. Los valores van siempre como parámetros (?).
	consulta := `SELECT id,cliente_id,cliente_email,cocina_id,estado,total,moneda,version,creado_en,actualizado_en FROM pedido WHERE `
	var args []any
	if f.ClienteID != "" {
		consulta += "cliente_id=?"
		args = append(args, f.ClienteID)
	} else {
		consulta += "cocina_id=?"
		args = append(args, f.CocinaID)
	}
	if f.Estado != "" {
		consulta += " AND estado=?"
		args = append(args, f.Estado)
	}
	consulta += " ORDER BY creado_en DESC, id LIMIT ? OFFSET ?"
	args = append(args, f.PorPagina, (f.Pagina-1)*f.PorPagina)

	// 2. Leemos los pedidos.
	filas, err := repo.DB.QueryContext(ctx, consulta, args...)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	pedidos := []domain.Pedido{}
	for filas.Next() {
		var p domain.Pedido
		if err := filas.Scan(&p.ID, &p.ClienteID, &p.ClienteEmail, &p.CocinaID, &p.Estado, &p.Total, &p.Moneda, &p.Version, &p.CreadoEn, &p.ActualizadoEn); err != nil {
			return nil, err
		}
		pedidos = append(pedidos, p)
	}
	if err := filas.Err(); err != nil {
		return nil, err
	}

	// 3. Los platos de cada uno (son 20 como mucho).
	for i := range pedidos {
		if pedidos[i].Items, err = repo.leerItems(ctx, pedidos[i].ID); err != nil {
			return nil, err
		}
	}
	return pedidos, nil
}

// GuardarCambio escribe el nuevo estado solo si el pedido sigue en la versión anterior.
// La condición `version = ?` va en el mismo UPDATE: MySQL no deja que dos UPDATE cambien la
// misma fila a la vez, así que si dos cambios llegan juntos, el segundo ya no encuentra la
// versión vieja y no escribe nada (control optimista, ADR-004).
func (repo PedidosMySQL) GuardarCambio(ctx context.Context, p domain.Pedido, c domain.CambioEstado) error {
	tx, err := repo.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Cambiamos el estado solo si nadie lo cambió antes.
	resultado, err := tx.ExecContext(ctx, `UPDATE pedido SET estado=?, version=?, actualizado_en=?
 WHERE id=? AND version=?`, c.EstadoNuevo, c.Version, c.OcurridoEn, p.ID, c.Version-1)
	if err != nil {
		return err
	}
	filas, err := resultado.RowsAffected()
	if err != nil {
		return err
	}
	if filas == 0 {
		return domain.ErrConflictoVersion
	}

	// 2. Anotamos el cambio en el historial, en la misma transacción.
	if err := insertarHistorial(ctx, tx, p.ID, c); err != nil {
		return err
	}
	return tx.Commit()
}

// Ready comprueba la conexión y que las tablas existan (las crean las migraciones).
func (repo PedidosMySQL) Ready(ctx context.Context) error {
	var n int
	return repo.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pedido p
 JOIN pedido_item i ON i.pedido_id=p.id JOIN pedido_historial h ON h.pedido_id=p.id
 JOIN carrito c ON c.cliente_id=p.cliente_id JOIN carrito_item ci ON ci.cliente_id=c.cliente_id
 JOIN confirmacion cf ON cf.pedido_id=p.id WHERE 1=0`).Scan(&n)
}

func insertarHistorial(ctx context.Context, tx *sql.Tx, pedidoID string, c domain.CambioEstado) error {
	var anterior any // NULL en la primera entrada
	if c.EstadoAnterior != "" {
		anterior = c.EstadoAnterior
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO pedido_historial
 (pedido_id,version,estado_anterior,estado_nuevo,actor_id,actor_rol,ocurrido_en) VALUES (?,?,?,?,?,?,?)`,
		pedidoID, c.Version, anterior, c.EstadoNuevo, c.ActorID, c.ActorRol, c.OcurridoEn)
	return err
}
