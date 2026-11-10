package mysql_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	adapter "ghostkitchen/inventario/internal/adapters/mysql"
	"ghostkitchen/inventario/internal/domain"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

func TestMySQLIngredientes(t *testing.T) {
	if os.Getenv("TEST_MYSQL") != "1" {
		t.Skip("requiere la base descartable de testdata/compose.yml")
	}
	cfg := driver.NewConfig()
	cfg.Net = "tcp"
	cfg.Addr = "mysql:3306"
	cfg.User = "inventario"
	cfg.Passwd = "test-inventario"
	cfg.DBName = "inventario_db"
	connector, err := driver.NewConnector(cfg)
	if err != nil {
		t.Fatal(err)
	}
	db := sql.OpenDB(connector)
	defer db.Close()
	repo := adapter.Nuevo(db)
	ctx := context.Background()
	if err := repo.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	i, err := domain.NuevoIngrediente(uuid.NewString(), "cerro", "Sal ' de prueba", "gramos", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Crear(ctx, i); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DELETE FROM ingrediente WHERE id=?", i.ID)
	defer db.Exec("DELETE FROM movimiento_stock WHERE ingrediente_id=?", i.ID)
	obtenido, err := repo.Obtener(ctx, "cerro", i.ID)
	if err != nil || obtenido != i {
		t.Fatalf("persistencia: %+v, %v", obtenido, err)
	}
	if _, err := repo.Obtener(ctx, "villa-allende", i.ID); !errors.Is(err, domain.ErrNoEncontrado) {
		t.Fatal("acceso cruzado", err)
	}
	// La base trae los ingredientes de ejemplo (migración 000009): se busca el creado acá y se
	// controla que cada listado tenga solo los de su cocina.
	items, err := repo.Listar(ctx, "cerro")
	if err != nil || !contiene(items, i.ID) {
		t.Fatal(items, err)
	}
	for _, it := range items {
		if it.CocinaID != "cerro" {
			t.Fatal("listado con ingredientes de otra cocina", it)
		}
	}
	ajenos, err := repo.Listar(ctx, "villa-allende")
	if err != nil || contiene(ajenos, i.ID) {
		t.Fatal("listado cruzado", err)
	}
	for _, cocina := range []string{"desconocida", "inactiva"} {
		if cocina == "inactiva" {
			if _, err := db.Exec("INSERT INTO cocina_ref (id,activa) VALUES ('inactiva',0)"); err != nil {
				t.Fatal(err)
			}
			defer db.Exec("DELETE FROM cocina_ref WHERE id='inactiva'")
		}
		otro := i
		otro.ID = uuid.NewString()
		otro.CocinaID = cocina
		if err := repo.Crear(ctx, otro); !errors.Is(err, domain.ErrNoEncontrado) {
			t.Fatal("permitió cocina no válida", err)
		}
	}
	for _, query := range []string{"UPDATE ingrediente SET total=-1 WHERE id=?", "UPDATE ingrediente SET reservado=1 WHERE id=?", "UPDATE ingrediente SET reservado=-1 WHERE id=?"} {
		if _, err := db.Exec(query, i.ID); err == nil {
			t.Fatal("faltó restricción de stock", query)
		}
	}
	if _, err := repo.MoverStock(ctx, "villa-allende", i.ID, "cocinera", 10); !errors.Is(err, domain.ErrNoEncontrado) {
		t.Fatal("movimiento en cocina ajena", err)
	}
	repuesto, err := repo.MoverStock(ctx, "cerro", i.ID, "cocinera", 10)
	if err != nil || repuesto.Total != 10 || repuesto.Disponible != 10 {
		t.Fatal(repuesto, err)
	}
	if _, err := db.Exec("UPDATE ingrediente SET reservado=6 WHERE id=?", i.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MoverStock(ctx, "cerro", i.ID, "cocinera", -5); !errors.Is(err, domain.ErrSinStock) {
		t.Fatal("merma sobre stock reservado", err)
	}
	mermado, err := repo.MoverStock(ctx, "cerro", i.ID, "cocinera", -4)
	if err != nil || mermado.Total != 6 || mermado.Reservado != 6 || mermado.Disponible != 0 {
		t.Fatal(mermado, err)
	}
	var movimientos, neto int
	err = db.QueryRow("SELECT COUNT(*),SUM(delta) FROM movimiento_stock WHERE ingrediente_id=?", i.ID).Scan(&movimientos, &neto)
	if err != nil || movimientos != 2 || neto != 6 {
		t.Fatal(movimientos, neto, err)
	}
}

func contiene(items []domain.Ingrediente, id string) bool {
	for _, it := range items {
		if it.ID == id {
			return true
		}
	}
	return false
}
