package domain

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Solo una cuenta Cocina, con su cocina asignada, administra el inventario. El admin no.
func TestIdentidadSoloCocinaConSuCocina(t *testing.T) {
	casos := []struct {
		que  string
		id   Identidad
		want error
	}{
		{"cocina con su cocina", Identidad{UsuarioID: "u1", Rol: "cocina", CocinaID: "cerro"}, nil},
		{"sin usuario", Identidad{Rol: "cocina", CocinaID: "cerro"}, ErrSinIdentidad},
		{"sin rol", Identidad{UsuarioID: "u1", CocinaID: "cerro"}, ErrSinIdentidad},
		{"admin", Identidad{UsuarioID: "u1", Rol: "admin", CocinaID: "cerro"}, ErrSinPermiso},
		{"cocina sin cocina asignada", Identidad{UsuarioID: "u1", Rol: "cocina"}, ErrSinPermiso},
	}
	for _, c := range casos {
		if err := c.id.Validar(); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, quería %v", c.que, err, c.want)
		}
	}
}

// Un ingrediente nace con todo su stock disponible y nada reservado; los datos inválidos se rechazan.
func TestNuevoIngrediente(t *testing.T) {
	i, err := NuevoIngrediente("id-1", "cerro", "  Pan de hamburguesa ", " unidad ", 20)
	if err != nil {
		t.Fatal(err)
	}
	if i.Nombre != "Pan de hamburguesa" || i.Unidad != "unidad" || i.Total != 20 || i.Disponible != 20 || i.Reservado != 0 {
		t.Fatalf("%+v", i)
	}
	if i, err := NuevoIngrediente("id-2", "cerro", "Sal", "g", 0); err != nil || i.Total != 0 {
		t.Fatal("stock inicial 0 es válido:", err)
	}
	invalidos := []struct {
		que                        string
		id, cocina, nombre, unidad string
		inicial                    int64
	}{
		{"sin id", "", "cerro", "Pan", "unidad", 1},
		{"sin cocina", "id", "", "Pan", "unidad", 1},
		{"nombre vacío", "id", "cerro", "   ", "unidad", 1},
		{"unidad vacía", "id", "cerro", "Pan", "", 1},
		{"stock negativo", "id", "cerro", "Pan", "unidad", -1},
		{"nombre de más de 120", "id", "cerro", strings.Repeat("a", 121), "unidad", 1},
		{"unidad de más de 30", "id", "cerro", "Pan", strings.Repeat("u", 31), 1},
	}
	for _, c := range invalidos {
		if _, err := NuevoIngrediente(c.id, c.cocina, c.nombre, c.unidad, c.inicial); !errors.Is(err, ErrDatosInvalidos) {
			t.Errorf("%s: tenía que rechazarse, dio %v", c.que, err)
		}
	}
}

// Una receta tiene ingredientes distintos con cantidades positivas, y se guarda ordenada.
func TestNuevaReceta(t *testing.T) {
	r, err := NuevaReceta("p1", "cerro", []RecetaItem{{IngredienteID: "pan", Cantidad: 1}, {IngredienteID: "carne", Cantidad: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if r.PlatoID != "p1" || r.CocinaID != "cerro" || r.Items[0].IngredienteID != "carne" || r.Items[1].IngredienteID != "pan" {
		t.Fatalf("la receta tiene que quedar ordenada por ingrediente: %+v", r)
	}
	invalidas := map[string][]RecetaItem{
		"sin ingredientes":     {},
		"ingrediente repetido": {{IngredienteID: "pan", Cantidad: 1}, {IngredienteID: "pan", Cantidad: 2}},
		"cantidad cero":        {{IngredienteID: "pan", Cantidad: 0}},
		"cantidad negativa":    {{IngredienteID: "pan", Cantidad: -1}},
		"ingrediente sin id":   {{IngredienteID: "", Cantidad: 1}},
	}
	for que, items := range invalidas {
		if _, err := NuevaReceta("p1", "cerro", items); !errors.Is(err, ErrRecetaInvalida) {
			t.Errorf("%s: tenía que rechazarse, dio %v", que, err)
		}
	}
	if _, err := NuevaReceta("", "cerro", []RecetaItem{{IngredienteID: "pan", Cantidad: 1}}); !errors.Is(err, ErrRecetaInvalida) {
		t.Error("sin plato tenía que rechazarse")
	}
	if _, err := NuevaReceta("p1", "", []RecetaItem{{IngredienteID: "pan", Cantidad: 1}}); !errors.Is(err, ErrRecetaInvalida) {
		t.Error("sin cocina tenía que rechazarse")
	}
}

// Los identificadores solo aceptan letras, números, guiones y guiones bajos, hasta 64.
func TestIdentificadorValido(t *testing.T) {
	for _, s := range []string{"p1", "nueva-cordoba", "car_rito-1", strings.Repeat("a", 64)} {
		if !IdentificadorValido(s) {
			t.Errorf("%q tenía que ser válido", s)
		}
	}
	for _, s := range []string{"", strings.Repeat("a", 65), "con espacio", "p1;DROP", "ñandú", "a/b"} {
		if IdentificadorValido(s) {
			t.Errorf("%q tenía que ser inválido", s)
		}
	}
}

// La Idempotency-Key tiene de 1 a 128 caracteres ASCII visibles.
func TestValidarClave(t *testing.T) {
	for _, c := range []string{"a", "clave-1:2.3", strings.Repeat("x", 128)} {
		if err := ValidarClave(c); err != nil {
			t.Errorf("%q tenía que ser válida: %v", c, err)
		}
	}
	for _, c := range []string{"", strings.Repeat("x", 129), "con espacio", "tab\tadentro", "eñe"} {
		if err := ValidarClave(c); !errors.Is(err, ErrClaveInvalida) {
			t.Errorf("%q tenía que ser inválida", c)
		}
	}
}

// Los platos de una reserva se ordenan (así el mismo pedido da la misma huella) y no se repiten.
func TestNormalizarItems(t *testing.T) {
	items, err := NormalizarItems([]PlatoSolicitado{{PlatoID: "p2", Cantidad: 1}, {PlatoID: "p1", Cantidad: 3}}, false)
	if err != nil || items[0].PlatoID != "p1" || items[1].PlatoID != "p2" {
		t.Fatalf("ordenados por plato: %+v %v", items, err)
	}
	if _, err := NormalizarItems(nil, false); !errors.Is(err, ErrReservaInvalida) {
		t.Error("una reserva nueva sin platos tenía que rechazarse")
	}
	if items, err := NormalizarItems(nil, true); err != nil || len(items) != 0 {
		t.Error("un ajuste a vacío (liberar todo) es válido:", err)
	}
	invalidos := map[string][]PlatoSolicitado{
		"plato repetido":    {{PlatoID: "p1", Cantidad: 1}, {PlatoID: "p1", Cantidad: 2}},
		"cantidad cero":     {{PlatoID: "p1", Cantidad: 0}},
		"cantidad negativa": {{PlatoID: "p1", Cantidad: -2}},
		"id inválido":       {{PlatoID: "p 1", Cantidad: 1}},
	}
	for que, items := range invalidos {
		if _, err := NormalizarItems(items, false); !errors.Is(err, ErrReservaInvalida) {
			t.Errorf("%s: tenía que rechazarse, dio %v", que, err)
		}
	}
}

func TestCorrelacionViajaEnElContexto(t *testing.T) {
	ctx := ConCorrelacion(context.Background(), "cid-123")
	if Correlacion(ctx) != "cid-123" || Correlacion(context.Background()) != "" {
		t.Fatal("el correlation id tiene que viajar en el contexto")
	}
}

// Los eventos de Catálogo que mantienen la copia local de cocinas y platos.
func TestLeerReferencia(t *testing.T) {
	const id = "9b2f3c3e-6a7b-4f1e-9a8d-2c1b0a9e8f7d"
	cocina := `{"evento_id":"` + id + `","tipo":"cocina.actualizada","version":1,"correlation_id":"c1","agregado_id":"cerro","secuencia":3,"datos":{"id":"cerro","activa":false,"version":3}}`
	ref, err := LeerReferencia([]byte(cocina), "cocina.actualizada")
	if err != nil || !ref.EsCocina || ref.CocinaID != "cerro" || ref.Activa || ref.Version != 3 || ref.Correlacion != "c1" {
		t.Fatalf("cocina: %+v %v", ref, err)
	}
	plato := `{"evento_id":"` + id + `","tipo":"plato.creado","version":1,"agregado_id":"p99","secuencia":1,"datos":{"id":"p99","cocinaId":"cerro","activo":true,"version":1}}`
	ref, err = LeerReferencia([]byte(plato), "plato.creado")
	if err != nil || ref.EsCocina || ref.ID != "p99" || ref.CocinaID != "cerro" || !ref.Activa {
		t.Fatalf("plato: %+v %v", ref, err)
	}
	invalidos := map[string]struct{ cuerpo, ruta string }{
		"JSON roto":                        {"{", "plato.creado"},
		"la ruta no es el tipo":            {plato, "plato.actualizado"},
		"evento_id que no es un UUID":      {strings.Replace(plato, id, "no-uuid", 1), "plato.creado"},
		"secuencia distinta de la versión": {strings.Replace(plato, `"secuencia":1`, `"secuencia":2`, 1), "plato.creado"},
		"plato sin activo":                 {strings.Replace(plato, `"activo":true,`, "", 1), "plato.creado"},
		"plato de cocina inválida":         {strings.Replace(plato, `"cocinaId":"cerro"`, `"cocinaId":"a b"`, 1), "plato.creado"},
		"cocina sin activa":                {strings.Replace(cocina, `"activa":false,`, "", 1), "cocina.actualizada"},
		"tipo desconocido":                 {strings.Replace(plato, "plato.creado", "plato.borrado", 1), "plato.borrado"},
		"versión del sobre distinta":       {strings.Replace(plato, `"version":1,"agregado_id"`, `"version":2,"agregado_id"`, 1), "plato.creado"},
	}
	for que, c := range invalidos {
		if _, err := LeerReferencia([]byte(c.cuerpo), c.ruta); !errors.Is(err, ErrEventoInvalido) {
			t.Errorf("%s: tenía que rechazarse, dio %v", que, err)
		}
	}
}
