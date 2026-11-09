package application

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ghostkitchen/pedidos/internal/adapters/inventario"
	"ghostkitchen/pedidos/internal/domain"
)

// Estos tests usan el adaptador HTTP real de Inventario contra un Inventario falso que tarda en
// contestar. Así se prueba todo junto: el límite de 800 ms de cada intento, los reintentos con
// backoff y el plazo de 3 s de la solicitud.

// InventarioLento es un Inventario falso: mientras lento sea true tarda 1,5 s en contestar
// (más que los 800 ms que espera Pedidos). Anota la clave de cada llamada.
type InventarioLento struct {
	mu     sync.Mutex
	claves []string
	lento  atomic.Bool
}

func (i *InventarioLento) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	i.mu.Lock()
	i.claves = append(i.claves, r.Header.Get("Idempotency-Key"))
	i.mu.Unlock()
	if i.lento.Load() {
		// Espera 1,5 s, o menos si Pedidos ya cortó la llamada.
		select {
		case <-time.After(1500 * time.Millisecond):
		case <-r.Context().Done():
		}
		return
	}
	w.Write([]byte(`{}`))
}

func (i *InventarioLento) llamadas() []string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return append([]string{}, i.claves...)
}

// confirmarConPlazo confirma igual que la API: con el plazo de 3 s de la solicitud.
func confirmarConPlazo(svc ConfirmacionService, clave string) (ResultadoConfirmacion, error, time.Duration) {
	conPlazo, cancelar := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelar()
	inicio := time.Now()
	r, err := svc.Confirmar(conPlazo, clienteConfirma, clave)
	return r, err, time.Since(inicio)
}

func TestInventarioLentoRespondeEstamosConfirmandoAntesDe3Segundos(t *testing.T) {
	svc, confirmaciones, _, carritos, pedidos := entornoConfirmacion(t)
	falso := &InventarioLento{}
	falso.lento.Store(true)
	servidor := httptest.NewServer(falso)
	defer servidor.Close()
	svc.Consumo = inventario.Nuevo(servidor.URL)

	// 1. Inventario no contesta a tiempo: no esperamos de más ni decimos que falló.
	r, err, tardo := confirmarConPlazo(svc, "clave-1")
	if err != nil || r.Pedido != nil || r.Confirmacion.Estado != domain.ConfirmacionEnCurso {
		t.Fatal(err, r)
	}
	if tardo >= 3*time.Second {
		t.Fatal("la respuesta tiene que llegar antes de los 3 s:", tardo)
	}

	// 2. Reintentó, y siempre con la misma clave: si un intento sí llegó, no se descuenta dos veces.
	claves := falso.llamadas()
	if len(claves) < 2 {
		t.Fatal("tiene que reintentar:", claves)
	}
	for _, c := range claves {
		if c != claves[0] {
			t.Fatal("todos los intentos usan la misma clave:", claves)
		}
	}
	if len(pedidos.pedidos) != 0 || len(carritos.carritos["u-cliente"].Items) == 0 {
		t.Fatal("todavía no hay pedido y el carrito sigue igual")
	}

	// 3. Inventario vuelve a contestar rápido: el proceso de recuperación termina la confirmación.
	falso.lento.Store(false)
	if _, err := svc.RecuperarPendientes(context.Background()); err != nil {
		t.Fatal(err)
	}
	conf, _ := confirmaciones.Obtener(context.Background(), r.Confirmacion.ID)
	if conf.Estado != domain.ConfirmacionCompletada || len(pedidos.pedidos) != 1 {
		t.Fatal("la recuperación tiene que completar la confirmación:", conf.Estado, len(pedidos.pedidos))
	}
	if claves := falso.llamadas(); claves[len(claves)-1] != claves[0] {
		t.Fatal("la recuperación usa la misma clave que la solicitud:", claves)
	}
}

func TestInventarioLentoUnaVezSeConfirmaConElReintento(t *testing.T) {
	svc, _, _, carritos, pedidos := entornoConfirmacion(t)
	falso := &InventarioLento{}
	falso.lento.Store(true)
	servidor := httptest.NewServer(falso)
	defer servidor.Close()
	svc.Consumo = inventario.Nuevo(servidor.URL)

	// El primer intento tarda; a los 300 ms Inventario se recupera y el reintento contesta.
	go func() {
		time.Sleep(300 * time.Millisecond)
		falso.lento.Store(false)
	}()

	r, err, tardo := confirmarConPlazo(svc, "clave-1")
	if err != nil || r.Pedido == nil || r.Pedido.Estado != domain.EstadoRecibido {
		t.Fatal("con el reintento se crea el pedido:", err, r)
	}
	if tardo >= 3*time.Second {
		t.Fatal("la respuesta tiene que llegar antes de los 3 s:", tardo)
	}
	if claves := falso.llamadas(); len(claves) != 2 || claves[0] != claves[1] {
		t.Fatal("un intento lento y un reintento, con la misma clave:", claves)
	}
	if c := carritos.carritos["u-cliente"]; len(pedidos.pedidos) != 1 || !c.Vacio() {
		t.Fatal("un solo pedido y el carrito vacío")
	}
}
