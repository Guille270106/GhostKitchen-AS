package mysql

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"ghostkitchen/pedidos/internal/domain"
)

// MetricasMySQL calcula las métricas del admin (HU22, G21) con consultas de solo lectura.
// Implementa ports.MetricasRepo. Todas filtran por cocina y fecha, que es el índice
// idx_pedido_cocina (cocina_id, estado, creado_en).
type MetricasMySQL struct {
	DB *sql.DB
}

// convertirDia pasa creado_en (guardado en UTC) a la fecha de Córdoba. CONVERT_TZ con
// desplazamientos fijos no necesita las tablas de zonas horarias de MySQL.
const convertirDia = `DATE_FORMAT(CONVERT_TZ(p.creado_en,'+00:00','-03:00'),'%Y-%m-%d')`

// PedidosPorCocina cuenta los pedidos no cancelados de cada cocina desde una fecha.
func (repo MetricasMySQL) PedidosPorCocina(ctx context.Context, cocinaIDs []string, desde time.Time) (map[string]int64, error) {
	res := map[string]int64{}
	if len(cocinaIDs) == 0 {
		return res, nil
	}
	args := make([]any, 0, len(cocinaIDs)+1)
	for _, id := range cocinaIDs {
		args = append(args, id)
	}
	args = append(args, desde.UTC())
	filas, err := repo.DB.QueryContext(ctx, `SELECT p.cocina_id, COUNT(*) FROM pedido p
 WHERE p.cocina_id IN (?`+strings.Repeat(",?", len(cocinaIDs)-1)+`) AND p.creado_en >= ? AND p.estado <> 'CANCELADO'
 GROUP BY p.cocina_id`, args...)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	for filas.Next() {
		var id string
		var n int64
		if err := filas.Scan(&id, &n); err != nil {
			return nil, err
		}
		res[id] = n
	}
	return res, filas.Err()
}

// MetricasDeCocina arma el detalle de una cocina: totales, pedidos por día, platos más
// vendidos, pedidos por restaurante y tiempo promedio hasta la entrega.
func (repo MetricasMySQL) MetricasDeCocina(ctx context.Context, cocinaID string, desde time.Time) (domain.MetricasCocina, map[string]int64, error) {
	var m domain.MetricasCocina
	d := desde.UTC()

	// 1. Totales. La facturación es la suma de los PLATOS de los pedidos no cancelados (p.total,
	//    sin envío): así la cuenta es la misma para todos los pedidos, también los anteriores a
	//    guardar el envío. La pantalla lo dice: "Ventas de platos, sin envíos".
	if err := repo.DB.QueryRowContext(ctx, `SELECT
 COALESCE(SUM(p.estado <> 'CANCELADO'),0),
 COALESCE(SUM(p.estado = 'CANCELADO'),0),
 COALESCE(SUM(CASE WHEN p.estado <> 'CANCELADO' THEN p.total END),0)
 FROM pedido p WHERE p.cocina_id=? AND p.creado_en >= ?`, cocinaID, d).Scan(&m.Pedidos, &m.Cancelados, &m.Facturacion); err != nil {
		return m, nil, err
	}

	// 2. Pedidos por día, en hora de Córdoba.
	porDia := map[string]int64{}
	filas, err := repo.DB.QueryContext(ctx, `SELECT `+convertirDia+` AS dia, COUNT(*) FROM pedido p
 WHERE p.cocina_id=? AND p.creado_en >= ? AND p.estado <> 'CANCELADO' GROUP BY dia`, cocinaID, d)
	if err != nil {
		return m, nil, err
	}
	for filas.Next() {
		var dia string
		var n int64
		if err := filas.Scan(&dia, &n); err != nil {
			filas.Close()
			return m, nil, err
		}
		porDia[dia] = n
	}
	filas.Close()
	if err := filas.Err(); err != nil {
		return m, nil, err
	}

	// 3. Los 5 platos más vendidos (por unidades). El nombre es el que tenía al pedirlo.
	filas, err = repo.DB.QueryContext(ctx, `SELECT i.plato_id, MAX(i.nombre), MAX(i.restaurante_nombre), SUM(i.cantidad) AS vendidos
 FROM pedido_item i JOIN pedido p ON p.id = i.pedido_id
 WHERE p.cocina_id=? AND p.creado_en >= ? AND p.estado <> 'CANCELADO'
 GROUP BY i.plato_id ORDER BY vendidos DESC, i.plato_id LIMIT 5`, cocinaID, d)
	if err != nil {
		return m, nil, err
	}
	for filas.Next() {
		var pv domain.PlatoVendido
		if err := filas.Scan(&pv.PlatoID, &pv.Nombre, &pv.RestauranteNombre, &pv.Vendidos); err != nil {
			filas.Close()
			return m, nil, err
		}
		m.TopPlatos = append(m.TopPlatos, pv)
	}
	filas.Close()
	if err := filas.Err(); err != nil {
		return m, nil, err
	}

	// 4. En cuántos pedidos aparece cada restaurante (un pedido puede tener varios).
	filas, err = repo.DB.QueryContext(ctx, `SELECT i.restaurante_id, MAX(i.restaurante_nombre), COUNT(DISTINCT i.pedido_id) AS pedidos
 FROM pedido_item i JOIN pedido p ON p.id = i.pedido_id
 WHERE p.cocina_id=? AND p.creado_en >= ? AND p.estado <> 'CANCELADO'
 GROUP BY i.restaurante_id ORDER BY pedidos DESC, i.restaurante_id`, cocinaID, d)
	if err != nil {
		return m, nil, err
	}
	for filas.Next() {
		var pm domain.PedidosMarca
		if err := filas.Scan(&pm.RestauranteID, &pm.Nombre, &pm.Pedidos); err != nil {
			filas.Close()
			return m, nil, err
		}
		m.PorMarca = append(m.PorMarca, pm)
	}
	filas.Close()
	if err := filas.Err(); err != nil {
		return m, nil, err
	}

	// 5. Minutos promedio desde que se recibió hasta que se entregó.
	var segundos sql.NullFloat64
	if err := repo.DB.QueryRowContext(ctx, `SELECT AVG(TIMESTAMPDIFF(SECOND, p.creado_en, h.ocurrido_en))
 FROM pedido p JOIN pedido_historial h ON h.pedido_id = p.id AND h.estado_nuevo = 'ENTREGADO'
 WHERE p.cocina_id=? AND p.creado_en >= ?`, cocinaID, d).Scan(&segundos); err != nil {
		return m, nil, err
	}
	if segundos.Valid {
		m.MinutosPromedio = int64(segundos.Float64/60 + 0.5)
	}
	return m, porDia, nil
}
