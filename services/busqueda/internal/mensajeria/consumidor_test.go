package mensajeria

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"ghostkitchen/busqueda/internal/indexer"
)

type procFalso struct{ err error }

func (p procFalso) Procesar(context.Context, indexer.Sobre) error { return p.err }

const cuerpo = `{"evento_id":"0b8e2f8e-1111-4c3a-9a51-2f3c1d2e4f50","tipo":"plato.creado","version":1,
"ocurrido_en":"2026-11-09T12:00:00Z","agregado_id":"p1","secuencia":1,"datos":{}}`

func TestManejarDecideQueContestar(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	casos := []struct {
		nombre string
		cuerpo string
		ruta   string
		err    error
		quiero Accion
	}{
		{"guardado: ack", cuerpo, "plato.creado", nil, Confirmar},
		{"no cumple el sobre: a la DLQ", `{}`, "plato.creado", nil, Rechazar},
		{"la ruta no es el tipo: a la DLQ", cuerpo, "cocina.creada", nil, Rechazar},
		{"datos inválidos: a la DLQ", cuerpo, "plato.creado", fmt.Errorf("x: %w", indexer.ErrEventoInvalido), Rechazar},
		{"Solr caído: reintentar", cuerpo, "plato.creado", errors.New("solr no responde"), Reintentar},
	}
	for _, c := range casos {
		got := NuevoConsumidor("", procFalso{c.err}, log).Manejar(context.Background(), []byte(c.cuerpo), c.ruta)
		if got != c.quiero {
			t.Errorf("%s: %v, esperaba %v", c.nombre, got, c.quiero)
		}
	}
}

func TestEsperaReintentoCrece(t *testing.T) {
	quiero := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 10 * time.Second, 10 * time.Second}
	for i, q := range quiero {
		if got := EsperaReintento(int64(i), 10*time.Second); got != q {
			t.Errorf("entrega %d: %v, esperaba %v", i, got, q)
		}
	}
}
