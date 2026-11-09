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
