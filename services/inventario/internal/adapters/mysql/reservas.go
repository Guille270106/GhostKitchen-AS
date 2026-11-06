package mysql

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"ghostkitchen/inventario/internal/domain"
	"ghostkitchen/inventario/internal/ports"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

var _ ports.Reservas = (*Repositorio)(nil)

type resultadoReserva struct {
	Reserva domain.Reserva `json:"reserva"`
	Error   string         `json:"error,omitempty"`
}

var erroresReserva = map[string]error{
	"SIN_STOCK": domain.ErrSinStock, "RESERVA_VENCIDA": domain.ErrReservaVencida,
	"RESERVA_CONFLICTO": domain.ErrReservaConflicto, "NO_ENCONTRADO": domain.ErrNoEncontrado,
	"DATOS_INVALIDOS": domain.ErrReservaInvalida,
}

func codigoReserva(err error) string {
	for codigo, candidato := range erroresReserva {
		if errors.Is(err, candidato) {
			return codigo
		}
	}
	return ""
}

// Clave, resultado y efectos se confirman en la misma transacción. Nunca se deja
// una clave EN_PROCESO confirmada sin efecto. Los duplicados esperan el commit.
func (r *Repositorio) idempotente(ctx context.Context, operacion, clave string, datos any, efecto func(*sql.Tx) (domain.Reserva, error)) (domain.Reserva, error) {
	for intento := 0; ; intento++ {
		res, err := r.idempotenteIntento(ctx, operacion, clave, datos, efecto)
		var mysqlErr *driver.MySQLError
		if intento >= 2 || !errors.As(err, &mysqlErr) || mysqlErr.Number != 1213 {
			return res, err
		}
		// El deadlock deshizo la transacción completa; repetir con la misma clave.
		select {
		case <-ctx.Done():
			return domain.Reserva{}, ctx.Err()
		case <-time.After(time.Duration(intento+1) * 20 * time.Millisecond):
		}
	}
}

func (r *Repositorio) idempotenteIntento(ctx context.Context, operacion, clave string, datos any, efecto func(*sql.Tx) (domain.Reserva, error)) (domain.Reserva, error) {
	encoded, err := json.Marshal(datos)
	if err != nil {
		return domain.Reserva{}, err
	}
	hash := sha256.Sum256(encoded)
	huella := hex.EncodeToString(hash[:])
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Reserva{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO idempotencia (operacion,clave,huella) VALUES (?,?,?) ON DUPLICATE KEY UPDATE clave=clave`, operacion, clave, huella)
	if err != nil {
		var mysqlErr *driver.MySQLError
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || (errors.As(err, &mysqlErr) && mysqlErr.Number == 1205) {
			return domain.Reserva{}, domain.ErrEnProceso
		}
		return domain.Reserva{}, err
	}
	var anterior string
	var respuesta []byte
	if err = tx.QueryRowContext(ctx, `SELECT huella,resultado FROM idempotencia WHERE operacion=? AND clave=? FOR UPDATE`, operacion, clave).Scan(&anterior, &respuesta); err != nil {
		return domain.Reserva{}, err
	}
	if anterior != huella {
		return domain.Reserva{}, domain.ErrIdempotencia
	}
	if len(respuesta) > 0 {
		var result resultadoReserva
		if err = json.Unmarshal(respuesta, &result); err != nil {
			return domain.Reserva{}, err
		}
		return result.Reserva, erroresReserva[result.Error]
	}
	if _, err = tx.ExecContext(ctx, `SAVEPOINT efecto`); err != nil {
		return domain.Reserva{}, err
	}
	reserva, fallo := efecto(tx)
	result := resultadoReserva{Reserva: reserva}
	if fallo != nil {
		result.Error = codigoReserva(fallo)
		if result.Error == "" {
			return domain.Reserva{}, fallo
		}
		// Deshacer todas las escrituras de negocio, pero conservar el rechazo estable.
		if _, err = tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT efecto`); err != nil {
			return domain.Reserva{}, err
		}
	}
	respuesta, err = json.Marshal(result)
	if err != nil {
		return domain.Reserva{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE idempotencia SET resultado=? WHERE operacion=? AND clave=?`, string(respuesta), operacion, clave); err != nil {
		return domain.Reserva{}, err
	}
	if err = tx.Commit(); err != nil {
		return domain.Reserva{}, err
	}
	return reserva, fallo
}

func leerReserva(ctx context.Context, tx *sql.Tx, id string, bloquear bool) (domain.Reserva, time.Time, error) {
	var res domain.Reserva
	var ahora time.Time
	query := `SELECT id,carrito_id,cocina_id,estado,vence_en,UTC_TIMESTAMP(6) FROM reserva WHERE id=?`
	if bloquear {
		query += ` FOR UPDATE`
	}
	err := tx.QueryRowContext(ctx, query, id).Scan(&res.ID, &res.CarritoID, &res.CocinaID, &res.Estado, &res.VenceEn, &ahora)
	if errors.Is(err, sql.ErrNoRows) {
		err = domain.ErrNoEncontrado
	}
	if err != nil {
		return res, ahora, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT plato_id,cantidad,lotes FROM reserva_plato WHERE reserva_id=? ORDER BY plato_id`, id)
	if err != nil {
		return res, ahora, err
	}
	defer rows.Close()
	res.Items = []domain.PlatoReservado{}
	for rows.Next() {
		var p domain.PlatoReservado
		var lotes []byte
		if err = rows.Scan(&p.PlatoID, &p.Cantidad, &lotes); err != nil {
			return res, ahora, err
		}
		if err = json.Unmarshal(lotes, &p.Lotes); err != nil {
			return res, ahora, err
		}
		res.Items = append(res.Items, p)
	}
	return res, ahora, rows.Err()
}

func recetaParaReserva(ctx context.Context, tx *sql.Tx, cocina, plato string) (domain.Receta, error) {
	var existe int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM plato_ref p JOIN cocina_ref c ON c.id=p.cocina_id
 WHERE p.id=? AND p.cocina_id=? AND p.activo=1 AND c.activa=1 FOR SHARE`, plato, cocina).Scan(&existe)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Receta{}, domain.ErrNoEncontrado
	}
	if err != nil {
		return domain.Receta{}, err
	}
	var receta domain.Receta
	err = tx.QueryRowContext(ctx, `SELECT version FROM receta WHERE plato_id=? AND cocina_id=? FOR SHARE`, plato, cocina).Scan(&receta.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return receta, domain.ErrNoEncontrado
	}
	if err != nil {
		return receta, err
	}
	// Lectura actual bajo bloqueo: no usar un snapshot anterior de la transacción.
	rows, err := tx.QueryContext(ctx, `SELECT ingrediente_id,cantidad FROM receta_item WHERE plato_id=? ORDER BY ingrediente_id FOR SHARE`, plato)
	if err != nil {
		return receta, err
	}
	defer rows.Close()
	for rows.Next() {
		var i domain.RecetaItem
		if err = rows.Scan(&i.IngredienteID, &i.Cantidad); err != nil {
			return receta, err
		}
		receta.Items = append(receta.Items, i)
	}
	if err = rows.Err(); err != nil {
		return receta, err
	}
	if len(receta.Items) == 0 {
		return receta, domain.ErrNoEncontrado
	}
	return receta, nil
}

func (r *Repositorio) aplicarItems(ctx context.Context, tx *sql.Tx, res domain.Reserva, deseados []domain.PlatoSolicitado) (domain.Reserva, error) {
	anteriores := map[string]domain.PlatoReservado{}
	for _, p := range res.Items {
		anteriores[p.PlatoID] = p
	}
	nuevos := []domain.PlatoReservado{}
	for _, pedido := range deseados {
		previo := anteriores[pedido.PlatoID]
		p := domain.PlatoReservado{PlatoID: pedido.PlatoID, Cantidad: pedido.Cantidad, Lotes: previo.Lotes}
		if pedido.Cantidad < previo.Cantidad {
			p.Lotes = domain.Reducir(previo.Lotes, previo.Cantidad-pedido.Cantidad)
		}
		if pedido.Cantidad > previo.Cantidad {
			receta, err := recetaParaReserva(ctx, tx, res.CocinaID, pedido.PlatoID)
			if err != nil {
				return res, err
			}
			p.Lotes = append(append([]domain.LoteReserva(nil), previo.Lotes...), domain.LoteReserva{Unidades: pedido.Cantidad - previo.Cantidad, RecetaVersion: receta.Version, Ingredientes: receta.Items})
		}
		nuevos = append(nuevos, p)
	}
	antes, err := domain.Totales(res.Items)
	if err != nil {
		return res, err
	}
	despues, err := domain.Totales(nuevos)
	if err != nil {
		return res, err
	}
	ids := map[string]bool{}
	for id := range antes {
		ids[id] = true
	}
	for id := range despues {
		ids[id] = true
	}
	orden := make([]string, 0, len(ids))
	for id := range ids {
		orden = append(orden, id)
	}
	sort.Strings(orden)
	for _, id := range orden {
		delta := despues[id] - antes[id]
		if delta == 0 {
			continue
		}
		var result sql.Result
		if delta > 0 {
			result, err = tx.ExecContext(ctx, `UPDATE ingrediente SET reservado=reservado+? WHERE id=? AND cocina_id=? AND total-reservado>=?`, delta, id, res.CocinaID, delta)
		} else {
			result, err = tx.ExecContext(ctx, `UPDATE ingrediente SET reservado=reservado-? WHERE id=? AND cocina_id=? AND reservado>=?`, -delta, id, res.CocinaID, -delta)
		}
		if err != nil {
			return res, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return res, err
		}
		if n != 1 {
			if delta > 0 {
				return res, domain.ErrSinStock
			}
			return res, errors.New("inconsistencia de stock reservado")
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM reserva_item WHERE reserva_id=?`, res.ID); err != nil {
		return res, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM reserva_plato WHERE reserva_id=?`, res.ID); err != nil {
		return res, err
	}
	for _, p := range nuevos {
		lotes, err := json.Marshal(p.Lotes)
		if err != nil {
			return res, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO reserva_plato (reserva_id,plato_id,cocina_id,cantidad,lotes) VALUES (?,?,?,?,?)`, res.ID, p.PlatoID, res.CocinaID, p.Cantidad, string(lotes)); err != nil {
			return res, err
		}
	}
	for _, id := range orden {
		if despues[id] > 0 {
			if _, err = tx.ExecContext(ctx, `INSERT INTO reserva_item (reserva_id,ingrediente_id,cocina_id,cantidad) VALUES (?,?,?,?)`, res.ID, id, res.CocinaID, despues[id]); err != nil {
				return res, err
			}
		}
	}
	res.Items = nuevos
	if len(nuevos) == 0 {
		res.Estado = "LIBERADA"
		if _, err = tx.ExecContext(ctx, `UPDATE reserva SET estado='LIBERADA' WHERE id=?`, res.ID); err != nil {
			return res, err
		}
	}
	return res, nil
}

func (r *Repositorio) CrearReserva(ctx context.Context, clave string, datos domain.SolicitudReserva) (domain.Reserva, error) {
	return r.idempotente(ctx, "reservar", clave, datos, func(tx *sql.Tx) (domain.Reserva, error) {
		var existe int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM cocina_ref WHERE id=? AND activa=1 FOR SHARE`, datos.CocinaID).Scan(&existe)
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Reserva{}, domain.ErrNoEncontrado
		}
		if err != nil {
			return domain.Reserva{}, err
		}
		id := uuid.NewString()
		_, err = tx.ExecContext(ctx, `INSERT INTO reserva (id,carrito_id,cocina_id,creada_en,vence_en) VALUES (?,?,?,UTC_TIMESTAMP(6),DATE_ADD(UTC_TIMESTAMP(6),INTERVAL 10 MINUTE))`, id, datos.CarritoID, datos.CocinaID)
		var mysqlErr *driver.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return domain.Reserva{}, domain.ErrReservaConflicto
		}
		if err != nil {
			return domain.Reserva{}, err
		}
		res, _, err := leerReserva(ctx, tx, id, true)
		if err != nil {
			return res, err
		}
		return r.aplicarItems(ctx, tx, res, datos.Items)
	})
}

func (r *Repositorio) modificarReserva(ctx context.Context, op, clave, id string, items []domain.PlatoSolicitado) (domain.Reserva, error) {
	datos := struct {
		ID    string
		Items []domain.PlatoSolicitado
	}{id, items}
	return r.idempotente(ctx, op, clave, datos, func(tx *sql.Tx) (domain.Reserva, error) {
		res, ahora, err := leerReserva(ctx, tx, id, true)
		if err != nil {
			return res, err
		}
		if op == "liberar" && res.Estado == "LIBERADA" {
			return res, nil
		}
		if res.Estado != "ACTIVA" {
			return res, domain.ErrReservaConflicto
		}
		if len(items) > 0 && !res.VenceEn.After(ahora) {
			return res, domain.ErrReservaVencida
		}
		return r.aplicarItems(ctx, tx, res, items)
	})
}
func (r *Repositorio) AjustarReserva(ctx context.Context, clave, id string, items []domain.PlatoSolicitado) (domain.Reserva, error) {
	return r.modificarReserva(ctx, "ajustar", clave, id, items)
}
func (r *Repositorio) LiberarReserva(ctx context.Context, clave, id string) (domain.Reserva, error) {
	return r.modificarReserva(ctx, "liberar", clave, id, []domain.PlatoSolicitado{})
}
func (r *Repositorio) ObtenerReserva(ctx context.Context, id string) (domain.Reserva, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return domain.Reserva{}, err
	}
	defer tx.Rollback()
	res, ahora, err := leerReserva(ctx, tx, id, false)
	if err != nil {
		return res, err
	}
	if res.Estado == "ACTIVA" && !res.VenceEn.After(ahora) {
		res.Estado = "VENCIDA"
	}
	return res, tx.Commit()
}
