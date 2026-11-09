package application

import (
	"context"
	"errors"
	"time"

	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/ports"
)

// DevolucionService le devuelve a Inventario los ingredientes de los pedidos cancelados.
//
// La devolución quedó anotada en MySQL junto con el cambio a CANCELADO (por eso no se pierde si
// Pedidos se reinicia o Inventario está caído). Este servicio las manda; main lo llama cada 5 s.
type DevolucionService struct {
	Repo       ports.DevolucionesRepo
	Inventario ports.Devolucion
}

// ProcesarPendientes manda las devoluciones pendientes. Usa siempre la MISMA clave por pedido:
// si un intento anterior sí llegó, Inventario no devuelve dos veces.
func (s DevolucionService) ProcesarPendientes(ctx context.Context) (hechas int, err error) {
	pendientes, err := s.Repo.Pendientes(ctx, 20)
	if err != nil {
		return 0, err
	}
	for _, d := range pendientes {
		// 1. Le pedimos a Inventario que devuelva (como mucho 1 s por devolución).
		conPlazo, cancelar := context.WithTimeout(ctx, time.Second)
		err := s.Inventario.Devolver(conPlazo, d.PedidoID+"-devolver", d.PedidoID)
		cancelar()

		// 2. Si Inventario no tiene un descuento de ese pedido, no hay nada que devolver.
		if err == nil || errors.Is(err, domain.ErrNoEncontrado) {
			if err := s.Repo.MarcarHecha(ctx, d.PedidoID); err != nil {
				return hechas, err
			}
			hechas++
			continue
		}

		// 3. No contestó (o algo falló): queda pendiente y se reintenta en la próxima vuelta.
		if err := s.Repo.SumarIntento(ctx, d.PedidoID); err != nil {
			return hechas, err
		}
	}
	return hechas, nil
}
