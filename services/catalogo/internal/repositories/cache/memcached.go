package cache

import (
	"errors"
	"strconv"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
)

// Almacen es lo que el decorator necesita de la caché. La implementación real es Memcached;
// los tests usan una en memoria que puede simular que se cae.
type Almacen interface {
	// Leer devuelve el valor, o ErrNoEsta si la clave no está (o venció).
	Leer(clave string) ([]byte, error)
	// Guardar guarda el valor por ttl.
	Guardar(clave string, valor []byte, ttl time.Duration) error
	// Generacion devuelve la generación actual, y la crea si no existe.
	Generacion() (uint64, error)
	// NuevaGeneracion suma 1 a la generación (invalida todas las claves de antes).
	NuevaGeneracion() error
}

// ErrNoEsta: la clave no está en la caché. No es una falla: es un miss.
var ErrNoEsta = errors.New("no está en la caché")

// ClaveGeneracion es el contador que se suma en cada escritura (ADR-007).
const ClaveGeneracion = "catalogo:v1:gen"

// Memcached implementa Almacen sobre Memcached.
type Memcached struct {
	cli *memcache.Client
}

// TimeoutMemcached es lo máximo que se espera a Memcached (ADR-007): si tarda más, se lee de
// MongoDB. La caché tiene que hacer la lectura más rápida, nunca más lenta.
const TimeoutMemcached = 50 * time.Millisecond

// NuevoMemcached crea el cliente. dir es, por ejemplo, memcached:11211.
func NuevoMemcached(dir string) *Memcached {
	cli := memcache.New(dir)
	cli.Timeout = TimeoutMemcached
	cli.MaxIdleConns = 16
	return &Memcached{cli: cli}
}

func (m *Memcached) Leer(clave string) ([]byte, error) {
	it, err := m.cli.Get(clave)
	if errors.Is(err, memcache.ErrCacheMiss) {
		return nil, ErrNoEsta
	}
	if err != nil {
		return nil, err
	}
	return it.Value, nil
}

func (m *Memcached) Guardar(clave string, valor []byte, ttl time.Duration) error {
	return m.cli.Set(&memcache.Item{Key: clave, Value: valor, Expiration: int32(ttl.Seconds())})
}

// Generacion lee el contador. Si no existe (Memcached recién arrancado o se reinició), lo crea
// con la hora actual en nanosegundos y no con 1: así nunca se repite una generación de antes
// del reinicio, y nadie lee una clave vieja que hubiera quedado.
func (m *Memcached) Generacion() (uint64, error) {
	it, err := m.cli.Get(ClaveGeneracion)
	if errors.Is(err, memcache.ErrCacheMiss) {
		inicial := strconv.FormatInt(time.Now().UnixNano(), 10)
		// Add solo crea la clave si no existe: si dos copias de Catálogo la crean a la vez,
		// gana una y la otra lee la que quedó.
		if err := m.cli.Add(&memcache.Item{Key: ClaveGeneracion, Value: []byte(inicial)}); err != nil && !errors.Is(err, memcache.ErrNotStored) {
			return 0, err
		}
		it, err = m.cli.Get(ClaveGeneracion)
	}
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(string(it.Value), 10, 64)
}

// NuevaGeneracion suma 1 de forma atómica en Memcached (incr), así lo ven las dos copias.
func (m *Memcached) NuevaGeneracion() error {
	_, err := m.cli.Increment(ClaveGeneracion, 1)
	if errors.Is(err, memcache.ErrCacheMiss) {
		_, err = m.Generacion() // no existía: crearla ya es una generación nueva
	}
	return err
}
