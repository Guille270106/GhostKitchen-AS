package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/ports"
)

type cocinasMock struct {
	porLocalidad map[string][]ports.CocinaLocalidad
	err          error
}

func (m cocinasMock) CocinasDeLocalidad(_ context.Context, loc string) ([]ports.CocinaLocalidad, error) {
	return m.porLocalidad[loc], m.err
}

type metricasMock struct {
	pidioIDs []string
	detalle  string
}

func (m *metricasMock) PedidosPorCocina(_ context.Context, ids []string, _ time.Time) (map[string]int64, error) {
	m.pidioIDs = ids
	return map[string]int64{"nc": 12, "guemes": 3}, nil
}

func (m *metricasMock) MetricasDeCocina(_ context.Context, id string, _ time.Time) (domain.MetricasCocina, map[string]int64, error) {
	m.detalle = id
	return domain.MetricasCocina{
		Pedidos: 4, Facturacion: 50000,
		PorMarca: []domain.PedidosMarca{{RestauranteID: "r1", Nombre: "Mostaza", Pedidos: 3}, {RestauranteID: "r2", Nombre: "Sushi", Pedidos: 2}},
	}, map[string]int64{"2026-11-09": 3, "2026-11-04": 1}, nil
}

var (
	adminNC = domain.Actor{UsuarioID: "adm", Rol: domain.RolAdmin, Localidad: "Nueva Córdoba"}
	// Lunes 9 de noviembre de 2026, 1:00 en UTC: en Córdoba todavía es domingo 8 a las 22.
	ahoraMetricas = func() time.Time { return time.Date(2026, 11, 9, 1, 0, 0, 0, time.UTC) }
)

func servicioMetricas(repo *metricasMock) MetricasService {
	return MetricasService{
		Cocinas: cocinasMock{porLocalidad: map[string][]ports.CocinaLocalidad{
			"Nueva Córdoba": {{ID: "nc", Nombre: "Cocina Nueva Córdoba"}, {ID: "guemes", Nombre: "Cocina Güemes"}},
			"Cerro":         {{ID: "cerro", Nombre: "Cocina Cerro"}},
		}},
		Repo:  repo,
		Ahora: ahoraMetricas,
	}
}

func TestMetricasDeLasCocinasDeSuLocalidad(t *testing.T) {
	repo := &metricasMock{}
	m, err := servicioMetricas(repo).Obtener(context.Background(), adminNC, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.pidioIDs) != 2 || repo.detalle != "nc" {
		t.Fatalf("tenía que contar sus 2 cocinas y detallar la primera: %v %s", repo.pidioIDs, repo.detalle)
	}
	if len(m.Cocinas) != 2 || m.Cocinas[0].Pedidos != 12 || m.Cocinas[1].Nombre != "Cocina Güemes" {
		t.Fatalf("%+v", m.Cocinas)
	}
	d := m.Detalle
	if d.Nombre != "Cocina Nueva Córdoba" || d.TicketPromedio != 12500 || d.PorMarca[0].Porcentaje != 75 || d.PorMarca[1].Porcentaje != 50 {
		t.Fatalf("%+v", d)
	}
	// 7 días en hora de Córdoba: del lunes 2 al domingo 8 (el lunes 9 todavía no empezó allá).
	if len(d.Dias) != 7 || d.Dias[0].Fecha != "2026-11-02" || d.Dias[6].Fecha != "2026-11-08" || d.Dias[2].Cantidad != 1 {
		t.Fatalf("%+v", d.Dias)
	}
	if d.TopPlatos == nil {
		t.Fatal("sin ventas, la lista va vacía y no null")
	}
}

func TestMetricasDeOtraCocina(t *testing.T) {
	repo := &metricasMock{}
	m, err := servicioMetricas(repo).Obtener(context.Background(), adminNC, "guemes")
	if err != nil || m.Detalle.CocinaID != "guemes" {
		t.Fatalf("%v %+v", err, m.Detalle)
	}
}

func TestMetricasSoloDeSuLocalidad(t *testing.T) {
	repo := &metricasMock{}
	_, err := servicioMetricas(repo).Obtener(context.Background(), adminNC, "cerro")
	if !errors.Is(err, domain.ErrSinPermiso) || repo.detalle != "" {
		t.Fatalf("la cocina de otra localidad no se puede ver: %v", err)
	}
}

func TestMetricasSoloParaAdmins(t *testing.T) {
	for _, actor := range []domain.Actor{
		{UsuarioID: "c1", Rol: domain.RolCocina, CocinaID: "nc", Localidad: "Nueva Córdoba"},
		{UsuarioID: "u1", Rol: domain.RolCliente},
		{UsuarioID: "adm", Rol: domain.RolAdmin}, // sin localidad
	} {
		if _, err := servicioMetricas(&metricasMock{}).Obtener(context.Background(), actor, ""); !errors.Is(err, domain.ErrSinPermiso) {
			t.Errorf("%+v: %v", actor, err)
		}
	}
	if _, err := servicioMetricas(&metricasMock{}).Obtener(context.Background(), domain.Actor{}, ""); !errors.Is(err, domain.ErrSinIdentidad) {
		t.Errorf("sin identidad: %v", err)
	}
}

func TestMetricasSinCocinasOConCatalogoCaido(t *testing.T) {
	s := servicioMetricas(&metricasMock{})
	m, err := s.Obtener(context.Background(), domain.Actor{UsuarioID: "a", Rol: domain.RolAdmin, Localidad: "Villa Allende"}, "")
	if err != nil || len(m.Cocinas) != 0 || m.Detalle != nil {
		t.Fatalf("una localidad sin cocinas: %v %+v", err, m)
	}
	s.Cocinas = cocinasMock{err: domain.ErrCatalogoCaido}
	if _, err := s.Obtener(context.Background(), adminNC, ""); !errors.Is(err, domain.ErrCatalogoCaido) {
		t.Fatalf("con Catálogo caído: %v", err)
	}
}
