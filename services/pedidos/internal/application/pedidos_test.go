package application

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/ports"
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

// Listar filtra igual que la consulta de MySQL: del más nuevo al más viejo y de a una página.
func (r *PedidosMock) Listar(_ context.Context, f ports.FiltroPedidos) ([]domain.Pedido, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	lista := []domain.Pedido{}
	for _, p := range r.pedidos {
		if f.ClienteID != "" && p.ClienteID != f.ClienteID || f.CocinaID != "" && p.CocinaID != f.CocinaID || f.Estado != "" && p.Estado != f.Estado {
			continue
		}
		lista = append(lista, p)
	}
	sort.Slice(lista, func(i, j int) bool { return lista[i].CreadoEn.After(lista[j].CreadoEn) })
	desde := (f.Pagina - 1) * f.PorPagina
	if desde >= len(lista) {
		return []domain.Pedido{}, nil
	}
	return lista[desde:min(desde+f.PorPagina, len(lista))], nil
}

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

// agregarPedido crea otro pedido en el mock, minutos después del anterior.
func agregarPedido(t *testing.T, repo *PedidosMock, id string, quien domain.Actor, cocinaID string, minutos int) {
	t.Helper()
	p, err := domain.NuevoPedido(id, quien, "c@ejemplo.com", cocinaID,
		[]domain.Item{{PlatoID: "p17", Nombre: "Burger", RestauranteID: "r5", PrecioUnitario: 12000, Cantidad: 1}},
		time.Now().Add(time.Duration(minutos)*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	_ = repo.Crear(context.Background(), p)
}

func TestListarMisPedidos(t *testing.T) {
	svc, repo := entorno(t)
	agregarPedido(t, repo, "ped-2", cliente, "cerro", 1)
	agregarPedido(t, repo, "ped-3", domain.Actor{UsuarioID: "otro", Rol: domain.RolCliente}, "cerro", 2)

	// 1. El cliente ve solo los suyos, del más nuevo al más viejo.
	lista, err := svc.Listar(context.Background(), cliente, "", 1)
	if err != nil || len(lista) != 2 || lista[0].ID != "ped-2" || lista[1].ID != "ped-1" {
		t.Fatal(err, lista)
	}

	// 2. Con filtro de estado.
	if _, err := svc.CambiarEstado(context.Background(), cocina, "ped-1", Aceptar); err != nil {
		t.Fatal(err)
	}
	lista, err = svc.Listar(context.Background(), cliente, domain.EstadoAceptado, 1)
	if err != nil || len(lista) != 1 || lista[0].ID != "ped-1" {
		t.Fatal(err, lista)
	}

	// 3. Una página que no tiene pedidos devuelve la lista vacía, no un error.
	lista, err = svc.Listar(context.Background(), cliente, "", 2)
	if err != nil || len(lista) != 0 {
		t.Fatal(err, lista)
	}
}

func TestListarDeLaCocinaSoloLosDeSuCocina(t *testing.T) {
	svc, repo := entorno(t)
	agregarPedido(t, repo, "ped-2", cliente, "nueva-cordoba", 1)
	lista, err := svc.Listar(context.Background(), cocina, "", 1)
	if err != nil || len(lista) != 1 || lista[0].ID != "ped-1" {
		t.Fatal(err, lista)
	}
}

func TestListarConDatosInvalidos(t *testing.T) {
	svc, _ := entorno(t)
	casos := []struct {
		nombre string
		actor  domain.Actor
		estado domain.Estado
		pagina int
		err    error
	}{
		{"estado que no existe", cliente, "PERDIDO", 1, domain.ErrDatosInvalidos},
		{"página cero", cliente, "", 0, domain.ErrDatosInvalidos},
		{"el admin no tiene pedidos", domain.Actor{UsuarioID: "u-admin", Rol: domain.RolAdmin}, "", 1, domain.ErrSinPermiso},
		{"cocina sin cocina asignada", domain.Actor{UsuarioID: "u-c", Rol: domain.RolCocina}, "", 1, domain.ErrSinIdentidad},
		{"sin identidad", domain.Actor{}, "", 1, domain.ErrSinIdentidad},
	}
	for _, c := range casos {
		if _, err := svc.Listar(context.Background(), c.actor, c.estado, c.pagina); !errors.Is(err, c.err) {
			t.Errorf("%s: esperaba %v, obtuvo %v", c.nombre, c.err, err)
		}
	}
}

func TestElClienteConfirmaQueLoRecibio(t *testing.T) {
	svc, _ := entorno(t)

	// 1. Antes de que salga, no puede confirmar la entrega.
	if _, err := svc.CambiarEstado(context.Background(), cliente, "ped-1", ConfirmarEntrega); !errors.Is(err, domain.ErrTransicionInvalida) {
		t.Fatal(err)
	}

	// 2. La cocina lo lleva hasta EN_CAMINO.
	for _, accion := range []Accion{Aceptar, Avanzar, Avanzar, Avanzar} {
		if _, err := svc.CambiarEstado(context.Background(), cocina, "ped-1", accion); err != nil {
			t.Fatal(accion, err)
		}
	}

	// 3. Ahora sí: queda ENTREGADO, y otro cliente no puede hacerlo por él.
	if _, err := svc.CambiarEstado(context.Background(), domain.Actor{UsuarioID: "otro", Rol: domain.RolCliente}, "ped-1", ConfirmarEntrega); !errors.Is(err, domain.ErrNoEncontrado) {
		t.Fatal(err)
	}
	p, err := svc.CambiarEstado(context.Background(), cliente, "ped-1", ConfirmarEntrega)
	if err != nil || p.Estado != domain.EstadoEntregado || len(p.Historial) != 6 {
		t.Fatal(err, p.Estado, len(p.Historial))
	}
}
