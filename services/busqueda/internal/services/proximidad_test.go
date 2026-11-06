package services

import (
	"errors"
	"testing"

	"ghostkitchen/busqueda/internal/domain"
	"ghostkitchen/busqueda/internal/geo"
	"ghostkitchen/busqueda/internal/repositories/memoria"
)

func TestProximidadMismoPunto(t *testing.T) {
	svc, _ := conDatos(t)
	res, err := svc.Proximidad(ctx, ConsultaProximidad{Cliente: nuevaCba, Sede: nuevaCba, RadioKm: 5})
	if err != nil {
		t.Fatal(err)
	}
	if res.SedeExterna.DistanciaKm != 0 || !res.SedeExterna.DentroDelRango || res.DarkKitchen != nil {
		t.Fatalf("%+v", res)
	}
}

func TestProximidadLimiteInclusivoSinRedondear(t *testing.T) {
	svc, _ := conDatos(t)
	sede := domain.Ubicacion{Lat: -31.40, Lng: -64.20}
	d := geo.DistanciaKm(nuevaCba, sede)

	res, _ := svc.Proximidad(ctx, ConsultaProximidad{Cliente: nuevaCba, Sede: sede, RadioKm: d})
	if !res.SedeExterna.DentroDelRango {
		t.Fatal("una sede justo en el radio tiene que estar dentro (límite inclusivo)")
	}
	res, _ = svc.Proximidad(ctx, ConsultaProximidad{Cliente: nuevaCba, Sede: sede, RadioKm: d - 1e-9})
	if res.SedeExterna.DentroDelRango {
		t.Fatal("se comparó con la distancia redondeada")
	}
	if res.SedeExterna.DistanciaKm != geo.Redondear(d, 3) {
		t.Fatalf("la distancia mostrada tiene que tener 3 decimales: %v", res.SedeExterna.DistanciaKm)
	}
}

func TestProximidadSinDarkKitchenNoDependeDelIndice(t *testing.T) {
	svc, repo := conDatos(t)
	repo.Falla = domain.ErrNoDisponible
	res, err := svc.Proximidad(ctx, ConsultaProximidad{Cliente: nuevaCba, Sede: buenosAires, RadioKm: 10})
	if err != nil {
		t.Fatalf("con incluirDarkKitchen=false no tiene que consultar Solr: %v", err)
	}
	if res.SedeExterna.DentroDelRango {
		t.Fatal("Buenos Aires no está a 10 km de Córdoba")
	}
}

func TestProximidadConDarkKitchenYIndiceCaido(t *testing.T) {
	svc, repo := conDatos(t)
	repo.Falla = domain.ErrNoDisponible
	_, err := svc.Proximidad(ctx, ConsultaProximidad{Cliente: nuevaCba, Sede: nuevaCba, RadioKm: 5, IncluirDarkKitchen: true})
	if !errors.Is(err, domain.ErrNoDisponible) {
		t.Fatalf("esperaba ErrNoDisponible (503), obtuve %v", err)
	}
}

func TestProximidadCocinaCercaDelClienteAunqueLaSedeEsteLejos(t *testing.T) {
	svc, _ := conDatos(t)
	res, err := svc.Proximidad(ctx, ConsultaProximidad{Cliente: nuevaCba, Sede: buenosAires, RadioKm: 5, IncluirDarkKitchen: true})
	if err != nil {
		t.Fatal(err)
	}
	dk := res.DarkKitchen
	if res.SedeExterna.DentroDelRango || dk == nil || dk.Estado != EstadoEnRango || dk.Sede.ID != "nueva-cordoba" {
		t.Fatalf("%+v %+v", res, dk)
	}
}

func TestProximidadRadioAplicadoMaximo25(t *testing.T) {
	svc, _ := conDatos(t)
	res, _ := svc.Proximidad(ctx, ConsultaProximidad{Cliente: nuevaCba, Sede: nuevaCba, RadioKm: 300, IncluirDarkKitchen: true})
	if res.DarkKitchen.RadioAplicadoKm != 25 || res.RadioKm != 300 {
		t.Fatalf("%+v", res.DarkKitchen)
	}
}

func TestProximidadCocinaFueraDeRango(t *testing.T) {
	svc, _ := conDatos(t)
	res, _ := svc.Proximidad(ctx, ConsultaProximidad{Cliente: buenosAires, Sede: buenosAires, RadioKm: 50, IncluirDarkKitchen: true})
	dk := res.DarkKitchen
	if !res.SedeExterna.DentroDelRango || dk.Estado != EstadoFueraRango || dk.DentroDelRango || dk.Sede == nil || dk.DistanciaKm == nil {
		t.Fatalf("%+v %+v", res, dk)
	}
}

func TestProximidadSinSedes(t *testing.T) {
	svc := Nueva(memoria.Nuevo())
	res, err := svc.Proximidad(ctx, ConsultaProximidad{Cliente: nuevaCba, Sede: nuevaCba, RadioKm: 5, IncluirDarkKitchen: true})
	if err != nil {
		t.Fatal(err)
	}
	dk := res.DarkKitchen
	if dk.Estado != EstadoSinSedes || dk.Sede != nil || dk.DistanciaKm != nil || dk.DentroDelRango || dk.RadioAplicadoKm != 5 {
		t.Fatalf("%+v", dk)
	}
}

func TestProximidadEmpatePorID(t *testing.T) {
	repo := memoria.Nuevo()
	punto := domain.Ubicacion{Lat: -31.4, Lng: -64.2}
	_ = repo.GuardarSedes(ctx, []domain.Sede{
		{ID: "zeta", Nombre: "Zeta", Ubicacion: punto, Activa: true},
		{ID: "alfa", Nombre: "Alfa", Ubicacion: punto, Activa: true},
	})
	res, _ := Nueva(repo).Proximidad(ctx, ConsultaProximidad{Cliente: punto, Sede: punto, RadioKm: 1, IncluirDarkKitchen: true})
	if res.DarkKitchen.Sede.ID != "alfa" {
		t.Fatalf("con empate tiene que ganar el id menor: %s", res.DarkKitchen.Sede.ID)
	}
}
