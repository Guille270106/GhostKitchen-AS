package notificaciones

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestLeerEvento(t *testing.T) {
	cuerpo, err := os.ReadFile("../../evento-prueba.json")
	if err != nil {
		t.Fatal(err)
	}
	p, err := LeerEvento(cuerpo, "pedido.estado_cambiado")
	if err != nil || p.Estado != "PENDIENTE" || p.EstadoPedido != "ACEPTADO" ||
		p.Destinatario != "cliente@example.test" || p.PedidoID != "pedido-prueba-1" {
		t.Fatalf("notificación = %+v, error = %v", p, err)
	}
	invalido := strings.Replace(string(cuerpo), "cliente@example.test", "sin correo", 1)
	if _, err := LeerEvento([]byte(invalido), "pedido.estado_cambiado"); !errors.Is(err, ErrEvento) {
		t.Fatalf("correo inválido: %v", err)
	}
	if _, err := LeerEvento(cuerpo, "plato.creado"); !errors.Is(err, ErrEvento) {
		t.Fatalf("routing key inesperada: %v", err)
	}
}
