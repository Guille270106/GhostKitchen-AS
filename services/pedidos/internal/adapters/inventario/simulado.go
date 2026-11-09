package inventario

import (
	"context"
	"log"
)

// ConsumoSimulado es el Inventario falso para el descuento, hasta que exista la V10
// (POST /reservas/{id}/consumir). Siempre dice que salió bien, así se puede probar la
// confirmación de punta a punta.
//
// Ojo: NO descuenta stock de verdad. La reserva queda activa en Inventario y se libera sola a
// los 10 minutos. Se cambia por el real con CONSUMO_INVENTARIO=real en docker-compose.
type ConsumoSimulado struct{}

// Consumir simula que Inventario descontó la reserva.
func (ConsumoSimulado) Consumir(_ context.Context, _, reservaID, pedidoID string) error {
	log.Printf("consumo SIMULADO: reserva %s del pedido %s (falta la V10 de Inventario)", reservaID, pedidoID)
	return nil
}
