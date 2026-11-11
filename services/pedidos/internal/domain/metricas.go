package domain

import "time"

// Métricas del negocio para el admin de cocina. Son lecturas: no cambian ningún
// pedido. Se calculan con los pedidos de los últimos días, sin contar los cancelados.

// DiasMetricas es cuántos días mira la pantalla de métricas (hoy y los 6 anteriores).
const DiasMetricas = 7

// ZonaMetricas es la hora con la que se cortan los días: un pedido a las 23:30 de Córdoba
// cuenta para ese día, aunque en UTC ya sea el siguiente.
var ZonaMetricas = time.FixedZone("ART", -3*60*60)

// ResumenCocina es cuántos pedidos tuvo una cocina de la localidad del admin.
type ResumenCocina struct {
	CocinaID string `json:"cocinaId"`
	Nombre   string `json:"nombre"`
	Pedidos  int64  `json:"pedidos"`
}

// PedidosDia es la cantidad de pedidos de un día.
type PedidosDia struct {
	Fecha    string `json:"fecha"` // AAAA-MM-DD, en hora de Córdoba
	Cantidad int64  `json:"cantidad"`
}

// PlatoVendido es un plato del ranking de más vendidos.
type PlatoVendido struct {
	PlatoID           string `json:"platoId"`
	Nombre            string `json:"nombre"`
	RestauranteNombre string `json:"restauranteNombre"`
	Vendidos          int64  `json:"vendidos"`
}

// PedidosMarca es en cuántos pedidos aparece cada restaurante.
type PedidosMarca struct {
	RestauranteID string `json:"restauranteId"`
	Nombre        string `json:"nombre"`
	Pedidos       int64  `json:"pedidos"`
	Porcentaje    int64  `json:"porcentaje"`
}

// MetricasCocina es el detalle de una cocina.
type MetricasCocina struct {
	CocinaID        string         `json:"cocinaId"`
	Nombre          string         `json:"nombre"`
	Pedidos         int64          `json:"pedidos"`
	Cancelados      int64          `json:"cancelados"`
	Facturacion     int64          `json:"facturacion"`
	TicketPromedio  int64          `json:"ticketPromedio"`
	MinutosPromedio int64          `json:"minutosPromedio"` // de recibido a entregado; 0 si no hay entregados
	Dias            []PedidosDia   `json:"dias"`
	TopPlatos       []PlatoVendido `json:"topPlatos"`
	PorMarca        []PedidosMarca `json:"porMarca"`
}

// Metricas es lo que ve el admin: el resumen de todas sus cocinas y el detalle de una.
type Metricas struct {
	Localidad string          `json:"localidad"`
	Desde     time.Time       `json:"desde"`
	Cocinas   []ResumenCocina `json:"cocinas"`
	Detalle   *MetricasCocina `json:"detalle,omitempty"`
}

// InicioMetricas es el comienzo del primer día que se mira: las 0 h de hace 6 días, en hora
// de Córdoba.
func InicioMetricas(ahora time.Time) time.Time {
	a := ahora.In(ZonaMetricas)
	hoy := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, ZonaMetricas)
	return hoy.AddDate(0, 0, -(DiasMetricas - 1))
}

// Completar ordena los días (con 0 los que no tuvieron pedidos), calcula el ticket promedio y
// el porcentaje de cada marca.
func (m *MetricasCocina) Completar(desde time.Time, porDia map[string]int64) {
	m.Dias = make([]PedidosDia, 0, DiasMetricas)
	for i := 0; i < DiasMetricas; i++ {
		f := desde.In(ZonaMetricas).AddDate(0, 0, i).Format("2006-01-02")
		m.Dias = append(m.Dias, PedidosDia{Fecha: f, Cantidad: porDia[f]})
	}
	if m.Pedidos > 0 {
		m.TicketPromedio = m.Facturacion / m.Pedidos
	}
	for i := range m.PorMarca {
		if m.Pedidos > 0 {
			m.PorMarca[i].Porcentaje = (m.PorMarca[i].Pedidos*100 + m.Pedidos/2) / m.Pedidos
		}
	}
	if m.TopPlatos == nil {
		m.TopPlatos = []PlatoVendido{}
	}
	if m.PorMarca == nil {
		m.PorMarca = []PedidosMarca{}
	}
}
