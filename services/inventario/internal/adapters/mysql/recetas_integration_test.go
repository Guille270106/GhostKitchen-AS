package mysql_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"reflect"
	"sync"
	"testing"

	adapter "ghostkitchen/inventario/internal/adapters/mysql"
	"ghostkitchen/inventario/internal/domain"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

func TestMySQLRecetas(t *testing.T) {
	if os.Getenv("TEST_MYSQL") != "1" {
		t.Skip("requiere MySQL descartable")
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
	ejecutar := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	ejecutar(`INSERT INTO cocina_ref (id,activa) VALUES ('recetas-prueba',1),('recetas-otra',1)`)
	defer db.Exec(`DELETE FROM cocina_ref WHERE id IN ('recetas-prueba','recetas-otra')`)
	ejecutar(`INSERT INTO plato_ref (id,cocina_id,activo) VALUES ('recetas-plato','recetas-prueba',1),('recetas-ajeno','recetas-otra',1),('recetas-inactivo','recetas-prueba',0)`)
	defer db.Exec(`DELETE FROM plato_ref WHERE id LIKE 'recetas-%'`)
	repo := adapter.Nuevo(db)
	ctx := context.Background()
	ids := []string{uuid.NewString(), uuid.NewString(), uuid.NewString()}
	for j, id := range ids {
		cocina := "recetas-prueba"
		if j == 2 {
			cocina = "recetas-otra"
		}
		i, err := domain.NuevoIngrediente(id, cocina, "Prueba receta", "gramos", 10)
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.Crear(ctx, i); err != nil {
			t.Fatal(err)
		}
	}
	defer db.Exec(`DELETE FROM ingrediente WHERE cocina_id IN ('recetas-prueba','recetas-otra')`)
	defer db.Exec(`DELETE FROM receta WHERE cocina_id IN ('recetas-prueba','recetas-otra')`)
	defer db.Exec(`DELETE FROM receta_item WHERE cocina_id IN ('recetas-prueba','recetas-otra')`)
	nueva := func(plato, cocina string, items ...domain.RecetaItem) domain.Receta {
		t.Helper()
		r, err := domain.NuevaReceta(plato, cocina, items)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	original := nueva("recetas-plato", "recetas-prueba", domain.RecetaItem{IngredienteID: ids[0], Cantidad: 2}, domain.RecetaItem{IngredienteID: ids[1], Cantidad: 3})
	guardada, err := repo.GuardarReceta(ctx, original)
	if err != nil || guardada.Version != 1 {
		t.Fatal(guardada, err)
	}
	leida, err := repo.ObtenerReceta(ctx, "recetas-prueba", "recetas-plato")
	if err != nil || !reflect.DeepEqual(leida, guardada) {
		t.Fatal(leida, err)
	}
	for _, ingrediente := range []string{ids[2], uuid.NewString()} {
		invalida := nueva("recetas-plato", "recetas-prueba", domain.RecetaItem{IngredienteID: ids[0], Cantidad: 9}, domain.RecetaItem{IngredienteID: ingrediente, Cantidad: 1})
		if _, err := repo.GuardarReceta(ctx, invalida); !errors.Is(err, domain.ErrNoEncontrado) {
			t.Fatal("aceptó ingrediente ajeno o desconocido", err)
		}
		trasFallo, err := repo.ObtenerReceta(ctx, "recetas-prueba", "recetas-plato")
		if err != nil || !reflect.DeepEqual(trasFallo, guardada) {
			t.Fatal("no conservó receta y versión", trasFallo, err)
		}
	}
	for _, plato := range []string{"recetas-ajeno", "recetas-inactivo", "no-existe"} {
		if _, err := repo.GuardarReceta(ctx, nueva(plato, "recetas-prueba", domain.RecetaItem{IngredienteID: ids[0], Cantidad: 1})); !errors.Is(err, domain.ErrNoEncontrado) {
			t.Fatal("aceptó plato ajeno/inactivo/desconocido", err)
		}
	}
	if _, err := repo.ObtenerReceta(ctx, "recetas-otra", "recetas-plato"); !errors.Is(err, domain.ErrNoEncontrado) {
		t.Fatal("lectura ajena", err)
	}
	lista, err := repo.ListarRecetas(ctx, "recetas-prueba")
	if err != nil || len(lista) != 1 {
		t.Fatal(lista, err)
	}
	ajenas, err := repo.ListarRecetas(ctx, "recetas-otra")
	if err != nil || len(ajenas) != 0 {
		t.Fatal(ajenas, err)
	}
	// Las claves compuestas impiden cruzar cocinas incluso saltándose el adaptador HTTP.
	if _, err := db.Exec(`INSERT INTO receta_item (plato_id,cocina_id,ingrediente_id,cantidad) VALUES ('recetas-plato','recetas-prueba',?,1)`, ids[2]); err == nil {
		t.Fatal("faltó FK de cocina")
	}
	if _, err := db.Exec(`UPDATE receta_item SET cantidad=0 WHERE plato_id='recetas-plato'`); err == nil {
		t.Fatal("faltó CHECK de cantidad")
	}
	reemplazo := nueva("recetas-plato", "recetas-prueba", domain.RecetaItem{IngredienteID: ids[0], Cantidad: 8})
	guardada, err = repo.GuardarReceta(ctx, reemplazo)
	if err != nil || guardada.Version != 2 || len(guardada.Items) != 1 {
		t.Fatal(guardada, err)
	}
	// Dos ediciones concurrentes dejan una receta completa y versiones consecutivas.
	var wg sync.WaitGroup
	resultados := make(chan error, 2)
	for _, receta := range []domain.Receta{original, reemplazo} {
		wg.Add(1)
		go func(receta domain.Receta) {
			defer wg.Done()
			_, err := repo.GuardarReceta(ctx, receta)
			resultados <- err
		}(receta)
	}
	wg.Wait()
	close(resultados)
	for err := range resultados {
		if err != nil {
			t.Fatal(err)
		}
	}
	final, err := repo.ObtenerReceta(ctx, "recetas-prueba", "recetas-plato")
	if err != nil || final.Version != 4 || (!reflect.DeepEqual(final.Items, original.Items) && !reflect.DeepEqual(final.Items, reemplazo.Items)) {
		t.Fatal("receta parcial", final, err)
	}
	ejecutar(`UPDATE cocina_ref SET activa=0 WHERE id='recetas-prueba'`)
	if _, err := repo.GuardarReceta(ctx, original); !errors.Is(err, domain.ErrNoEncontrado) {
		t.Fatal("aceptó cocina inactiva", err)
	}
}
