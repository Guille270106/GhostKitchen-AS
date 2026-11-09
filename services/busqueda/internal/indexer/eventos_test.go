package indexer

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func sobre(t *testing.T, tipo, agregado string, secuencia int64, datos any) []byte {
	t.Helper()
	d, _ := json.Marshal(datos)
	b, _ := json.Marshal(map[string]any{
		"evento_id": "0b8e2f8e-1111-4c3a-9a51-2f3c1d2e4f50", "tipo": tipo, "version": 1,
		"ocurrido_en": "2026-11-09T12:00:00Z", "correlation_id": "corr-1",
		"agregado_id": agregado, "secuencia": secuencia, "datos": json.RawMessage(d),
	})
	return b
}

func TestLeerSobreValidaElContrato(t *testing.T) {
	bueno := sobre(t, "plato.creado", "p1", 1, map[string]any{"id": "p1"})
	if _, err := LeerSobre(bueno, "plato.creado"); err != nil {
		t.Fatal(err)
	}
	casos := map[string]struct {
		cuerpo []byte
		ruta   string
	}{
		"no es JSON":           {[]byte("{"), "plato.creado"},
		"tipo distinto a ruta": {bueno, "plato.actualizado"},
		"sin secuencia":        {sobre(t, "plato.creado", "p1", 0, map[string]any{}), "plato.creado"},
		"sin agregado":         {sobre(t, "plato.creado", "", 1, map[string]any{}), "plato.creado"},
	}
	for nombre, c := range casos {
		if _, err := LeerSobre(c.cuerpo, c.ruta); !errors.Is(err, ErrEventoInvalido) {
			t.Errorf("%s: %v", nombre, err)
		}
	}
}

func TestProcesarEventos(t *testing.T) {
	a, repo := preparar(t)
	ctx := context.Background()
	leer := func(cuerpo []byte, ruta string) Sobre {
		s, err := LeerSobre(cuerpo, ruta)
		must(t, err)
		return s
	}
	// Los datos son lo que publica Catálogo (G13) e Inventario (V12).
	plato := map[string]any{"id": "p1", "cocinaId": "nueva-cordoba", "restauranteId": "r1", "nombre": "Doble",
		"categoria": "Hamburguesas", "precio": 9000, "moneda": "ARS", "activo": true, "version": 2}
	must(t, a.Procesar(ctx, leer(sobre(t, "plato.creado", "p1", 2, plato), "plato.creado")))
	dispo := map[string]any{"plato_id": "p1", "cocina_id": "nueva-cordoba", "disponible": true, "tiene_receta": true}
	must(t, a.Procesar(ctx, leer(sobre(t, "disponibilidad.cambiada", "p1", 7, dispo), "disponibilidad.cambiada")))
	p, ok, _ := repo.ObtenerPlato(ctx, "p1")
	if !ok || p.Precio != 9000 || !p.Disponible || p.VersionInventario != 7 {
		t.Fatalf("%v %+v", ok, p)
	}

	c := cocinaNC(5, false)
	b, _ := json.Marshal(c)
	var datos map[string]any
	_ = json.Unmarshal(b, &datos)
	must(t, a.Procesar(ctx, leer(sobre(t, "cocina.desactivada", "nueva-cordoba", 5, datos), "cocina.desactivada")))
	if _, ok, _ := repo.ObtenerPlato(ctx, "p1"); ok {
		t.Fatal("cocina desactivada")
	}
}

func TestProcesarRechazaDatosQueNoCoinciden(t *testing.T) {
	a, _ := preparar(t)
	ctx := context.Background()
	casos := map[string]Sobre{}
	s, _ := LeerSobre(sobre(t, "plato.actualizado", "p1", 3,
		map[string]any{"id": "p1", "cocinaId": "nueva-cordoba", "version": 2}), "plato.actualizado")
	casos["secuencia distinta a la versión"] = s
	s, _ = LeerSobre(sobre(t, "plato.actualizado", "p1", 2,
		map[string]any{"id": "p2", "cocinaId": "nueva-cordoba", "version": 2}), "plato.actualizado")
	casos["otro plato"] = s
	s, _ = LeerSobre(sobre(t, "pedido.creado", "x", 1, map[string]any{}), "pedido.creado")
	casos["tipo desconocido"] = s
	s, _ = LeerSobre(sobre(t, "disponibilidad.cambiada", "p1", 1, "texto"), "disponibilidad.cambiada")
	casos["datos con otra forma"] = s
	for nombre, s := range casos {
		if err := a.Procesar(ctx, s); !errors.Is(err, ErrEventoInvalido) {
			t.Errorf("%s: %v", nombre, err)
		}
	}
}
