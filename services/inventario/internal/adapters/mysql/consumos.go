package mysql

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"sort"
	"time"

	"ghostkitchen/inventario/internal/domain"
	"ghostkitchen/inventario/internal/ports"
	driver "github.com/go-sql-driver/mysql"
)

var _ ports.Consumos = (*Repositorio)(nil)

const consultaConsumo = `SELECT pedido_id,reserva_id,consumido_en,devuelto_en FROM consumo WHERE pedido_id=?`

func leerConsumo(fila *sql.Row) (domain.Consumo, error) {
	var c domain.Consumo
	err := fila.Scan(&c.PedidoID, &c.ReservaID, &c.ConsumidoEn, &c.DevueltoEn)
	if errors.Is(err, sql.ErrNoRows) {
		return c, domain.ErrNoEncontrado
	}
	c.Estado = "CONSUMIDO"
	if c.DevueltoEn != nil {
		c.Estado = "DEVUELTO"
	}
	return c, err
}

func (r *Repositorio) ObtenerConsumo(ctx context.Context, pedidoID string) (domain.Consumo, error) {
	return leerConsumo(r.db.QueryRowContext(ctx, consultaConsumo, pedidoID))
}

func (r *Repositorio) Consumir(ctx context.Context, clave, reservaID, pedidoID string) (domain.Consumo, error) {
	datos := struct{ ReservaID, PedidoID string }{reservaID, pedidoID}
	return idempotente(r, ctx, "consumir", clave, datos, func(tx *sql.Tx) (domain.Consumo, error) {
		res, _, err := leerReserva(ctx, tx, reservaID, true)
		if err != nil {
			return domain.Consumo{}, err
		}
		switch res.Estado {
		case "VENCIDA":
			return domain.Consumo{}, domain.ErrReservaVencida
		case "CONSUMIDA":
			c, err := leerConsumo(tx.QueryRowContext(ctx, consultaConsumo+" FOR UPDATE", pedidoID))
			if errors.Is(err, domain.ErrNoEncontrado) || (err == nil && c.ReservaID != reservaID) {
				return domain.Consumo{}, domain.ErrReservaConflicto
			}
			return c, err
		case "ACTIVA":
		default:
			return domain.Consumo{}, domain.ErrReservaConflicto
		}
		var ahora time.Time
		if err = tx.QueryRowContext(ctx, `SELECT UTC_TIMESTAMP(6)`).Scan(&ahora); err != nil {
			return domain.Consumo{}, err
		}
		if !res.VenceEn.After(ahora) {
			if _, err = r.vencerReserva(ctx, tx, res, ahora); err != nil {
				return domain.Consumo{}, err
			}
			return domain.Consumo{}, domain.ErrReservaVencida
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO consumo (pedido_id,reserva_id,consumido_en) VALUES (?,?,?)`, pedidoID, reservaID, ahora)
		var duplicado *driver.MySQLError
		if errors.As(err, &duplicado) && duplicado.Number == 1062 {
			return domain.Consumo{}, domain.ErrReservaConflicto
		}
		if err != nil {
			return domain.Consumo{}, err
		}
		if err = moverStock(ctx, tx, res, false); err != nil {
			return domain.Consumo{}, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE reserva SET estado='CONSUMIDA' WHERE id=?`, reservaID); err != nil {
			return domain.Consumo{}, err
		}
		return domain.Consumo{PedidoID: pedidoID, ReservaID: reservaID, Estado: "CONSUMIDO", ConsumidoEn: ahora}, nil
	})
}

func (r *Repositorio) Devolver(ctx context.Context, clave, pedidoID string) (domain.Consumo, error) {
	return idempotente(r, ctx, "devolver", clave, pedidoID, func(tx *sql.Tx) (domain.Consumo, error) {
		c, err := leerConsumo(tx.QueryRowContext(ctx, consultaConsumo+" FOR UPDATE", pedidoID))
		if err != nil || c.DevueltoEn != nil {
			return c, err
		}
		// El detalle de una reserva CONSUMIDA es inmutable.
		res, _, err := leerReserva(ctx, tx, c.ReservaID, false)
		if err != nil {
			return c, err
		}
		if err = moverStock(ctx, tx, res, true); err != nil {
			return c, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE consumo SET devuelto_en=UTC_TIMESTAMP(6) WHERE pedido_id=?`, pedidoID); err != nil {
			return c, err
		}
		return leerConsumo(tx.QueryRowContext(ctx, consultaConsumo, pedidoID))
	})
}

func moverStock(ctx context.Context, tx *sql.Tx, res domain.Reserva, devolver bool) error {
	cantidades, err := domain.Totales(res.Items)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(cantidades))
	for id := range cantidades {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		n := cantidades[id]
		var resultado sql.Result
		if devolver {
			resultado, err = tx.ExecContext(ctx, `UPDATE ingrediente SET total=total+?
 WHERE id=? AND cocina_id=? AND total<=?`, n, id, res.CocinaID, int64(math.MaxInt64)-n)
		} else {
			resultado, err = tx.ExecContext(ctx, `UPDATE ingrediente SET total=total-?,reservado=reservado-?
 WHERE id=? AND cocina_id=? AND reservado>=?`, n, n, id, res.CocinaID, n)
		}
		if err != nil {
			return err
		}
		filas, err := resultado.RowsAffected()
		if err != nil {
			return err
		}
		if filas != 1 {
			return errors.New("no se pudo aplicar el movimiento de stock")
		}
	}
	return actualizarDisponibilidad(ctx, tx, res.CocinaID)
}
