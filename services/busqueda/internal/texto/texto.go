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

// ContieneConPrefijo es Contiene, pero la última palabra de la consulta también vale si es el
// comienzo de una palabra del texto: "hambur" encuentra "Hamburguesa". Es lo que hace Solr con
// la consulta que arma ParametrosBusqueda ("hambur OR hambur*"): el comodín se compara con
// las palabras ya reducidas a su raíz, sin quitarle nada a lo escrito.
func ContieneConPrefijo(texto, consulta string) bool {
	palabras := strings.FieldsFunc(consulta, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(palabras) == 0 {
		return true
	}
	ultima := sinAcentos(strings.ToLower(palabras[len(palabras)-1]))
	if !Contiene(texto, strings.Join(palabras[:len(palabras)-1], " ")) {
		return false
	}
	raiz := Raiz(ultima)
	for _, t := range Terminos(texto) {
		if t == raiz || (len([]rune(ultima)) >= MinPrefijo && strings.HasPrefix(t, ultima)) {
			return true
		}
	}
	return false
}
