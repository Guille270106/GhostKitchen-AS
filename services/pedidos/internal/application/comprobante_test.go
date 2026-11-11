package application

import (
	"context"
	"fmt"
	"testing"
	"time"

	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/ports"
)

// ComprobantesMock guarda el estado de la factura de cada pedido.
type ComprobantesMock struct {
	estados  map[string]domain.EstadoComprobante
	numeros  map[string]string
	intentos map[string]int
}

func (m *ComprobantesMock) Pendientes(_ context.Context, limite int) ([]string, error) {
	ids := []string{}
	for id, e := range m.estados {
		if e == domain.ComprobantePendiente && len(ids) < limite {
			ids = append(ids, id)
		}
	}
	return ids, nil
}
func (m *ComprobantesMock) MarcarEmitido(_ context.Context, id string, c ports.ComprobanteEmitido) error {
	m.estados[id], m.numeros[id] = domain.ComprobanteEmitido, c.Numero
	return nil
}
func (m *ComprobantesMock) MarcarRechazado(_ context.Context, id, _ string) error {
	m.estados[id] = domain.ComprobanteRechazado
	return nil
}
func (m *ComprobantesMock) SumarIntento(_ context.Context, id string) error {
	m.intentos[id]++
	return nil
}

// ProveedorMock es el Grupo 4: anota lo que le pidieron y responde lo que se le diga.
type ProveedorMock struct {
	claves      []string
	solicitudes []ports.SolicitudComprobante
	err         error
}

func (m *ProveedorMock) Emitir(_ context.Context, clave string, s ports.SolicitudComprobante) (ports.ComprobanteEmitido, error) {
	m.claves = append(m.claves, clave)
	m.solicitudes = append(m.solicitudes, s)
	if m.err != nil {
		return ports.ComprobanteEmitido{}, m.err
	}
	return ports.ComprobanteEmitido{ID: "cmp-1", Numero: "B-0001-00000042"}, nil
}

func entornoComprobante(t *testing.T) (ComprobanteService, *ComprobantesMock, *ProveedorMock) {
	t.Helper()
	pedidos := &PedidosMock{pedidos: map[string]domain.Pedido{}}
	p, err := domain.NuevoPedido("ped-1", cliente, "cliente@ejemplo.com", "cerro",
		[]domain.Item{{PlatoID: "p17", Nombre: "Burger", RestauranteID: "r5", PrecioUnitario: 12000, Cantidad: 2}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	_ = pedidos.Crear(context.Background(), p)
	repo := &ComprobantesMock{
		estados:  map[string]domain.EstadoComprobante{"ped-1": domain.ComprobantePendiente},
		numeros:  map[string]string{},
		intentos: map[string]int{},
	}
	proveedor := &ProveedorMock{}
	return ComprobanteService{Repo: repo, Pedidos: pedidos, Proveedor: proveedor}, repo, proveedor
}

func TestSePideLaFacturaConLosDatosDelPedido(t *testing.T) {
	svc, repo, proveedor := entornoComprobante(t)
	emitidos, err := svc.EmitirPendientes(context.Background())
	if err != nil || emitidos != 1 || repo.estados["ped-1"] != domain.ComprobanteEmitido || repo.numeros["ped-1"] != "B-0001-00000042" {
		t.Fatal(err, emitidos, repo.estados)
	}
	s := proveedor.solicitudes[0]
	if proveedor.claves[0] != "pedido-ped-1" || s.Email != "cliente@ejemplo.com" || len(s.Lineas) != 1 || s.Lineas[0].Importe != 24000 {
		t.Fatalf("solicitud: %v %+v", proveedor.claves, s)
	}
	// Ya emitida: no se vuelve a pedir.
	if emitidos, _ := svc.EmitirPendientes(context.Background()); emitidos != 0 || len(proveedor.claves) != 1 {
		t.Fatal("no se pide dos veces")
	}
}

// El proveedor caído no afecta al pedido: la factura queda pendiente y se reintenta con la
// misma clave (así no hay dos facturas).
func TestProveedorCaidoLaFacturaQuedaPendiente(t *testing.T) {
	svc, repo, proveedor := entornoComprobante(t)
	proveedor.err = domain.ErrProveedorCaido
	for i := 0; i < 2; i++ {
		if emitidos, err := svc.EmitirPendientes(context.Background()); err != nil || emitidos != 0 {
			t.Fatal(err, emitidos)
		}
	}
	if repo.estados["ped-1"] != domain.ComprobantePendiente || repo.intentos["ped-1"] != 2 {
		t.Fatal(repo.estados, repo.intentos)
	}
	proveedor.err = nil
	if emitidos, _ := svc.EmitirPendientes(context.Background()); emitidos != 1 {
		t.Fatal("cuando vuelve se emite")
	}
	for _, c := range proveedor.claves {
		if c != "pedido-ped-1" {
			t.Fatal("siempre la misma clave:", proveedor.claves)
		}
	}
}

func TestSiElProveedorLaRechazaNoSeReintenta(t *testing.T) {
	svc, repo, proveedor := entornoComprobante(t)
	proveedor.err = fmt.Errorf("%w: 422 Regla incumplida", domain.ErrProveedorRechazo)
	_, _ = svc.EmitirPendientes(context.Background())
	_, _ = svc.EmitirPendientes(context.Background())
	if repo.estados["ped-1"] != domain.ComprobanteRechazado || len(proveedor.claves) != 1 {
		t.Fatal(repo.estados, proveedor.claves)
	}
}
