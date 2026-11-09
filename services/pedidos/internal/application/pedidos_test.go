package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"ghostkitchen/pedidos/internal/domain"
)

// PedidosMock simula ser MySQL, pero vive en la RAM (como el ItemsMock de clase): GuardarCambio solo guarda si la versión no cambió.
type PedidosMock struct {
	mu      sync.Mutex
	pedidos map[string]domain.Pedido
}

func (r *PedidosMock) Crear(_ context.Context, p domain.Pedido) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pedidos[p.ID] = p
	return nil
}

func (r *PedidosMock) Obtener(_ context.Context, id string) (domain.Pedido, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.pedidos[id]
	if !ok {
		return domain.Pedido{}, domain.ErrNoEncontrado
	}
	p.Historial = append([]domain.CambioEstado(nil), p.Historial...)
	return p, nil
}

func (r *PedidosMock) GuardarCambio(_ context.Context, p domain.Pedido, c domain.CambioEstado) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pedidos[p.ID].Version != c.Version-1 {
		return domain.ErrConflictoVersion
	}
	r.pedidos[p.ID] = p
	return nil
}

func (r *PedidosMock) Ready(context.Context) error { return nil }

var (
	cliente = domain.Actor{UsuarioID: "u-cliente", Rol: domain.RolCliente}
	cocina  = domain.Actor{UsuarioID: "u-cocina", Rol: domain.RolCocina, CocinaID: "cerro"}
)

func entorno(t *testing.T) (PedidosService, *PedidosMock) {
	t.Helper()
	repo := &PedidosMock{pedidos: map[string]domain.Pedido{}}
	p, err := domain.NuevoPedido("ped-1", cliente, "c@ejemplo.com", "cerro",
		[]domain.Item{{PlatoID: "p17", Nombre: "Burger", RestauranteID: "r5", PrecioUnitario: 12000, Cantidad: 1}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	_ = repo.Crear(context.Background(), p)
	return PedidosService{Repo: repo}, repo
}

func TestCambiarEstadoGuardaElCambio(t *testing.T) {
	svc, repo := entorno(t)
	p, err := svc.CambiarEstado(context.Background(), cocina, "ped-1", Aceptar)
	if err != nil || p.Estado != domain.EstadoAceptado {
		t.Fatal(err, p.Estado)
	}
	if guardado := repo.pedidos["ped-1"]; guardado.Version != 2 || len(guardado.Historial) != 2 {
		t.Fatal("no se guardó el cambio:", guardado.Version, len(guardado.Historial))
	}
}

func TestObtenerPedidoAjenoEs404(t *testing.T) {
	svc, _ := entorno(t)
	if _, err := svc.Obtener(context.Background(), domain.Actor{UsuarioID: "otro", Rol: domain.RolCliente}, "ped-1"); !errors.Is(err, domain.ErrNoEncontrado) {
		t.Fatal(err)
	}
	if _, err := svc.Obtener(context.Background(), cliente, "ped-1"); err != nil {
		t.Fatal(err)
	}
}

// El cliente cancela y la cocina acepta al mismo tiempo: solo una de las dos acciones gana.
func TestCancelarYAceptarALaVezGanaUnaSola(t *testing.T) {
	for i := 0; i < 50; i++ {
		svc, repo := entorno(t)
		var wg sync.WaitGroup
		errs := make([]error, 2)
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, errs[0] = svc.CambiarEstado(context.Background(), cliente, "ped-1", Cancelar)
		}()
		go func() {
			defer wg.Done()
			_, errs[1] = svc.CambiarEstado(context.Background(), cocina, "ped-1", Aceptar)
		}()
		wg.Wait()

		exitos := 0
		for _, err := range errs {
			switch {
			case err == nil:
				exitos++
			case errors.Is(err, domain.ErrConflictoVersion), errors.Is(err, domain.ErrTransicionInvalida):
			default:
				t.Fatal("error inesperado:", err)
			}
		}
		final := repo.pedidos["ped-1"]
		if exitos != 1 || final.Version != 2 || len(final.Historial) != 2 {
			t.Fatalf("vuelta %d: %d acciones ganaron, versión %d", i, exitos, final.Version)
		}
	}
}

func TestAccionDesconocida(t *testing.T) {
	svc, _ := entorno(t)
	if _, err := svc.CambiarEstado(context.Background(), cocina, "ped-1", Accion("borrar")); !errors.Is(err, domain.ErrDatosInvalidos) {
		t.Fatal(err)
	}
}
