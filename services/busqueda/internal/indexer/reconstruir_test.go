package indexer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"ghostkitchen/busqueda/internal/clients"
	"ghostkitchen/busqueda/internal/repositories/memoria"
)

// catalogoFalso responde /internal/cocinas y /internal/platos como Catálogo.
func catalogoFalso(t *testing.T, platos []clients.PlatoCatalogo, fallarPagina int) *httptest.Server {
	t.Helper()
	cocinas := []map[string]any{
		{"id": "nueva-cordoba", "nombre": "Cocina Nueva Córdoba", "activa": true,
			"ubicacion": map[string]float64{"lat": -31.4262, "lng": -64.1883},
			"restaurantes": []map[string]any{
				{"id": "r1", "nombre": "Mostaza", "activo": true},
				{"id": "r2", "nombre": "Sushi Club", "activo": false},
			}},
		{"id": "cerrada", "nombre": "Cocina Cerrada", "activa": false,
			"restaurantes": []map[string]any{{"id": "r9", "nombre": "Otra", "activo": true}}},
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/internal/cocinas":
			_ = json.NewEncoder(w).Encode(map[string]any{"items": cocinas})
		case "/internal/platos":
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
			if page == fallarPagina {
				w.WriteHeader(503)
				return
			}
			desde, hasta := min((page-1)*size, len(platos)), min(page*size, len(platos))
			total := (len(platos) + size - 1) / size
			_ = json.NewEncoder(w).Encode(map[string]any{"items": platos[desde:hasta], "page": page, "totalPages": total})
		default:
			w.WriteHeader(404)
		}
	}))
}

func plato(id, rest string, activo bool) clients.PlatoCatalogo {
	cocina := "nueva-cordoba"
	if rest == "r9" {
		cocina = "cerrada"
	}
	return clients.PlatoCatalogo{ID: id, CocinaID: cocina, RestauranteID: rest, Nombre: "Plato " + id, Precio: 1000, Activo: activo, Version: 1, ActualizadoEn: time.Now()}
}

func TestReconstruirDesdeCatalogo(t *testing.T) {
	platos := []clients.PlatoCatalogo{
		plato("p1", "r1", true),  // visible
		plato("p2", "r1", false), // plato desactivado
		plato("p3", "r2", true),  // restaurante desactivado
		plato("p4", "r9", true),  // cocina desactivada
		plato("p5", "rX", true),  // restaurante desconocido: queda inactivo
	}
	srv := catalogoFalso(t, platos, 0)
	defer srv.Close()
	repo := memoria.Nuevo()
	dispo := func(id string) bool { return id == "p1" }
	rec := NuevoReconstructor(clients.NuevoCatalogo(srv.URL), nil, repo, dispo)

	res, err := rec.Reconstruir(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Sedes != 2 || res.Platos != 5 {
		t.Fatalf("%+v", res)
	}
	ctx := context.Background()
	sedes, _ := repo.ListarSedes(ctx)
	if len(sedes) != 1 || len(sedes[0].Restaurantes) != 1 || sedes[0].Restaurantes[0].ID != "r1" {
		t.Fatalf("solo la cocina activa, con sus restaurantes activos: %+v", sedes)
	}
	for id, visible := range map[string]bool{"p1": true, "p2": false, "p3": false, "p4": false, "p5": false} {
		_, ok, _ := repo.ObtenerPlato(ctx, id)
		if ok != visible {
			t.Errorf("%s: visible=%v, esperaba %v", id, ok, visible)
		}
	}
	p1, _, _ := repo.ObtenerPlato(ctx, "p1")
	if !p1.Disponible || p1.Sede.ID != "nueva-cordoba" || p1.Restaurante.Nombre != "Mostaza" || p1.Moneda != "ARS" {
		t.Fatalf("%+v", p1)
	}
}

func TestReconstruirRecorreTodasLasPaginas(t *testing.T) {
	var platos []clients.PlatoCatalogo
	for i := 1; i <= 250; i++ {
		platos = append(platos, plato("p"+strconv.Itoa(i), "r1", true))
	}
	srv := catalogoFalso(t, platos, 0)
	defer srv.Close()
	res, err := NuevoReconstructor(clients.NuevoCatalogo(srv.URL), nil, memoria.Nuevo(), func(string) bool { return true }).Reconstruir(context.Background())
	if err != nil || res.Platos != 250 {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestReconstruirConCatalogoCaido(t *testing.T) {
	var platos []clients.PlatoCatalogo
	for i := 1; i <= 150; i++ {
		platos = append(platos, plato("p"+strconv.Itoa(i), "r1", true))
	}
	srv := catalogoFalso(t, platos, 2) // la segunda página falla siempre
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := NuevoReconstructor(clients.NuevoCatalogo(srv.URL), nil, memoria.Nuevo(), func(string) bool { return true }).Reconstruir(ctx)
	if err == nil || res.Platos != 100 {
		t.Fatalf("tenía que fallar después de guardar la primera página: %+v %v", res, err)
	}
}

// inventarioFalso devuelve la lista de disponibilidad, o falla.
type inventarioFalso struct {
	lista []clients.DisponibilidadInventario
	err   error
}

func (f inventarioFalso) Disponibilidad(context.Context) ([]clients.DisponibilidadInventario, error) {
	return f.lista, f.err
}

func TestReconstruirConInventario(t *testing.T) {
	srv := catalogoFalso(t, []clients.PlatoCatalogo{plato("p1", "r1", true), plato("p2", "r1", true)}, 0)
	defer srv.Close()
	repo := memoria.Nuevo()
	inv := inventarioFalso{lista: []clients.DisponibilidadInventario{
		{PlatoID: "p1", CocinaID: "nueva-cordoba", Disponible: true, TieneReceta: true},
		{PlatoID: "p2", CocinaID: "nueva-cordoba", Disponible: false, TieneReceta: false},
	}}
	// La tabla provisional no se usa si hay Inventario.
	res, err := NuevoReconstructor(clients.NuevoCatalogo(srv.URL), inv, repo, func(string) bool { return true }).Reconstruir(context.Background())
	if err != nil || res.Disponibilidad != 2 || res.SinInventario {
		t.Fatalf("%+v %v", res, err)
	}
	if p, _ := repo.Plato("p1"); !p.Disponible || !p.TieneReceta {
		t.Fatalf("p1: %+v", p)
	}
	if p, _ := repo.Plato("p2"); p.Disponible {
		t.Fatalf("p2 no tiene receta: %+v", p)
	}
}

func TestReconstruirConInventarioCaidoSigue(t *testing.T) {
	srv := catalogoFalso(t, []clients.PlatoCatalogo{plato("p1", "r1", true)}, 0)
	defer srv.Close()
	repo := memoria.Nuevo()
	res, err := NuevoReconstructor(clients.NuevoCatalogo(srv.URL), inventarioFalso{err: errors.New("caído")}, repo, nil).Reconstruir(context.Background())
	if err != nil || !res.SinInventario || res.Platos != 1 {
		t.Fatalf("sin Inventario tiene que seguir: %+v %v", res, err)
	}
	if _, ok, _ := repo.ObtenerPlato(context.Background(), "p1"); !ok {
		t.Fatal("el menú se ve igual, aunque sin disponibilidad")
	}
}

func TestReconstruirNoVuelveAtrasUnMensajeMasNuevo(t *testing.T) {
	srv := catalogoFalso(t, []clients.PlatoCatalogo{plato("p1", "r1", true)}, 0) // versión 1
	defer srv.Close()
	repo := memoria.Nuevo()
	ctx := context.Background()
	a := NuevoAplicador(repo)
	// Mientras se reconstruía, llegaron por mensajes una versión 2 del plato y su stock.
	if err := a.Cocina(ctx, cocinaNC(1, true)); err != nil {
		t.Fatal(err)
	}
	nuevo := plato("p1", "r1", true)
	nuevo.Version, nuevo.Precio = 2, 1500
	if err := a.Plato(ctx, nuevo); err != nil {
		t.Fatal(err)
	}
	if err := a.Disponibilidad(ctx, clients.DisponibilidadInventario{PlatoID: "p1", CocinaID: "nueva-cordoba", Disponible: true, TieneReceta: true, Secuencia: 4}); err != nil {
		t.Fatal(err)
	}
	inv := inventarioFalso{lista: []clients.DisponibilidadInventario{{PlatoID: "p1", CocinaID: "nueva-cordoba", Disponible: false, TieneReceta: true}}}
	if _, err := NuevoReconstructor(clients.NuevoCatalogo(srv.URL), inv, repo, nil).Reconstruir(ctx); err != nil {
		t.Fatal(err)
	}
	p, _ := repo.Plato("p1")
	if p.Precio != 1500 || p.VersionCatalogo != 2 || !p.Disponible || p.VersionInventario != 4 {
		t.Fatalf("la reconstrucción pisó datos más nuevos: %+v", p)
	}
}
