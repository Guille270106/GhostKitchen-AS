package inventario

import (
	"context"
	"log"
)

// ConsumoSimulado es el Inventario falso para el descuento: se usa con CONSUMO_INVENTARIO=simulado,
// para probar Pedidos sin Inventario. Siempre dice que salió bien.
//
// Ojo: NO descuenta stock de verdad. La reserva queda activa en Inventario y se libera sola a
// los 10 minutos. docker-compose usa el real (CONSUMO_INVENTARIO=real).
type ConsumoSimulado struct{}

// Consumir simula que Inventario descontó la reserva.
func (ConsumoSimulado) Consumir(_ context.Context, _, reservaID, pedidoID string) error {
	log.Printf("consumo SIMULADO: reserva %s del pedido %s", reservaID, pedidoID)
	return nil
}

// DevolucionSimulada es el Inventario falso para devolver ingredientes al cancelar. Se usa
// mientras CONSUMO_INVENTARIO=simulado (porque con el descuento simulado no hay nada que
// devolver de verdad). Siempre dice que salió bien.
type DevolucionSimulada struct{}

// Devolver simula que Inventario devolvió los ingredientes.
func (DevolucionSimulada) Devolver(_ context.Context, _, pedidoID string) error {
	log.Printf("devolución SIMULADA del pedido %s", pedidoID)
	return nil
}
