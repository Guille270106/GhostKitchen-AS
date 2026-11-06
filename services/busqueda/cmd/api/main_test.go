package main

import (
	"testing"
	"time"
)

func TestEsperaDeArranqueNoSuperaDiezSegundos(t *testing.T) {
	espera := time.Second
	for i, segundos := range []int{1, 2, 4, 8, 10, 10, 10} {
		if espera != time.Duration(segundos)*time.Second {
			t.Fatalf("intento %d: espera %s, esperada %ds", i+1, espera, segundos)
		}
		espera = siguienteEspera(espera)
	}
}
