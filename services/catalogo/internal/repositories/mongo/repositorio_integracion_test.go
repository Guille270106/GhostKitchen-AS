package mongo

// Test de integración contra un MongoDB real (lo pide el enunciado: "tests de integración por
// servicio contra una dependencia real"). Se saltea si no hay MONGO_TEST_URI.
//
// Con docker compose levantado:
//   docker run --rm --network ghostkitchen-as_default -v ${PWD}:/src -w /src \
//     -e MONGO_TEST_URI="mongodb://root:<MONGO_ROOT_PASSWORD>@mongo:27017" golang:1.24-alpine \
//     go test ./internal/repositories/mongo/ -run Integracion -v

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"ghostkitchen/catalogo/internal/domain"
)

func TestIntegracionRepositorioMongo(t *testing.T) {
	uri := os.Getenv("MONGO_TEST_URI")
	if uri == "" {
		t.Skip("sin MONGO_TEST_URI: se saltea el test contra MongoDB real")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cli, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	base := "catalogo_test_" + time.Now().Format("150405")
	t.Cleanup(func() { _ = cli.Database(base).Drop(context.Background()); _ = cli.Disconnect(context.Background()) })

	repo := Nuevo(cli, base)
	if err := repo.CrearIndices(ctx); err != nil {
		t.Fatal(err)
	}
	ahora := time.Now().UTC().Truncate(time.Millisecond)
	cocinas := []domain.Cocina{
		{ID: "b", Nombre: "B", Activa: true, Restaurantes: []domain.Restaurante{{ID: "r1", Nombre: "R1", Activo: true}}},
		{ID: "a", Nombre: "A", Activa: false},
	}
	platos := []domain.Plato{
		{ID: "p2", CocinaID: "b", RestauranteID: "r1", Nombre: "Dos", Precio: 200, Activo: true, Etiquetas: []string{}, ActualizadoEn: ahora},
		{ID: "p1", CocinaID: "b", RestauranteID: "r1", Nombre: "Uno", Precio: 100, Activo: false, Etiquetas: []string{"Vegano"}, ActualizadoEn: ahora},
	}
	if err := repo.InsertarIniciales(ctx, cocinas, platos); err != nil {
		t.Fatal(err)
	}
	// Insertar de nuevo no falla ni pisa (clave duplicada).
	platos[0].Precio = 999
	if err := repo.InsertarIniciales(ctx, cocinas, platos); err != nil {
		t.Fatalf("la segunda carga tiene que ignorar duplicados: %v", err)
	}
	if p, ok, _ := repo.ObtenerPlato(ctx, "p2"); !ok || p.Precio != 200 || !p.ActualizadoEn.Equal(ahora) {
		t.Fatalf("se pisó el plato: %+v", p)
	}
	if cs, _ := repo.ListarCocinas(ctx, true); len(cs) != 1 || cs[0].ID != "b" {
		t.Fatalf("solo activas: %+v", cs)
	}
	if cs, _ := repo.ListarCocinas(ctx, false); len(cs) != 2 || cs[0].ID != "a" {
		t.Fatalf("todas, ordenadas por id: %+v", cs)
	}
	if ps, _ := repo.ListarPlatosDeCocina(ctx, "b"); len(ps) != 1 || ps[0].ID != "p2" {
		t.Fatalf("menú solo con activos: %+v", ps)
	}
	ps, total, err := repo.PaginaPlatos(ctx, 1, 1)
	if err != nil || total != 2 || len(ps) != 1 || ps[0].ID != "p1" {
		t.Fatalf("página: %+v total=%d err=%v", ps, total, err)
	}
	if _, ok, err := repo.ObtenerCocina(ctx, "zzz"); ok || err != nil {
		t.Fatal("una cocina inexistente tiene que dar ok=false sin error")
	}
}

// Las escrituras del admin (G11): crear sin pisar y reemplazar solo con la versión esperada.
func TestIntegracionEscriturasMongo(t *testing.T) {
	uri := os.Getenv("MONGO_TEST_URI")
	if uri == "" {
		t.Skip("sin MONGO_TEST_URI: se saltea el test contra MongoDB real")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cli, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	base := "catalogo_test_esc_" + time.Now().Format("150405")
	t.Cleanup(func() { _ = cli.Database(base).Drop(context.Background()); _ = cli.Disconnect(context.Background()) })
	repo := Nuevo(cli, base)

	ahora := time.Now()
	c := domain.Cocina{ID: "guemes", Nombre: "Güemes", Activa: true, Version: 1, Restaurantes: []domain.Restaurante{}}
	if err := repo.CrearCocina(ctx, c, domain.EventoCocina(domain.CocinaCreada, c, "t1", ahora)); err != nil {
		t.Fatal(err)
	}
	if err := repo.CrearCocina(ctx, c, domain.EventoCocina(domain.CocinaCreada, c, "t2", ahora)); !errors.Is(err, domain.ErrYaExiste) {
		t.Fatalf("crear dos veces: %v", err)
	}
	c.Restaurantes = append(c.Restaurantes, domain.Restaurante{ID: "r1", Nombre: "Mostaza", Activo: true})
	c.Version = 2
	if err := repo.ReemplazarCocina(ctx, c, 1, domain.EventoCocina(domain.CocinaActualizada, c, "t3", ahora.Add(time.Second))); err != nil {
		t.Fatal(err)
	}
	// Otra solicitud que había leído la versión 1 ya no puede guardar.
	viejo := domain.Cocina{ID: "guemes", Nombre: "Pisado", Version: 2}
	if err := repo.ReemplazarCocina(ctx, viejo, 1, domain.EventoCocina(domain.CocinaActualizada, viejo, "t4", ahora.Add(2*time.Second))); !errors.Is(err, domain.ErrConflicto) {
		t.Fatalf("versión vieja: %v", err)
	}
	guardada, _, _ := repo.ObtenerCocina(ctx, "guemes")
	if guardada.Nombre != "Güemes" || guardada.Version != 2 || len(guardada.Restaurantes) != 1 {
		t.Fatalf("quedó %+v", guardada)
	}

	// Outbox: solo los dos cambios que se guardaron tienen su evento; los que fallaron, no
	// (la transacción se canceló entera).
	pend, err := repo.Pendientes(ctx, 10)
	if err != nil || len(pend) != 2 || pend[0].Tipo != domain.CocinaCreada || pend[1].Tipo != domain.CocinaActualizada {
		t.Fatalf("pendientes: %+v %v", pend, err)
	}
	if err := repo.MarcarPublicado(ctx, pend[0].EventoID, time.Now()); err != nil {
		t.Fatal(err)
	}
	if pend, _ := repo.Pendientes(ctx, 10); len(pend) != 1 || pend[0].Tipo != domain.CocinaActualizada {
		t.Fatalf("después de publicar el primero: %+v", pend)
	}
}
