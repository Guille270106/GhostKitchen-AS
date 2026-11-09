package services

import (
	"context"
	"errors"
	"testing"

	"ghostkitchen/catalogo/internal/domain"
	"ghostkitchen/catalogo/internal/repositories/memoria"
)

type recetasFalsas struct {
	con   map[string]bool
	caido bool
	n     int
}

func (r *recetasFalsas) TieneReceta(_ context.Context, _, _, platoID, _ string) (bool, error) {
	r.n++
	if r.caido {
		return false, domain.ErrInventarioNoDisponible
	}
	return r.con[platoID], nil
}

var cocinaNC = Cocina{UsuarioID: "k1", Rol: "cocina", CocinaID: "nueva-cordoba"}

func entornoPlatos(t *testing.T) (*PlatosCocina, *memoria.Repositorio, *recetasFalsas) {
	t.Helper()
	_, repo := entornoAdmin(t) // nueva-cordoba (r1 Mostaza, r2 Sushi Pop) y cerro
	nc, _, _ := repo.ObtenerCocina(context.Background(), "nueva-cordoba")
	nc.Restaurantes = append(nc.Restaurantes, domain.Restaurante{ID: "r3", Nombre: "Cerrado", Activo: false})
	repo.Guardar(&nc, nil)
	repo.Guardar(nil, &domain.Plato{ID: "p-cerro", CocinaID: "cerro", RestauranteID: "rc", Nombre: "Ajeno", Categoria: "X", Precio: 100, Version: 1})
	rec := &recetasFalsas{con: map[string]bool{}}
	return NuevoPlatosCocina(repo, repo, rec), repo, rec
}

var platoNuevo = domain.CambiosPlato{Nombre: ptr("Mega triple"), Categoria: ptr("Hamburguesas"), Precio: ptr(12500), Etiquetas: &[]string{"Picante"}}

func TestCrearPlatoArrancaDesactivado(t *testing.T) {
	svc, repo, _ := entornoPlatos(t)
	activo := true
	datos := platoNuevo
	datos.Activo = &activo // aunque lo pida activo, sin receta no se publica (RN18)
	p, err := svc.CrearPlato(context.Background(), cocinaNC, "r1", datos)
	if err != nil {
		t.Fatal(err)
	}
	if p.Activo || p.CocinaID != "nueva-cordoba" || p.RestauranteID != "r1" || p.Moneda != "ARS" || p.Version != 1 {
		t.Fatalf("plato inesperado: %+v", p)
	}
	if _, ok, _ := repo.ObtenerPlato(context.Background(), p.ID); !ok {
		t.Fatal("no se guardó")
	}
}

func TestCrearPlatoRechazos(t *testing.T) {
	svc, _, _ := entornoPlatos(t)
	casos := []struct {
		nombre      string
		quien       Cocina
		restaurante string
		datos       domain.CambiosPlato
		esperado    error
	}{
		{"un admin", Cocina{UsuarioID: "a", Rol: "admin"}, "r1", platoNuevo, domain.ErrSinPermiso},
		{"restaurante de otra cocina", cocinaNC, "rc", platoNuevo, domain.ErrRestauranteNoEncontrado},
		{"restaurante desactivado", cocinaNC, "r3", platoNuevo, domain.ErrDatosInvalidos},
		{"precio 0", cocinaNC, "r1", domain.CambiosPlato{Nombre: ptr("X"), Categoria: ptr("Y"), Precio: ptr(0)}, domain.ErrDatosInvalidos},
	}
	for _, c := range casos {
		if _, err := svc.CrearPlato(context.Background(), c.quien, c.restaurante, c.datos); !errors.Is(err, c.esperado) {
			t.Errorf("%s: esperaba %v, obtuve %v", c.nombre, c.esperado, err)
		}
	}
}

func TestActivarExigeReceta(t *testing.T) {
	svc, _, rec := entornoPlatos(t)
	p, _ := svc.CrearPlato(context.Background(), cocinaNC, "r1", platoNuevo)
	activar := domain.CambiosPlato{Activo: ptr(true)}

	if _, err := svc.ModificarPlato(context.Background(), cocinaNC, p.ID, activar, ""); !errors.Is(err, domain.ErrSinReceta) {
		t.Fatalf("sin receta: %v", err)
	}
	rec.caido = true
	if _, err := svc.ModificarPlato(context.Background(), cocinaNC, p.ID, activar, ""); !errors.Is(err, domain.ErrInventarioNoDisponible) {
		t.Fatalf("Inventario caído no es \"sin receta\": %v", err)
	}
	rec.caido = false
	rec.con[p.ID] = true
	got, err := svc.ModificarPlato(context.Background(), cocinaNC, p.ID, activar, "")
	if err != nil || !got.Activo || got.Version != 2 {
		t.Fatalf("con receta se activa: %+v %v", got, err)
	}
}

func TestModificarYDesactivarNoPreguntaReceta(t *testing.T) {
	svc, _, rec := entornoPlatos(t)
	p, _ := svc.CrearPlato(context.Background(), cocinaNC, "r1", platoNuevo)
	got, err := svc.ModificarPlato(context.Background(), cocinaNC, p.ID, domain.CambiosPlato{Precio: ptr(13000), Activo: ptr(false)}, "")
	if err != nil || got.Precio != 13000 || rec.n != 0 {
		t.Fatalf("cambiar el precio o desactivar no consulta a Inventario: %+v %v (%d consultas)", got, err, rec.n)
	}
}

func TestPlatoDeOtraCocina(t *testing.T) {
	svc, _, _ := entornoPlatos(t)
	if _, err := svc.ModificarPlato(context.Background(), cocinaNC, "p-cerro", domain.CambiosPlato{Precio: ptr(1)}, ""); !errors.Is(err, domain.ErrOtraCocina) {
		t.Fatalf("otra cocina: %v", err)
	}
	if _, err := svc.ModificarPlato(context.Background(), cocinaNC, "no-existe", domain.CambiosPlato{}, ""); !errors.Is(err, domain.ErrPlatoNoEncontrado) {
		t.Fatalf("inexistente: %v", err)
	}
	mis, _ := svc.MisPlatos(context.Background(), cocinaNC)
	for _, p := range mis {
		if p.CocinaID != "nueva-cordoba" {
			t.Fatalf("MisPlatos trajo uno de otra cocina: %+v", p)
		}
	}
}
