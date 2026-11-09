package cache

// Test contra un Memcached real. Se saltea sin MEMCACHED_TEST_ADDR (ej. memcached:11211).

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestIntegracionMemcached(t *testing.T) {
	dir := os.Getenv("MEMCACHED_TEST_ADDR")
	if dir == "" {
		t.Skip("sin MEMCACHED_TEST_ADDR: se saltea el test contra Memcached real")
	}
	m := NuevoMemcached(dir)
	g1, err := m.Generacion()
	if err != nil || g1 == 0 {
		t.Fatalf("generación: %d %v", g1, err)
	}
	if err := m.NuevaGeneracion(); err != nil {
		t.Fatal(err)
	}
	if g2, _ := m.Generacion(); g2 != g1+1 {
		t.Fatalf("incr: %d → %d", g1, g2)
	}
	clave := "catalogo:v1:prueba:" + time.Now().Format("150405.000")
	if _, err := m.Leer(clave); !errors.Is(err, ErrNoEsta) {
		t.Fatalf("clave nueva: %v", err)
	}
	if err := m.Guardar(clave, []byte(`{"ok":true}`), 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if b, err := m.Leer(clave); err != nil || string(b) != `{"ok":true}` {
		t.Fatalf("leer: %s %v", b, err)
	}
	// Con Memcached apagado, el timeout es corto (50 ms): la lectura no se cuelga.
	caido := NuevoMemcached("127.0.0.1:1")
	inicio := time.Now()
	if _, err := caido.Generacion(); err == nil || time.Since(inicio) > time.Second {
		t.Fatalf("apagado: %v en %v", err, time.Since(inicio))
	}
}
