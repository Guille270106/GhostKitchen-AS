package texto

import (
	"reflect"
	"testing"
)

func TestTerminos(t *testing.T) {
	got := Terminos("Las HAMBURGUESAS de Café, ñoquis")
	want := []string{"las", "hamburgues", "de", "cafe", "noquis"} // "-is" no se recorta, igual que en Solr
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestRaiz(t *testing.T) {
	casos := map[string]string{
		"hamburguesa":  "hamburgues",
		"hamburguesas": "hamburgues",
		"milanesas":    "milanes",
		"peces":        "pez",
		"meses":        "mes", // "-eses" saca solo "es"
		"pollo":        "poll",
		"cafe":         "cafe",  // menos de 5 letras: queda igual
		"sushi":        "sushi", // no termina en vocal que se saque
	}
	for palabra, raiz := range casos {
		if got := Raiz(palabra); got != raiz {
			t.Errorf("Raiz(%q) = %q, esperaba %q", palabra, got, raiz)
		}
	}
}

func TestContiene(t *testing.T) {
	texto := "Mega doble · Dos medallones de carne · Hamburguesas · Mostaza"
	casos := map[string]bool{
		"hamburguesa":     true,  // singular encuentra plural
		"HAMBURGUESAS":    true,  // mayúsculas
		"medallón carne":  true,  // todas las palabras, con acento
		"hamburguesa pez": false, // falta una palabra (mm=100%)
		"burg":            false, // un pedazo de palabra no alcanza, igual que en Solr
	}
	for consulta, esperado := range casos {
		if got := Contiene(texto, consulta); got != esperado {
			t.Errorf("Contiene(%q) = %v, esperaba %v", consulta, got, esperado)
		}
	}
}

func TestContieneConPrefijo(t *testing.T) {
	plato := "Hamburguesa doble con queso"
	casos := map[string]bool{
		"hambur":           true, // la última palabra, como comienzo
		"HAMBÚR":           true, // sin mayúsculas ni acentos
		"hamburguesas":     true, // la palabra entera, por su raíz
		"doble hambur":     true,
		"hambur doble":     false, // solo la última puede estar a medias
		"doble queso hamb": true,
		"hamb doble":       false, // solo la última puede estar a medias
		"ha":               false, // menos de 3 letras: no cuenta como comienzo
		"pizza":            false,
		"":                 true,
	}
	for consulta, quiero := range casos {
		if got := ContieneConPrefijo(plato, consulta); got != quiero {
			t.Errorf("%q: %v, esperaba %v", consulta, got, quiero)
		}
	}
}
