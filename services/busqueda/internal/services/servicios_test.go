package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"ghostkitchen/busqueda/internal/domain"
	"ghostkitchen/busqueda/internal/geo"
	"ghostkitchen/busqueda/internal/repositories/memoria"
	"ghostkitchen/busqueda/internal/seed"
)

var (
	ctx         = context.Background()
	nuevaCba    = domain.Ubicacion{Lat: -31.4262, Lng: -64.1883}
	buenosAires = domain.Ubicacion{Lat: -34.6037, Lng: -58.3816}
)

// conDatos arma el servicio con las 4 cocinas y los 64 platos de ejemplo.
func conDatos(t *testing.T) (*Busqueda, *memoria.Repositorio) {
	t.Helper()
	repo := memoria.Nuevo()
	if _, _, err := seed.Cargar(ctx, repo, time.Now()); err != nil {
		t.Fatal(err)
	}
	return Nueva(repo), repo
}

func TestSedeCercanaDentroDeCobertura(t *testing.T) {
	svc, _ := conDatos(t)
	sede, d, err := svc.SedeCercana(ctx, domain.Ubicacion{Lat: -31.333, Lng: -64.255})
	if err != nil {
		t.Fatal(err)
	}
	if sede.ID != "cerro" {
		t.Fatalf("esperaba cerro, obtuve %s", sede.ID)
	}
	if d != geo.Redondear(d, 1) {
		t.Fatalf("la distancia %v no está redondeada a 0,1 km", d)
	}
}

func TestSedeCercanaSinCobertura(t *testing.T) {
	svc, _ := conDatos(t)
	_, _, err := svc.SedeCercana(ctx, buenosAires)
	if !errors.Is(err, domain.ErrSinCobertura) {
		t.Fatalf("esperaba ErrSinCobertura, obtuve %v", err)
	}
}

func TestSedeCercanaSinSedesActivas(t *testing.T) {
	svc := Nueva(memoria.Nuevo())
	_, _, err := svc.SedeCercana(ctx, nuevaCba)
	if !errors.Is(err, domain.ErrSinCobertura) {
		t.Fatalf("esperaba ErrSinCobertura, obtuve %v", err)
	}
}

func TestFallaDelIndiceNoSeDisimula(t *testing.T) {
	svc, repo := conDatos(t)
	repo.Falla = domain.ErrNoDisponible
	if _, err := svc.ListarSedes(ctx); !errors.Is(err, domain.ErrNoDisponible) {
		t.Fatalf("ListarSedes: esperaba ErrNoDisponible, obtuve %v", err)
	}
	if _, err := svc.BuscarPlatos(ctx, filtro("nueva-cordoba")); !errors.Is(err, domain.ErrNoDisponible) {
		t.Fatalf("BuscarPlatos: esperaba ErrNoDisponible, obtuve %v", err)
	}
}

func filtro(sede string) domain.FiltroPlatos {
	return domain.FiltroPlatos{SedeID: sede, SoloDisponibles: true, Orden: domain.OrdenRelevancia, Pagina: 1, TamPagina: 20}
}

func TestBuscarPlatosSedeInexistente(t *testing.T) {
	svc, _ := conDatos(t)
	_, err := svc.BuscarPlatos(ctx, filtro("rio-cuarto"))
	if !errors.Is(err, domain.ErrSedeNoEncontrada) {
		t.Fatalf("esperaba ErrSedeNoEncontrada, obtuve %v", err)
	}
}

func TestBuscarPlatosSoloDisponiblesYPaginas(t *testing.T) {
	svc, _ := conDatos(t)
	f := filtro("nueva-cordoba")
	f.TamPagina = 5
	res, err := svc.BuscarPlatos(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	// Nueva Córdoba tiene 16 platos y 2 sin stock (p5 y p16).
	if res.Total != 14 || res.TotalPagina != 3 || len(res.Items) != 5 {
		t.Fatalf("total=%d páginas=%d items=%d", res.Total, res.TotalPagina, len(res.Items))
	}
	for _, p := range res.Items {
		if !p.Disponible {
			t.Errorf("apareció %s sin stock", p.ID)
		}
	}
	f.SoloDisponibles = false
	res, _ = svc.BuscarPlatos(ctx, f)
	if res.Total != 16 {
		t.Fatalf("con soloDisponibles=false esperaba 16, obtuve %d", res.Total)
	}
}

func TestBuscarPlatosSedeSinResultadosDevuelveListaVacia(t *testing.T) {
	svc, _ := conDatos(t)
	f := filtro("nueva-cordoba")
	f.Texto = "no-existe-nada"
	res, err := svc.BuscarPlatos(ctx, f)
	if err != nil || res.Items == nil || len(res.Items) != 0 || res.TotalPagina != 0 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestPlatoDesactivadoNoSeEncuentra(t *testing.T) {
	svc, _ := conDatos(t)
	if _, err := svc.ObtenerPlato(ctx, "p64"); !errors.Is(err, domain.ErrPlatoNoEncontrado) {
		t.Fatalf("esperaba ErrPlatoNoEncontrado, obtuve %v", err)
	}
	if p, err := svc.ObtenerPlato(ctx, "p1"); err != nil || p.Nombre != "Mega doble" {
		t.Fatalf("p1: %+v %v", p, err)
	}
}
