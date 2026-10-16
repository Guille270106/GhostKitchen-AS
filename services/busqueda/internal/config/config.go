// Package config lee la configuración de Búsqueda desde variables de entorno.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config del servicio.
type Config struct {
	Puerto        string        // PORT (8083)
	SolrURL       string        // SOLR_URL (http://solr:8983/solr)
	SolrTimeout   time.Duration // SOLR_TIMEOUT_MS (2000)
	APIKeys       []string      // PUBLIC_API_KEYS, separadas por coma. Vacío = nadie entra.
	SeedAlIniciar bool          // SEED_AL_INICIAR (true): solo si no hay CATALOGO_URL
	CatalogoURL   string        // CATALOGO_URL (http://catalogo:8082). Si está, el índice se arma desde Catálogo
}

// Cargar lee las variables de entorno con sus valores por defecto.
func Cargar() Config {
	return Config{
		Puerto:        valor("PORT", "8083"),
		SolrURL:       valor("SOLR_URL", "http://solr:8983/solr"),
		SolrTimeout:   time.Duration(entero("SOLR_TIMEOUT_MS", 2000)) * time.Millisecond,
		APIKeys:       lista(os.Getenv("PUBLIC_API_KEYS")),
		SeedAlIniciar: valor("SEED_AL_INICIAR", "true") == "true",
		CatalogoURL:   strings.TrimSpace(os.Getenv("CATALOGO_URL")),
	}
}

func valor(clave, porDefecto string) string {
	if v := strings.TrimSpace(os.Getenv(clave)); v != "" {
		return v
	}
	return porDefecto
}

func entero(clave string, porDefecto int) int {
	if n, err := strconv.Atoi(os.Getenv(clave)); err == nil && n > 0 {
		return n
	}
	return porDefecto
}

func lista(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
