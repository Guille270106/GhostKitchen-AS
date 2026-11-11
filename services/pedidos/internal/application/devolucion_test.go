package application

import (
	"context"
	"testing"

	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/ports"
)

// DevolucionesMock guarda las devoluciones en un mapa: pendiente o hecha, con sus intentos.
type DevolucionesMock struct {
	lista map[string]*ports.DevolucionPendiente
	hecha map[string]bool
}

func (m *DevolucionesMock) Pendientes(_ context.Context, limite int) ([]ports.DevolucionPendiente, error) {
	pendientes := []ports.DevolucionPendiente{}
	for id, d := range m.lista {
		if !m.hecha[id] && len(pendientes) < limite {
			pendientes = append(pendientes, *d)
		}
	}
	return pendientes, nil
}
func (m *DevolucionesMock) MarcarHecha(_ context.Context, pedidoID string) error {
	m.hecha[pedidoID] = true
	return nil
}
func (m *DevolucionesMock) SumarIntento(_ context.Context, pedidoID string) error {
	m.lista[pedidoID].Intentos++
	return nil
}

// DevolucionMock es Inventario: anota las claves y responde lo que se le diga.
type DevolucionMock struct {
	claves []string
	err    error
}

func (m *DevolucionMock) Devolver(_ context.Context, clave, _ string) error {
	m.claves = append(m.claves, clave)
	return m.err
}

func entornoDevolucion() (DevolucionService, *DevolucionesMock, *DevolucionMock) {
	repo := &DevolucionesMock{
		lista: map[string]*ports.DevolucionPendiente{"ped-1": {PedidoID: "ped-1"}},
		hecha: map[string]bool{},
	}
	inventario := &DevolucionMock{}
	return DevolucionService{Repo: repo, Inventario: inventario}, repo, inventario
}

func TestDevolverLosIngredientesDeUnPedidoCancelado(t *testing.T) {
	svc, repo, inventario := entornoDevolucion()
	hechas, err := svc.ProcesarPendientes(context.Background())
	if err != nil || hechas != 1 || !repo.hecha["ped-1"] || len(inventario.claves) != 1 || inventario.claves[0] != "ped-1-devolver" {
		t.Fatal(err, hechas, repo.hecha, inventario.claves)
	}
	// Ya hecha: la próxima vuelta no la vuelve a mandar.
	if hechas, _ := svc.ProcesarPendientes(context.Background()); hechas != 0 || len(inventario.claves) != 1 {
		t.Fatal("no se devuelve dos veces:", hechas, inventario.claves)
	}
}

func TestInventarioCaidoLaDevolucionQuedaPendienteYSeReintentaConLaMismaClave(t *testing.T) {
	svc, repo, inventario := entornoDevolucion()
	inventario.err = domain.ErrInventarioCaido

	// 1. Dos vueltas con Inventario caído: sigue pendiente y suma intentos.
	for i := 0; i < 2; i++ {
		if hechas, err := svc.ProcesarPendientes(context.Background()); err != nil || hechas != 0 {
			t.Fatal(err, hechas)
		}
	}
	if repo.hecha["ped-1"] || repo.lista["ped-1"].Intentos != 2 {
		t.Fatal("tiene que seguir pendiente con 2 intentos:", repo.lista["ped-1"].Intentos)
	}

	// 2. Inventario vuelve: se hace, con la misma clave de siempre.
	inventario.err = nil
	if hechas, _ := svc.ProcesarPendientes(context.Background()); hechas != 1 || !repo.hecha["ped-1"] {
		t.Fatal("cuando Inventario vuelve se tiene que hacer")
	}
	for _, c := range inventario.claves {
		if c != "ped-1-devolver" {
			t.Fatal("todos los intentos usan la misma clave:", inventario.claves)
		}
	}
}

func TestSiInventarioNoTieneElDescuentoNoHayNadaQueDevolver(t *testing.T) {
	svc, repo, inventario := entornoDevolucion()
	inventario.err = domain.ErrNoEncontrado
	if hechas, err := svc.ProcesarPendientes(context.Background()); err != nil || hechas != 1 || !repo.hecha["ped-1"] {
		t.Fatal(err, hechas)
	}
}
