package indexer

import (
	"context"
	"errors"
	"sync"
)

// ErrEnCurso: se pidió una reconstrucción manual mientras otra (la del arranque u otra
// manual) todavía corre.
var ErrEnCurso = errors.New("ya hay una reconstrucción en curso")

// Exclusiva hace que haya una sola reconstrucción a la vez, venga del arranque o de
// POST /internal/reindexar (G22). Dos en paralelo duplicarían la carga sobre Catálogo,
// Inventario y Solr sin ninguna ventaja (bulkhead, ADR-010).
type Exclusiva struct {
	mu          sync.Mutex
	reconstruir func(context.Context) (Resultado, error)
}

// NuevaExclusiva envuelve la reconstrucción.
func NuevaExclusiva(reconstruir func(context.Context) (Resultado, error)) *Exclusiva {
	return &Exclusiva{reconstruir: reconstruir}
}

// Esperar reconstruye; si hay otra en curso, espera a que termine. La usa el arranque, que
// necesita el índice armado para estar listo.
func (e *Exclusiva) Esperar(ctx context.Context) (Resultado, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.reconstruir(ctx)
}

// Intentar reconstruye solo si no hay otra en curso; si la hay, devuelve ErrEnCurso enseguida.
// La usa la reconstrucción manual: quien la pidió prefiere saberlo a quedarse esperando.
func (e *Exclusiva) Intentar(ctx context.Context) (Resultado, error) {
	if !e.mu.TryLock() {
		return Resultado{}, ErrEnCurso
	}
	defer e.mu.Unlock()
	return e.reconstruir(ctx)
}
