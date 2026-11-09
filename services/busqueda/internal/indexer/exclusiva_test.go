package indexer

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestUnaSolaReconstruccionALaVez(t *testing.T) {
	var enCurso, maximo atomic.Int32
	libre := make(chan struct{})
	e := NuevaExclusiva(func(context.Context) (Resultado, error) {
		n := enCurso.Add(1)
		for {
			m := maximo.Load()
			if n <= m || maximo.CompareAndSwap(m, n) {
				break
			}
		}
		<-libre
		enCurso.Add(-1)
		return Resultado{Sedes: 1}, nil
	})
	ctx := context.Background()
	arranque := make(chan error, 1)
	go func() { _, err := e.Esperar(ctx); arranque <- err }()
	for enCurso.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	// La manual, mientras corre la del arranque: no espera, avisa.
	if _, err := e.Intentar(ctx); !errors.Is(err, ErrEnCurso) {
		t.Fatalf("la manual tenía que dar ErrEnCurso, dio %v", err)
	}
	// Otra del arranque (un reintento) espera su turno.
	segunda := make(chan error, 1)
	go func() { _, err := e.Esperar(ctx); segunda <- err }()
	close(libre) // desde ahora ninguna se traba: si corrieran a la vez, maximo daría 2
	if err := <-arranque; err != nil {
		t.Fatal(err)
	}
	if err := <-segunda; err != nil {
		t.Fatal(err)
	}
	if maximo.Load() != 1 {
		t.Fatalf("corrieron %d a la vez", maximo.Load())
	}
	if res, err := e.Intentar(ctx); err != nil || res.Sedes != 1 {
		t.Fatal("sin otra en curso, la manual corre", err)
	}
}
