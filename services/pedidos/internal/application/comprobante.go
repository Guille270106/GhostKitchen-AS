package application

import (
	"context"
	"errors"
	"time"

	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/ports"
)

// ComprobanteService le pide al grupo proveedor la factura de cada pedido confirmado.
//
// La factura NO es un paso esencial (ADR-009): el pedido ya está confirmado aunque el proveedor
// esté caído. Por eso no se pide durante la confirmación (que tiene 3 s) sino después, en
// segundo plano: la confirmación anota la factura PENDIENTE y main llama a este servicio cada 10 s.
type ComprobanteService struct {
	Repo      ports.ComprobantesRepo
	Pedidos   ports.PedidosRepo
	Proveedor ports.Proveedor
}

// EmitirPendientes pide las facturas pendientes. Usa siempre la misma clave por pedido
// ("pedido-{id}"): si un intento anterior sí llegó, el proveedor no emite otra.
func (s ComprobanteService) EmitirPendientes(ctx context.Context) (emitidos int, err error) {
	pendientes, err := s.Repo.Pendientes(ctx, 20)
	if err != nil {
		return 0, err
	}
	for _, pedidoID := range pendientes {
		// 1. Los datos salen del pedido: el mail del cliente y los platos con su precio.
		pedido, err := s.Pedidos.Obtener(ctx, pedidoID)
		if err != nil {
			return emitidos, err
		}
		solicitud := ports.SolicitudComprobante{Email: pedido.ClienteEmail, Lineas: pedido.LineasDeComprobante()}

		// 2. Se la pedimos al proveedor (como mucho 5 s, lo que ellos recomiendan).
		conPlazo, cancelar := context.WithTimeout(ctx, 5*time.Second)
		emitido, err := s.Proveedor.Emitir(conPlazo, "pedido-"+pedidoID, solicitud)
		cancelar()

		// 3. Según cómo salió: emitida, rechazada (no se reintenta) o queda pendiente.
		switch {
		case err == nil:
			if err := s.Repo.MarcarEmitido(ctx, pedidoID, emitido); err != nil {
				return emitidos, err
			}
			emitidos++
		case errors.Is(err, domain.ErrProveedorRechazo):
			if err := s.Repo.MarcarRechazado(ctx, pedidoID, err.Error()); err != nil {
				return emitidos, err
			}
		default:
			if err := s.Repo.SumarIntento(ctx, pedidoID); err != nil {
				return emitidos, err
			}
		}
	}
	return emitidos, nil
}
