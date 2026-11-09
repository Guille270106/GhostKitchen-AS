package mysql_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	adapter "ghostkitchen/inventario/internal/adapters/mysql"
	"ghostkitchen/inventario/internal/application"
	"ghostkitchen/inventario/internal/domain"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

type baseReservas struct {
	db                   *sql.DB
	repo                 *adapter.Repositorio
	svc                  *application.Reservas
	prefijo, cocina      string
	platos, ingredientes []string
}

func prepararReservas(t *testing.T) *baseReservas {
	t.Helper()
	if os.Getenv("TEST_MYSQL") != "1" {
		t.Skip("requiere MySQL descartable")
	}
	cfg := driver.NewConfig()
	cfg.Net = "tcp"
	cfg.Addr = "mysql:3306"
	cfg.User = "inventario"
	cfg.Passwd = "test-inventario"
	cfg.DBName = "inventario_db"
	cfg.ParseTime = true
	connector, err := driver.NewConnector(cfg)
	if err != nil {
		t.Fatal(err)
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(12)
	f := &baseReservas{db: db, prefijo: uuid.NewString()}
	f.cocina = f.prefijo
	f.repo = adapter.Nuevo(db)
	f.svc = application.NuevasReservas(f.repo)
	f.platos = []string{f.prefijo + "-p1", f.prefijo + "-p2"}
	f.ingredientes = []string{uuid.NewString(), uuid.NewString()}
	sort.Strings(f.ingredientes)
	t.Cleanup(func() {
		db.Exec(`DELETE c FROM consumo c JOIN reserva r ON r.id=c.reserva_id WHERE r.cocina_id=?`, f.cocina)
		db.Exec(`DELETE FROM outbox WHERE JSON_UNQUOTE(JSON_EXTRACT(cuerpo, '$.datos.cocina_id'))=?`, f.cocina)
		for _, tabla := range []string{"reserva_item", "reserva_plato", "reserva", "receta_item", "receta", "plato_ref", "ingrediente"} {
			db.Exec("DELETE FROM "+tabla+" WHERE cocina_id=?", f.cocina)
		}
		db.Exec("DELETE FROM idempotencia WHERE clave LIKE ?", f.prefijo+"%")
		db.Exec("DELETE FROM cocina_ref WHERE id=?", f.cocina)
		db.Close()
	})
	if _, err := db.Exec("INSERT INTO cocina_ref (id,activa) VALUES (?,1)", f.cocina); err != nil {
		t.Fatal(err)
	}
	for _, plato := range f.platos {
		if _, err := db.Exec("INSERT INTO plato_ref (id,cocina_id,activo) VALUES (?,?,1)", plato, f.cocina); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range f.ingredientes {
		i, _ := domain.NuevoIngrediente(id, f.cocina, "Reserva prueba", "unidad", 20)
		if err := f.repo.Crear(context.Background(), i); err != nil {
			t.Fatal(err)
		}
	}
	f.receta(t, f.platos[0], domain.RecetaItem{IngredienteID: f.ingredientes[0], Cantidad: 2}, domain.RecetaItem{IngredienteID: f.ingredientes[1], Cantidad: 1})
	f.receta(t, f.platos[1], domain.RecetaItem{IngredienteID: f.ingredientes[0], Cantidad: 3})
	return f
}
func (f *baseReservas) receta(t *testing.T, plato string, items ...domain.RecetaItem) {
	t.Helper()
	r, err := domain.NuevaReceta(plato, f.cocina, items)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.repo.GuardarReceta(context.Background(), r); err != nil {
		t.Fatal(err)
	}
}
func (f *baseReservas) solicitud(carrito string, items ...domain.PlatoSolicitado) domain.SolicitudReserva {
	return domain.SolicitudReserva{CarritoID: f.prefijo + carrito, CocinaID: f.cocina, Items: items}
}
func (f *baseReservas) stock(t *testing.T, id string) int64 {
	t.Helper()
	var n int64
	if err := f.db.QueryRow("SELECT reservado FROM ingrediente WHERE id=?", id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func jsonReserva(t *testing.T, res domain.Reserva) string {
	t.Helper()
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestMySQLReservaIdempotenteYCarritoUnico(t *testing.T) {
	f := prepararReservas(t)
	ctx := context.Background()
	datos := f.solicitud("cart", domain.PlatoSolicitado{PlatoID: f.platos[1], Cantidad: 1}, domain.PlatoSolicitado{PlatoID: f.platos[0], Cantidad: 2})
	primera, err := f.svc.Crear(ctx, f.prefijo+"k", datos)
	if err != nil {
		t.Fatal(err)
	}
	// El orden del JSON no cambia el significado ni la huella.
	datos.Items[0], datos.Items[1] = datos.Items[1], datos.Items[0]
	repetida, err := f.svc.Crear(ctx, f.prefijo+"k", datos)
	if err != nil || jsonReserva(t, primera) != jsonReserva(t, repetida) {
		t.Fatal("replay", err, primera, repetida)
	}
	if f.stock(t, f.ingredientes[0]) != 7 || f.stock(t, f.ingredientes[1]) != 2 {
		t.Fatal("stock duplicado")
	}
	var segundos int
	if err := f.db.QueryRow("SELECT TIMESTAMPDIFF(SECOND,creada_en,vence_en) FROM reserva WHERE id=?", primera.ID).Scan(&segundos); err != nil || segundos != 600 {
		t.Fatal(segundos, err)
	}
	datos.Items[0].Cantidad++
	if _, err := f.svc.Crear(ctx, f.prefijo+"k", datos); !errors.Is(err, domain.ErrIdempotencia) {
		t.Fatal("clave con otros datos", err)
	}
	if _, err := f.svc.Crear(ctx, f.prefijo+"otra", datos); !errors.Is(err, domain.ErrReservaConflicto) {
		t.Fatal("dos reservas del carrito", err)
	}
	if _, err := f.svc.Liberar(ctx, f.prefijo+"liberar", primera.ID); err != nil {
		t.Fatal(err)
	}
	// La repetición devuelve el éxito original, no una nueva reserva ni la lectura actual.
	replay, err := f.svc.Crear(ctx, f.prefijo+"k", f.solicitud("cart", domain.PlatoSolicitado{PlatoID: f.platos[0], Cantidad: 2}, domain.PlatoSolicitado{PlatoID: f.platos[1], Cantidad: 1}))
	if err != nil || replay.ID != primera.ID || replay.Estado != "ACTIVA" || f.stock(t, f.ingredientes[0]) != 0 {
		t.Fatal(replay, err)
	}
}

func TestMySQLReservaTodoONadaYErrorDurable(t *testing.T) {
	f := prepararReservas(t)
	ctx := context.Background()
	// El ingrediente que se procesa último no alcanza, después de tocar el primero.
	if _, err := f.db.Exec("UPDATE ingrediente SET total=0 WHERE id=?", f.ingredientes[1]); err != nil {
		t.Fatal(err)
	}
	datos := f.solicitud("fallo", domain.PlatoSolicitado{PlatoID: f.platos[0], Cantidad: 1})
	if _, err := f.svc.Crear(ctx, f.prefijo+"sin-stock", datos); !errors.Is(err, domain.ErrSinStock) {
		t.Fatal(err)
	}
	if f.stock(t, f.ingredientes[0]) != 0 {
		t.Fatal("reserva parcial")
	}
	var n int
	if err := f.db.QueryRow("SELECT COUNT(*) FROM reserva WHERE carrito_id=?", datos.CarritoID).Scan(&n); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	f.db.Exec("UPDATE ingrediente SET total=20 WHERE id=?", f.ingredientes[1])
	// El rechazo quedó guardado: un nuevo intento lógico necesita una nueva clave.
	if _, err := f.svc.Crear(ctx, f.prefijo+"sin-stock", datos); !errors.Is(err, domain.ErrSinStock) {
		t.Fatal("rechazo no durable", err)
	}
	res, err := f.svc.Crear(ctx, f.prefijo+"nuevo", datos)
	if err != nil {
		t.Fatal(err)
	}
	antes := jsonReserva(t, res)
	if _, err := f.svc.Ajustar(ctx, f.prefijo+"ajuste", res.ID, []domain.PlatoSolicitado{{PlatoID: f.platos[0], Cantidad: 11}}); !errors.Is(err, domain.ErrSinStock) {
		t.Fatal(err)
	}
	despues, err := f.svc.Obtener(ctx, res.ID)
	if err != nil || jsonReserva(t, despues) != antes || f.stock(t, f.ingredientes[0]) != 2 {
		t.Fatal("ajuste parcial", despues, err)
	}
	// No hay claves EN_PROCESO abandonadas ni marcas de éxito parciales.
	if err := f.db.QueryRow("SELECT COUNT(*) FROM idempotencia WHERE clave LIKE ? AND resultado IS NULL", f.prefijo+"%").Scan(&n); err != nil || n != 0 {
		t.Fatal(n, err)
	}
}

func TestMySQLAjusteConservaRecetasYVencimiento(t *testing.T) {
	f := prepararReservas(t)
	ctx := context.Background()
	res, err := f.svc.Crear(ctx, f.prefijo+"inicio", f.solicitud("cart", domain.PlatoSolicitado{PlatoID: f.platos[0], Cantidad: 2}))
	if err != nil {
		t.Fatal(err)
	}
	f.receta(t, f.platos[0], domain.RecetaItem{IngredienteID: f.ingredientes[0], Cantidad: 5}, domain.RecetaItem{IngredienteID: f.ingredientes[1], Cantidad: 3})
	aumento, err := f.svc.Ajustar(ctx, f.prefijo+"subir", res.ID, []domain.PlatoSolicitado{{PlatoID: f.platos[0], Cantidad: 3}})
	if err != nil || !aumento.VenceEn.Equal(res.VenceEn) || f.stock(t, f.ingredientes[0]) != 9 || f.stock(t, f.ingredientes[1]) != 5 {
		t.Fatal(aumento, err)
	}
	bajada, err := f.svc.Ajustar(ctx, f.prefijo+"bajar", res.ID, []domain.PlatoSolicitado{{PlatoID: f.platos[0], Cantidad: 1}})
	if err != nil || !bajada.VenceEn.Equal(res.VenceEn) || f.stock(t, f.ingredientes[0]) != 2 || f.stock(t, f.ingredientes[1]) != 1 {
		t.Fatal(bajada, err)
	}
	liberada, err := f.svc.Liberar(ctx, f.prefijo+"liberar", res.ID)
	if err != nil || liberada.Estado != "LIBERADA" || len(liberada.Items) != 0 || f.stock(t, f.ingredientes[0]) != 0 || f.stock(t, f.ingredientes[1]) != 0 {
		t.Fatal(liberada, err)
	}
	if _, err := f.svc.Liberar(ctx, f.prefijo+"otra-liberacion", res.ID); err != nil {
		t.Fatal("doble liberación", err)
	}
	if _, err := f.svc.Ajustar(ctx, f.prefijo+"reactivar", res.ID, []domain.PlatoSolicitado{{PlatoID: f.platos[0], Cantidad: 1}}); !errors.Is(err, domain.ErrReservaConflicto) {
		t.Fatal("reactivó reserva", err)
	}
}

func TestMySQLReservaVencidaYReferencias(t *testing.T) {
	f := prepararReservas(t)
	ctx := context.Background()
	res, err := f.svc.Crear(ctx, f.prefijo+"inicio", f.solicitud("cart", domain.PlatoSolicitado{PlatoID: f.platos[0], Cantidad: 1}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec("UPDATE reserva SET vence_en=DATE_SUB(UTC_TIMESTAMP(6),INTERVAL 1 SECOND) WHERE id=?", res.ID); err != nil {
		t.Fatal(err)
	}
	leida, err := f.svc.Obtener(ctx, res.ID)
	if err != nil || leida.Estado != "VENCIDA" {
		t.Fatal(leida, err)
	}
	if _, err := f.svc.Ajustar(ctx, f.prefijo+"tarde", res.ID, []domain.PlatoSolicitado{{PlatoID: f.platos[0], Cantidad: 2}}); !errors.Is(err, domain.ErrReservaVencida) {
		t.Fatal(err)
	}
	if _, err := f.svc.Liberar(ctx, f.prefijo+"liberar", res.ID); err != nil || f.stock(t, f.ingredientes[0]) != 0 {
		t.Fatal("liberar vencida", err)
	}
	var estado string
	if err := f.db.QueryRow("SELECT estado FROM reserva WHERE id=?", res.ID).Scan(&estado); err != nil || estado != "VENCIDA" {
		t.Fatalf("estado persistido = %q, error = %v", estado, err)
	}
	var cantidad int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM outbox WHERE tipo='reserva.vencida'
 AND JSON_UNQUOTE(JSON_EXTRACT(cuerpo, '$.datos.reserva_id'))=?`, res.ID).Scan(&cantidad); err != nil || cantidad != 1 {
		t.Fatalf("eventos de vencimiento = %d, error = %v", cantidad, err)
	}
	// Plato ajeno y plato sin receta no pueden producir una reserva parcial.
	for j, plato := range []string{"p1", f.prefijo + "-sin-receta"} {
		if j == 1 {
			f.db.Exec("INSERT INTO plato_ref (id,cocina_id,activo) VALUES (?,?,1)", plato, f.cocina)
		}
		if _, err := f.svc.Crear(ctx, f.prefijo+plato, f.solicitud("ref-"+string(rune('a'+j)), domain.PlatoSolicitado{PlatoID: plato, Cantidad: 1})); !errors.Is(err, domain.ErrNoEncontrado) {
			t.Fatal("referencia inválida", err)
		}
	}
}

func TestMySQLReservasConcurrentes(t *testing.T) {
	f := prepararReservas(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	f.db.Exec("UPDATE ingrediente SET total=2 WHERE id=?", f.ingredientes[0])
	var wg sync.WaitGroup
	resultados := make(chan error, 8)
	for j := 0; j < 8; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.svc.Crear(ctx, f.prefijo+"duplicada", f.solicitud("cart", domain.PlatoSolicitado{PlatoID: f.platos[0], Cantidad: 1}))
			resultados <- err
		}()
	}
	wg.Wait()
	close(resultados)
	for err := range resultados {
		if err != nil {
			t.Fatal("duplicados simultáneos", err)
		}
	}
	if f.stock(t, f.ingredientes[0]) != 2 {
		t.Fatal("duplicó stock")
	}
	// Otra clave y otro carrito no consiguen el stock ya apartado.
	if _, err := f.svc.Crear(ctx, f.prefijo+"competidor", f.solicitud("otro", domain.PlatoSolicitado{PlatoID: f.platos[0], Cantidad: 1})); !errors.Is(err, domain.ErrSinStock) {
		t.Fatal(err)
	}
}

func TestMySQLConsumoYDevolucionIdempotentes(t *testing.T) {
	f := prepararReservas(t)
	ctx := context.Background()
	svc := application.NuevosConsumos(f.repo)
	pedido := f.prefijo + "-pedido"
	item := domain.PlatoSolicitado{PlatoID: f.platos[0], Cantidad: 1}
	res, err := f.svc.Crear(ctx, f.prefijo+"reserva", f.solicitud("cart", item))
	if err != nil {
		t.Fatal(err)
	}
	ingrediente := f.ingredientes[0]
	f.receta(t, f.platos[0], domain.RecetaItem{IngredienteID: ingrediente, Cantidad: 9})
	verStock := func(esperado int64) {
		t.Helper()
		var total, reservado int64
		err := f.db.QueryRow("SELECT total,reservado FROM ingrediente WHERE id=?", ingrediente).Scan(&total, &reservado)
		if err != nil || total != esperado || reservado != 0 {
			t.Fatalf("stock = %d, reservado = %d, error = %v", total, reservado, err)
		}
	}
	for _, sufijo := range []string{"consumo", "consumo", "otro-consumo"} {
		c, err := svc.Consumir(ctx, f.prefijo+sufijo, res.ID, pedido)
		if err != nil || c.Estado != "CONSUMIDO" {
			t.Fatalf("consumo = %+v, error = %v", c, err)
		}
		verStock(18)
	}
	for _, sufijo := range []string{"devolucion", "devolucion", "otra-devolucion"} {
		c, err := svc.Devolver(ctx, f.prefijo+sufijo, pedido)
		if err != nil || c.Estado != "DEVUELTO" {
			t.Fatalf("devolución = %+v, error = %v", c, err)
		}
		verStock(20)
	}
	c, err := svc.Obtener(ctx, pedido)
	if err != nil || c.Estado != "DEVUELTO" || c.ReservaID != res.ID {
		t.Fatalf("consulta = %+v, error = %v", c, err)
	}
}

func TestMySQLReferenciasYDisponibilidad(t *testing.T) {
	f := prepararReservas(t)
	ctx := context.Background()
	plato := f.platos[0]
	ref := domain.Referencia{EventoID: uuid.NewString(), ID: plato, CocinaID: f.cocina, Version: 2, Activa: false}
	t.Cleanup(func() {
		f.db.Exec("DELETE FROM mensaje_procesado WHERE evento_id=?", ref.EventoID)
	})
	if err := f.repo.AplicarReferencia(ctx, ref); err != nil {
		t.Fatal(err)
	}
	var secuencia int
	var disponible, receta bool
	estado := func() {
		t.Helper()
		if err := f.db.QueryRow("SELECT secuencia,disponible,tiene_receta FROM disponibilidad_estado WHERE plato_id=?", plato).Scan(&secuencia, &disponible, &receta); err != nil {
			t.Fatal(err)
		}
	}
	estado()
	if disponible || !receta {
		t.Fatal("el plato inactivo debe conservar la receta y quedar no disponible")
	}
	anterior := secuencia
	viejo := ref
	viejo.EventoID, viejo.Version, viejo.Activa = uuid.NewString(), 1, true
	t.Cleanup(func() { f.db.Exec("DELETE FROM mensaje_procesado WHERE evento_id=?", viejo.EventoID) })
	for _, evento := range []domain.Referencia{ref, viejo} {
		if err := f.repo.AplicarReferencia(ctx, evento); err != nil {
			t.Fatal(err)
		}
	}
	estado()
	if disponible || secuencia != anterior {
		t.Fatal("el duplicado o evento viejo cambió el estado")
	}
	nuevo := viejo
	nuevo.EventoID, nuevo.Version = uuid.NewString(), 3
	t.Cleanup(func() { f.db.Exec("DELETE FROM mensaje_procesado WHERE evento_id=?", nuevo.EventoID) })
	if err := f.repo.AplicarReferencia(ctx, nuevo); err != nil {
		t.Fatal(err)
	}
	estado()
	if !disponible || secuencia != anterior+1 {
		t.Fatal("faltó publicar la reactivación")
	}
	var n int
	err := f.db.QueryRow(`SELECT COUNT(*) FROM outbox WHERE tipo='disponibilidad.cambiada'
 AND JSON_UNQUOTE(JSON_EXTRACT(cuerpo,'$.agregado_id'))=?`, plato).Scan(&n)
	if err != nil || n != secuencia {
		t.Fatal(n, secuencia, err)
	}
}
