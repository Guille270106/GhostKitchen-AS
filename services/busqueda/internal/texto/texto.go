// Package texto analiza texto igual que el tipo text_gk de Solr: separa palabras, pasa a
// minúsculas, quita acentos y reduce cada palabra a su raíz en español.
//
// Lo usa el repositorio en memoria de los tests, para que busque como el Solr real. Si
// buscara distinto, un test podría pasar con el falso y fallar en producción (pasó con los
// plurales: "hamburguesa" no encontraba "Hamburguesas" en Solr, y el test no lo vio).
// Si cambia la definición de text_gk (solr/esquema.go), hay que cambiar esto también.
package texto

import (
	"strings"
	"unicode"
)

// Terminos devuelve las palabras de s ya analizadas, en orden. "Las HAMBURGUESAS de Café"
// queda ["las", "hamburgues", "de", "cafe"].
func Terminos(s string) []string {
	var out []string
	for _, palabra := range strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		out = append(out, Raiz(sinAcentos(strings.ToLower(palabra))))
	}
	return out
}

// sinAcentos hace lo mismo que ASCIIFoldingFilter de Solr con las letras del español.
func sinAcentos(s string) string {
	return strings.NewReplacer(
		"á", "a", "à", "a", "â", "a", "ä", "a",
		"é", "e", "è", "e", "ê", "e", "ë", "e",
		"í", "i", "ì", "i", "î", "i", "ï", "i",
		"ó", "o", "ò", "o", "ô", "o", "ö", "o",
		"ú", "u", "ù", "u", "û", "u", "ü", "u",
		"ñ", "n", "ç", "c",
	).Replace(s)
}

// Raiz es el mismo algoritmo que SpanishLightStemFilter de Solr (el "stemmer liviano" de
// Lucene): las palabras de menos de 5 letras quedan igual; si no, saca la vocal final o el
// plural. "hamburguesas" y "hamburguesa" quedan "hamburgues"; "peces" queda "pez".
func Raiz(p string) string {
	s := []rune(p)
	n := len(s)
	if n < 5 {
		return p
	}
	switch s[n-1] {
	case 'o', 'a', 'e':
		return string(s[:n-1])
	case 's':
		switch {
		case s[n-2] == 'e' && s[n-3] == 's' && s[n-4] == 'e': // "-eses"
			return string(s[:n-2])
		case s[n-2] == 'e' && s[n-3] == 'c': // "-ces" → "-z"
			s[n-3] = 'z'
			return string(s[:n-2])
		case s[n-2] == 'o' || s[n-2] == 'a' || s[n-2] == 'e':
			return string(s[:n-2])
		}
	}
	return p
}

// Contiene dice si todas las palabras de la consulta están en el texto, ya analizadas.
// Es lo que hace Solr con mm=100%: tienen que aparecer todas.
func Contiene(texto, consulta string) bool {
	hay := map[string]bool{}
	for _, t := range Terminos(texto) {
		hay[t] = true
	}
	for _, t := range Terminos(consulta) {
		if !hay[t] {
			return false
		}
	}
	return true
}

// MinPrefijo es el largo mínimo de la última palabra para buscarla también como comienzo de
// palabra. Con menos letras, "ha" encontraría casi todo.
const MinPrefijo = 3

// MinDifuso evita ampliar palabras cortas a términos sin relación.
const MinDifuso = 4

// ContieneConPrefijo exige todas las palabras y conserva prefijos en la última.
// La alternativa difusa compara raíces, igual que la consulta enviada a Solr.
func ContieneConPrefijo(texto, consulta string) bool {
	palabras := strings.FieldsFunc(strings.ToLower(consulta), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	terminos := Terminos(texto)
	for i, palabra := range palabras {
		normal := sinAcentos(palabra)
		raiz := Raiz(normal)
		coincide := false
		for _, t := range terminos {
			if t == raiz || (i == len(palabras)-1 && len([]rune(palabra)) >= MinPrefijo && strings.HasPrefix(t, normal)) ||
				(len([]rune(palabra)) >= MinDifuso && unaEdicion(raiz, t)) {
				coincide = true
				break
			}
		}
		if !coincide {
			return false
		}
	}
	return true
}

// unaEdicion admite una inserción, eliminación, sustitución o transposición adyacente.
func unaEdicion(a, b string) bool {
	x, y := []rune(a), []rune(b)
	if len(x)-len(y) > 1 || len(y)-len(x) > 1 {
		return false
	}
	i := 0
	for i < len(x) && i < len(y) && x[i] == y[i] {
		i++
	}
	if i == len(x) || i == len(y) {
		return true
	}
	if len(x) == len(y) {
		if string(x[i+1:]) == string(y[i+1:]) {
			return true
		}
		return i+1 < len(x) && x[i] == y[i+1] && x[i+1] == y[i] && string(x[i+2:]) == string(y[i+2:])
	}
	if len(x) > len(y) {
		return string(x[i+1:]) == string(y[i:])
	}
	return string(x[i:]) == string(y[i+1:])
}
