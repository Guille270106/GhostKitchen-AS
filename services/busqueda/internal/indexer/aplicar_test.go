package indexer

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"ghostkitchen/busqueda/internal/clients"
	"ghostkitchen/busqueda/internal/domain"
	"ghostkitchen/busqueda/internal/repositories/memoria"
)

// cocinaNC arma la cocina de Nueva Córdoba con Mostaza (r1) y Sushi Club (r2, inactivo).
func cocinaNC(version int64, activa bool) clients.CocinaCatalogo {
	var c clients.CocinaCatalogo
	_ = json.Unmarshal([]byte(`{"id":"nueva-cordoba","nombre":"Cocina Nueva Córdoba","activa":true,
		"ubicacion":{"lat":-31.4262,"lng":-64.1883},
		"restaurantes":[{"id":"r1","nombre":"Mostaza","activo":true},{"id":"r2","nombre":"Sushi Club","activo":false}]}`), &c)
	c.Version, c.Activa = version, activa
	return c
}

func platoNC(id string, version int64, precio int) clients.PlatoCatalogo {
	return clients.PlatoCatalogo{ID: id, CocinaID: "nueva-cordoba", RestauranteID: "r1", Nombre: "Hamburguesa",
		Categoria: "Hamburguesas", Precio: precio, Activo: true, Version: version, ActualizadoEn: time.Now()}
}

func stock(id string, secuencia int64, disponible bool) clients.DisponibilidadInventario {
	return clients.DisponibilidadInventario{PlatoID: id, CocinaID: "nueva-cordoba", Disponible: disponible, TieneReceta: true, Secuencia: secuencia}
}

func preparar(t *testing.T) (*Aplicador, *memoria.Repositorio) {
	t.Helper()
	repo := memoria.Nuevo()
	a := NuevoAplicador(repo)
	if err := a.Cocina(context.Background(), cocinaNC(1, true)); err != nil {
		t.Fatal(err)
	}
	return a, repo
}

func TestCadaFuenteCambiaSoloSusCampos(t *testing.T) {
	a, repo := preparar(t)
	ctx := context.Background()
	must(t, a.Plato(ctx, platoNC("p1", 2, 1000)))
	must(t, a.Disponibilidad(ctx, stock("p1", 3, true)))
	must(t, a.Plato(ctx, platoNC("p1", 3, 1200))) // un cambio de precio no toca el stock
	p, _ := repo.Plato("p1")
	if p.Precio != 1200 || !p.Disponible || p.VersionInventario != 3 {
		t.Fatalf("el precio pisó el stock: %+v", p)
	}
	must(t, a.Disponibilidad(ctx, stock("p1", 4, false))) // un cambio de stock no toca el precio
	p, _ = repo.Plato("p1")
	if p.Precio != 1200 || p.Disponible || p.VersionCatalogo != 3 {
		t.Fatalf("el stock pisó el precio: %+v", p)
	}
}

func TestMensajesViejosSeIgnoran(t *testing.T) {
	a, repo := preparar(t)
	ctx := context.Background()
	must(t, a.Plato(ctx, platoNC("p1", 3, 1500)))
	must(t, a.Disponibilidad(ctx, stock("p1", 5, true)))
	// Llegan tarde (por un reintento) la versión 2 del plato y el contador 4 del stock.
	must(t, a.Plato(ctx, platoNC("p1", 2, 900)))
	must(t, a.Disponibilidad(ctx, stock("p1", 4, false)))
	p, _ := repo.Plato("p1")
	if p.Precio != 1500 || !p.Disponible {
		t.Fatalf("un mensaje viejo pisó uno nuevo: %+v", p)
	}
	// Y la cocina vieja tampoco vuelve atrás.
	must(t, a.Cocina(ctx, cocinaNC(3, false)))
	must(t, a.Cocina(ctx, cocinaNC(2, true)))
	if s, _ := repo.Sede("nueva-cordoba"); s.Activa || s.VersionCatalogo != 3 {
		t.Fatalf("%+v", s)
	}
}

func TestMensajeRepetidoNoCambiaNada(t *testing.T) {
	a, repo := preparar(t)
	ctx := context.Background()
	must(t, a.Plato(ctx, platoNC("p1", 2, 1000)))
	antes, _ := repo.Plato("p1")
	must(t, a.Plato(ctx, platoNC("p1", 2, 1000)))
	despues, _ := repo.Plato("p1")
	if antes.Precio != despues.Precio || antes.VersionCatalogo != despues.VersionCatalogo || antes.Activo != despues.Activo {
		t.Fatalf("%+v → %+v", antes, despues)
	}
}

func TestInventarioAvisaAntesQueCatalogo(t *testing.T) {
	a, repo := preparar(t)
	ctx := context.Background()
	must(t, a.Disponibilidad(ctx, stock("p9", 1, true)))
	if _, ok, _ := repo.ObtenerPlato(ctx, "p9"); ok {
		t.Fatal("un plato que solo tiene stock no se muestra")
	}
	must(t, a.Plato(ctx, platoNC("p9", 1, 800)))
	p, ok, _ := repo.ObtenerPlato(ctx, "p9")
	if !ok || !p.Disponible || p.Restaurante.Nombre != "Mostaza" || p.Sede.Nombre != "Cocina Nueva Córdoba" {
		t.Fatalf("%v %+v", ok, p)
	}
}

func TestPlatoNuevoEntraNoDisponible(t *testing.T) {
	a, repo := preparar(t)
	must(t, a.Plato(context.Background(), platoNC("p1", 1, 1000)))
	if p, _ := repo.Plato("p1"); p.Disponible {
		t.Fatal("sin datos de Inventario, un plato nuevo no se puede pedir")
	}
}

func TestSinRecetaNuncaDisponible(t *testing.T) {
	a, repo := preparar(t)
	ctx := context.Background()
	must(t, a.Plato(ctx, platoNC("p1", 1, 1000)))
	d := stock("p1", 1, true)
	d.TieneReceta = false
	must(t, a.Disponibilidad(ctx, d))
	if p, _ := repo.Plato("p1"); p.Disponible || p.TieneReceta {
		t.Fatalf("%+v", p)
	}
}

func TestLaListaSinContadorNoPisaUnMensaje(t *testing.T) {
	a, repo := preparar(t)
	ctx := context.Background()
	must(t, a.Plato(ctx, platoNC("p1", 1, 1000)))
	must(t, a.Disponibilidad(ctx, stock("p1", 2, true)))
	must(t, a.Disponibilidad(ctx, stock("p1", 0, false))) // lista de la reconstrucción
	if p, _ := repo.Plato("p1"); !p.Disponible {
		t.Fatal("la lista sin contador pisó un mensaje")
	}
	must(t, a.Plato(ctx, platoNC("p2", 1, 1000)))
	must(t, a.Disponibilidad(ctx, stock("p2", 0, true))) // nunca tuvo mensaje: se usa la lista
	if p, _ := repo.Plato("p2"); !p.Disponible {
		t.Fatal("la lista tiene que valer para un plato sin mensajes")
	}
}

func TestCambiosDeLaCocinaLleganASusPlatos(t *testing.T) {
	a, repo := preparar(t)
	ctx := context.Background()
	must(t, a.Plato(ctx, platoNC("p1", 1, 1000)))
	sushi := platoNC("p2", 1, 2000)
	sushi.RestauranteID = "r2" // restaurante inactivo
	must(t, a.Plato(ctx, sushi))
	if p, _ := repo.Plato("p2"); p.Activo || p.Restaurante.Nombre != "Sushi Club" {
		t.Fatalf("plato de un restaurante inactivo: %+v", p)
	}

	// Se desactiva la cocina: ningún plato se muestra.
	must(t, a.Cocina(ctx, cocinaNC(2, false)))
	if _, ok, _ := repo.ObtenerPlato(ctx, "p1"); ok {
		t.Fatal("cocina desactivada: p1 no se tiene que mostrar")
	}

	// Vuelve, con otro nombre, y se reactiva Sushi Club.
	c := cocinaNC(3, true)
	c.Nombre = "Cocina NC"
	c.Restaurantes[1].Activo = true
	must(t, a.Cocina(ctx, c))
	for _, id := range []string{"p1", "p2"} {
		p, ok, _ := repo.ObtenerPlato(ctx, id)
		if !ok || p.Sede.Nombre != "Cocina NC" {
			t.Fatalf("%s: %v %+v", id, ok, p)
		}
	}

	// Un plato desactivado por la cocina no vuelve aunque se reactive la cocina.
	p1 := platoNC("p1", 2, 1000)
	p1.Activo = false
	must(t, a.Plato(ctx, p1))
	must(t, a.Cocina(ctx, cocinaNC(4, true)))
	if _, ok, _ := repo.ObtenerPlato(ctx, "p1"); ok {
		t.Fatal("p1 está desactivado")
	}
}

func TestPlatoAntesQueSuCocina(t *testing.T) {
	repo := memoria.Nuevo()
	a := NuevoAplicador(repo)
	ctx := context.Background()
	must(t, a.Plato(ctx, platoNC("p1", 1, 1000)))
	if p, ok := repo.Plato("p1"); !ok || p.Activo || p.Sede.ID != "nueva-cordoba" {
		t.Fatalf("se guarda inactivo, con el id de la cocina: %v %+v", ok, p)
	}
	must(t, a.Cocina(ctx, cocinaNC(1, true)))
	p, ok, _ := repo.ObtenerPlato(ctx, "p1")
	if !ok || p.Restaurante.Nombre != "Mostaza" || p.Sede.Nombre != "Cocina Nueva Córdoba" {
		t.Fatalf("al llegar la cocina se completa: %v %+v", ok, p)
	}
}

func TestSolrCaidoEsPasajero(t *testing.T) {
	repo := memoria.Nuevo()
	repo.Falla = domain.ErrNoDisponible
	err := NuevoAplicador(repo).Disponibilidad(context.Background(), stock("p1", 1, true))
	if !errors.Is(err, domain.ErrNoDisponible) || errors.Is(err, ErrEventoInvalido) {
		t.Fatal(err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
