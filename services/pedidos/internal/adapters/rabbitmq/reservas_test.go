package rabbitmq

import (
	"context"
	"errors"
	"testing"

	"ghostkitchen/pedidos/internal/application"
	"ghostkitchen/pedidos/internal/ports"
)

// CarritosMock anota qué carrito se pidió marcar vencido.
type CarritosMock struct {
	ports.CarritosRepo
	carritoID, reservaID string
	falla                error
}

func (m *CarritosMock) MarcarVencido(_ context.Context, carritoID, reservaID string) (bool, error) {
	m.carritoID, m.reservaID = carritoID, reservaID
	return true, m.falla
}

func TestElAvisoDeReservaVencidaMarcaElCarrito(t *testing.T) {
	repo := &CarritosMock{}
	manejar := ReservaVencida(application.CarritoService{Repo: repo})
	aviso := `{"evento_id":"ev-1","tipo":"reserva.vencida","version":1,"correlation_id":"req-1","agregado_id":"car-77",
 "secuencia":1,"datos":{"reserva_id":"res-381","carrito_id":"car-77","cocina_id":"cerro","campo_nuevo":true}}`
	if err := manejar(context.Background(), []byte(aviso)); err != nil {
		t.Fatal(err)
	}
	if repo.carritoID != "car-77" || repo.reservaID != "res-381" {
		t.Fatal("no marcó el carrito del aviso:", repo.carritoID, repo.reservaID)
	}
}

// Un mensaje roto o de otro tipo no se arregla reintentando: va a la DLQ.
func TestMensajesRotosVanALaDLQ(t *testing.T) {
	manejar := ReservaVencida(application.CarritoService{Repo: &CarritosMock{}})
	for _, cuerpo := range []string{`no es json`, `{"tipo":"plato.creado","datos":{}}`, `{"tipo":"reserva.vencida","datos":{"reserva_id":"","carrito_id":"car-1"}}`} {
		if err := manejar(context.Background(), []byte(cuerpo)); !errors.Is(err, ErrMensajeInvalido) {
			t.Errorf("%s: esperaba ErrMensajeInvalido, obtuvo %v", cuerpo, err)
		}
	}
}

// Si MySQL no contesta, el error vuelve tal cual: el consumidor devuelve el mensaje a la cola.
func TestSiFallaLaBaseElMensajeSeReintenta(t *testing.T) {
	manejar := ReservaVencida(application.CarritoService{Repo: &CarritosMock{falla: errors.New("mysql caído")}})
	err := manejar(context.Background(), []byte(`{"tipo":"reserva.vencida","datos":{"reserva_id":"res-1","carrito_id":"car-1"}}`))
	if err == nil || errors.Is(err, ErrMensajeInvalido) {
		t.Fatal("tiene que ser un error para reintentar:", err)
	}
}

func (r *CarritosMock) OperacionAplicada(context.Context, string, string) (bool, error) {
	return false, nil
}
