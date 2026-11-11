package mysql_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	adapter "ghostkitchen/pedidos/internal/adapters/mysql"
	"ghostkitchen/pedidos/internal/application"
	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/ports"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

func conectar(t *testing.T) *sql.DB {
	t.Helper()
	if os.Getenv("TEST_MYSQL") != "1" {
		t.Skip("requiere la base descartable de testdata/compose.yml")
	}
	cfg := driver.NewConfig()
	cfg.Net = "tcp"
	cfg.Addr = "mysql:3306"
	cfg.User = "pedidos"
	cfg.Passwd = "test-pedidos"
	cfg.DBName = "pedidos_db"
	cfg.ParseTime = true
	connector, err := driver.NewConnector(cfg)
	if err != nil {
		t.Fatal(err)
	}
	db := sql.OpenDB(connector)
	t.Cleanup(func() { db.Close() })
	return db
}

func crearPedido(t *testing.T, db *sql.DB, repo adapter.PedidosMySQL) domain.Pedido {
	t.Helper()
	cliente := domain.Actor{UsuarioID: "u-" + uuid.NewString()[:8], Rol: domain.RolCliente}
	p, err := domain.NuevoPedido(uuid.NewString(), cliente, "cliente@ejemplo.com", "cerro", []domain.Item{
		{PlatoID: "p17", Nombre: "Burger ' doble", RestauranteID: "r5", RestauranteNombre: "Johnny B. Good", PrecioUnitario: 12000, Cantidad: 2},
		{PlatoID: "p21", Nombre: "Pizza", RestauranteID: "r6", RestauranteNombre: "Il Gatto", PrecioUnitario: 9500, Cantidad: 1},
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Crear(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM pedido_historial WHERE pedido_id=?", p.ID)
		db.Exec("DELETE FROM pedido_item WHERE pedido_id=?", p.ID)
		db.Exec("DELETE FROM pedido WHERE id=?", p.ID)
	})
	return p
}

func TestMySQLCrearYObtener(t *testing.T) {
	db := conectar(t)
	repo := adapter.PedidosMySQL{DB: db}
	if err := repo.Ready(context.Background()); err != nil {
		t.Fatal("las migraciones no están aplicadas:", err)
	}
	p := crearPedido(t, db, repo)
	leido, err := repo.Obtener(context.Background(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if leido.Estado != domain.EstadoRecibido || leido.Total != 33500 || leido.Version != 1 ||
		len(leido.Items) != 2 || leido.Items[0].Nombre != "Burger ' doble" || len(leido.Historial) != 1 {
		t.Fatalf("%+v", leido)
	}
	if _, err := repo.Obtener(context.Background(), "no-existe"); !errors.Is(err, domain.ErrNoEncontrado) {
		t.Fatal(err)
	}
}

// Cancelar y aceptar a la vez, contra MySQL real: la condición de versión del UPDATE deja
// pasar solo un cambio.
func TestMySQLCambiosSimultaneosGanaUnoSolo(t *testing.T) {
	db := conectar(t)
	repo := adapter.PedidosMySQL{DB: db}
	svc := application.PedidosService{Repo: repo}
	for i := 0; i < 20; i++ {
		p := crearPedido(t, db, repo)
		cliente := domain.Actor{UsuarioID: p.ClienteID, Rol: domain.RolCliente}
		cocina := domain.Actor{UsuarioID: "u-cocina", Rol: domain.RolCocina, CocinaID: "cerro"}

		var wg sync.WaitGroup
		errs := make([]error, 2)
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, errs[0] = svc.CambiarEstado(context.Background(), cliente, p.ID, application.Cancelar)
		}()
		go func() {
			defer wg.Done()
			_, errs[1] = svc.CambiarEstado(context.Background(), cocina, p.ID, application.Aceptar)
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
		final, err := repo.Obtener(context.Background(), p.ID)
		if err != nil {
			t.Fatal(err)
		}
		if exitos != 1 || final.Version != 2 || len(final.Historial) != 2 {
			t.Fatalf("vuelta %d: ganaron %d, versión %d, historial %d", i, exitos, final.Version, len(final.Historial))
		}
	}
}

// Mis pedidos contra MySQL real: solo los del cliente, del más nuevo al más viejo, con sus
// platos, filtrando por estado y de a una página.
func TestMySQLListar(t *testing.T) {
	db := conectar(t)
	repo := adapter.PedidosMySQL{DB: db}
	svc := application.PedidosService{Repo: repo}

	// 1. Tres pedidos del mismo cliente, creados uno después del otro.
	viejo := crearPedido(t, db, repo)
	cliente := domain.Actor{UsuarioID: viejo.ClienteID, Rol: domain.RolCliente}
	var ids []string
	for i := 1; i <= 2; i++ {
		p, err := domain.NuevoPedido(uuid.NewString(), cliente, "cliente@ejemplo.com", "cerro",
			[]domain.Item{{PlatoID: "p17", Nombre: "Burger", RestauranteID: "r5", PrecioUnitario: 12000, Cantidad: 1}},
			time.Now().Add(time.Duration(i)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.Crear(context.Background(), p); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, p.ID)
		t.Cleanup(func() {
			db.Exec("DELETE FROM pedido_historial WHERE pedido_id=?", p.ID)
			db.Exec("DELETE FROM pedido_item WHERE pedido_id=?", p.ID)
			db.Exec("DELETE FROM pedido WHERE id=?", p.ID)
		})
	}

	// 2. Todos: el más nuevo primero, y cada uno con sus platos.
	lista, err := repo.Listar(context.Background(), ports.FiltroPedidos{ClienteID: cliente.UsuarioID, Pagina: 1, PorPagina: 20})
	if err != nil || len(lista) != 3 || lista[0].ID != ids[1] || lista[2].ID != viejo.ID || len(lista[2].Items) != 2 {
		t.Fatal(err, lista)
	}

	// 3. De a 2 por página: la segunda página trae el más viejo.
	lista, err = repo.Listar(context.Background(), ports.FiltroPedidos{ClienteID: cliente.UsuarioID, Pagina: 2, PorPagina: 2})
	if err != nil || len(lista) != 1 || lista[0].ID != viejo.ID {
		t.Fatal(err, lista)
	}

	// 4. Por estado: cancelamos el más viejo y filtramos.
	if _, err := svc.CambiarEstado(context.Background(), cliente, viejo.ID, application.Cancelar); err != nil {
		t.Fatal(err)
	}
	lista, err = svc.Listar(context.Background(), cliente, domain.EstadoCancelado, 1)
	if err != nil || len(lista) != 1 || lista[0].ID != viejo.ID {
		t.Fatal(err, lista)
	}
}

// El envío cobrado se guarda con el pedido. Un pedido sin envío registrado (los anteriores a la
// migración 000010) se lee con el envío en nil: no se inventa.
func TestMySQLElPedidoGuardaSuEnvio(t *testing.T) {
	db := conectar(t)
	repo := adapter.PedidosMySQL{DB: db}
	ctx := context.Background()

	conEnvio := crearPedidoConEnvio(t, db, repo, ptr(1300))
	leido, err := repo.Obtener(ctx, conEnvio.ID)
	if err != nil || leido.CostoEnvio == nil || *leido.CostoEnvio != 1300 || *leido.TotalConEnvio != 33500+1300 {
		t.Fatalf("envío guardado: %+v %v", leido, err)
	}

	sinEnvio := crearPedidoConEnvio(t, db, repo, nil)
	viejo, err := repo.Obtener(ctx, sinEnvio.ID)
	if err != nil || viejo.CostoEnvio != nil || viejo.TotalConEnvio != nil || viejo.Total != 33500 {
		t.Fatalf("sin envío registrado: %+v %v", viejo, err)
	}

	lista, err := repo.Listar(ctx, ports.FiltroPedidos{ClienteID: conEnvio.ClienteID, Pagina: 1, PorPagina: 20})
	if err != nil || len(lista) != 1 || lista[0].CostoEnvio == nil || *lista[0].CostoEnvio != 1300 {
		t.Fatalf("la lista también trae el envío: %+v %v", lista, err)
	}
}

func ptr(v int64) *int64 { return &v }

func crearPedidoConEnvio(t *testing.T, db *sql.DB, repo adapter.PedidosMySQL, envio *int64) domain.Pedido {
	t.Helper()
	cliente := domain.Actor{UsuarioID: "u-" + uuid.NewString()[:8], Rol: domain.RolCliente}
	p, err := domain.NuevoPedido(uuid.NewString(), cliente, "cliente@ejemplo.com", "cerro", []domain.Item{
		{PlatoID: "p17", Nombre: "Burger", RestauranteID: "r5", RestauranteNombre: "Johnny B. Good", PrecioUnitario: 12000, Cantidad: 2},
		{PlatoID: "p21", Nombre: "Pizza", RestauranteID: "r6", RestauranteNombre: "Il Gatto", PrecioUnitario: 9500, Cantidad: 1},
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	p.FijarEnvio(envio)
	if err := repo.Crear(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM outbox WHERE agregado_id=?", p.ID)
		db.Exec("DELETE FROM pedido_historial WHERE pedido_id=?", p.ID)
		db.Exec("DELETE FROM pedido_item WHERE pedido_id=?", p.ID)
		db.Exec("DELETE FROM pedido WHERE id=?", p.ID)
	})
	return p
}

// La dirección de entrega se guarda con el pedido y la ven el detalle y la lista de la cocina.
// Un pedido sin dirección (los anteriores a la migración 000012) se lee con la dirección vacía.
func TestMySQLElPedidoGuardaSuDireccion(t *testing.T) {
	db := conectar(t)
	repo := adapter.PedidosMySQL{DB: db}
	cliente := domain.Actor{UsuarioID: "u-" + uuid.NewString()[:8], Rol: domain.RolCliente}
	cocina := "dir-" + uuid.NewString()[:8]
	crear := func(direccion string) domain.Pedido {
		t.Helper()
		p, err := domain.NuevoPedido(uuid.NewString(), cliente, "cliente@ejemplo.com", cocina,
			[]domain.Item{{PlatoID: "p17", Nombre: "Burger", RestauranteID: "r5", PrecioUnitario: 12000, Cantidad: 1}}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		p.DireccionEntrega = direccion
		if err := repo.Crear(context.Background(), p); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			db.Exec("DELETE FROM pedido_historial WHERE pedido_id=?", p.ID)
			db.Exec("DELETE FROM pedido_item WHERE pedido_id=?", p.ID)
			db.Exec("DELETE FROM pedido WHERE id=?", p.ID)
		})
		return p
	}
	con := crear("Obispo Trejo 242, 3°B — timbre \"Pérez\"")
	sin := crear("")

	if leido, err := repo.Obtener(context.Background(), con.ID); err != nil || leido.DireccionEntrega != con.DireccionEntrega {
		t.Fatalf("detalle: %q, %v", leido.DireccionEntrega, err)
	}
	if leido, err := repo.Obtener(context.Background(), sin.ID); err != nil || leido.DireccionEntrega != "" {
		t.Fatalf("sin dirección: %q, %v", leido.DireccionEntrega, err)
	}
	var nulo sql.NullString
	if err := db.QueryRow("SELECT direccion_entrega FROM pedido WHERE id=?", sin.ID).Scan(&nulo); err != nil || nulo.Valid {
		t.Fatal("sin dirección se guarda NULL, no texto vacío:", nulo, err)
	}
	lista, err := repo.Listar(context.Background(), ports.FiltroPedidos{CocinaID: cocina, Pagina: 1, PorPagina: 20})
	if err != nil || len(lista) != 2 {
		t.Fatal(err, lista)
	}
	for _, p := range lista {
		if quiero := map[string]string{con.ID: con.DireccionEntrega, sin.ID: ""}[p.ID]; p.DireccionEntrega != quiero {
			t.Fatalf("lista de la cocina: %q, esperaba %q", p.DireccionEntrega, quiero)
		}
	}
}
