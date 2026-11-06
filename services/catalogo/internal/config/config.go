// Package config lee la configuración de Catálogo desde variables de entorno.
// Solo lo usa main: las demás capas reciben los valores ya leídos.
//
// En docker-compose, MONGO_URI lleva un usuario propio de Catálogo que solo puede entrar a
// catalogo_db: un MongoDB compartido, pero una base por servicio (ADR-001, ADR-003).
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config del servicio.
type Config struct {
	Puerto        string        // PORT (8082)
	MongoURI      string        // MONGO_URI (mongodb://mongo:27017; en docker-compose, con usuario y contraseña)
	MongoBase     string        // MONGO_DB (catalogo_db)
	MongoTimeout  time.Duration // MONGO_TIMEOUT_MS (2000): tiempo máximo de cada operación
	SeedAlIniciar bool          // SEED_AL_INICIAR (true)
}

// Cargar lee las variables de entorno con sus valores por defecto.
func Cargar() Config {
	return Config{
		Puerto:        valor("PORT", "8082"),
		MongoURI:      valor("MONGO_URI", "mongodb://mongo:27017"),
		MongoBase:     valor("MONGO_DB", "catalogo_db"),
		MongoTimeout:  time.Duration(entero("MONGO_TIMEOUT_MS", 2000)) * time.Millisecond,
		SeedAlIniciar: valor("SEED_AL_INICIAR", "true") == "true",
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
