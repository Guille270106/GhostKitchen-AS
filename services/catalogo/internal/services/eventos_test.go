package services

import (
	"context"
	"testing"

	"ghostkitchen/catalogo/internal/domain"
)

func tipos(evs []domain.Evento) []string {
	out := make([]string, len(evs))
	for i, e := range evs {
		out[i] = e.Tipo
	}
	return out
}

// Cada cambio guardado deja su evento en el outbox, con el tipo que corresponde y la versión
// como secuencia. Un cambio rechazado no deja ninguno.
func TestCadaCambioDejaSuEvento(t *testing.T) {
	svc, repo := entornoAdmin(t)
	ctx := context.Background()
	c, _ := svc.CrearCocina(ctx, adminNC, datosNueva)
	_, _ = svc.AgregarRestaurante(ctx, adminNC, c.ID, "Mostaza")
	_, _ = svc.AgregarRestaurante(ctx, adminNC, c.ID, "mostaza") // repetido: rechazado
	_, _ = svc.ModificarCocina(ctx, adminNC, c.ID, domain.CambiosCocina{Activa: ptr(false)})
	_, _ = svc.ModificarCocina(ctx, adminNC, "cerro", domain.CambiosCocina{Activa: ptr(false)}) // otra localidad

	evs := repo.Eventos()
	want := []string{"cocina.creada", "cocina.actualizada", "cocina.desactivada"}
	if got := tipos(evs); len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("eventos: %v, esperaba %v", got, want)
	}
	for i, e := range evs {
		if e.AgregadoID != c.ID || e.Secuencia != int64(i+1) || e.EventoID == "" {
			t.Fatalf("evento %d: %+v", i, e)
		}
	}
}

func TestEventosDePlatos(t *testing.T) {
	svc, repo, rec := entornoPlatos(t)
	ctx := context.Background()
	p, _ := svc.CrearPlato(ctx, cocinaNC, "r1", platoNuevo)
	_, _ = svc.ModificarPlato(ctx, cocinaNC, p.ID, domain.CambiosPlato{Activo: ptr(true)}, "cid-1") // sin receta: rechazado
	rec.con[p.ID] = true
	_, _ = svc.ModificarPlato(ctx, cocinaNC, p.ID, domain.CambiosPlato{Activo: ptr(true)}, "cid-2")
	_, _ = svc.ModificarPlato(ctx, cocinaNC, p.ID, domain.CambiosPlato{Activo: ptr(false)}, "cid-3")

	evs := repo.Eventos()
	got := tipos(evs)
	if len(got) != 3 || got[0] != "plato.creado" || got[1] != "plato.actualizado" || got[2] != "plato.desactivado" {
		t.Fatalf("eventos: %v", got)
	}
	if evs[1].CorrelationID != "cid-2" || evs[2].Secuencia != 3 {
		t.Fatalf("correlation y secuencia: %+v", evs[1:])
	}
}
