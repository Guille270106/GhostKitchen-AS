// Package application tiene los casos de uso de Pedidos. Coordina el dominio con los puertos:
// busca los datos, le pide la acción al dominio y guarda el resultado. Las reglas están en domain.
package application

import (
	"context"
	"time"

	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/ports"
)

// Accion es lo que se le pide a un pedido.
type Accion string

const (
	Aceptar          Accion = "aceptar"
	Rechazar         Accion = "rechazar"
	Avanzar          Accion = "avanzar"
	Cancelar         Accion = "cancelar"
	ConfirmarEntrega Accion = "entregado"
)

// PedidosService tiene los casos de uso sobre pedidos ya creados.
type PedidosService struct {
	// Inyectamos la interfaz, no la base de datos real.
	Repo ports.PedidosRepo
}

// Obtener devuelve un pedido que el actor puede ver. Si no puede, responde como si no
// existiera (404), para no revelar pedidos ajenos.
func (s PedidosService) Obtener(ctx context.Context, actor domain.Actor, id string) (domain.Pedido, error) {
	if err := actor.Validar(); err != nil {
		return domain.Pedido{}, err
	}
	pedido, err := s.Repo.Obtener(ctx, id)
	if err != nil {
		return domain.Pedido{}, err
	}
	if !pedido.PuedeVer(actor) {
		return domain.Pedido{}, domain.ErrNoEncontrado
	}
	return pedido, nil
}

// CambiarEstado aplica una acción y la guarda con control de versión. Si dos acciones llegan
// a la vez (el cliente cancela mientras la cocina acepta), la base guarda solo la primera y la
// otra recibe domain.ErrConflictoVersion. No se reintenta sola: con el pedido ya cambiado, la
// acción puede no tener sentido.
func (s PedidosService) CambiarEstado(ctx context.Context, actor domain.Actor, id string, accion Accion) (domain.Pedido, error) {
	if err := actor.Validar(); err != nil {
		return domain.Pedido{}, err
	}

	// 1. Buscamos el pedido.
	pedido, err := s.Repo.Obtener(ctx, id)
	if err != nil {
		return domain.Pedido{}, err
	}

	// 2. Le pedimos la acción: el pedido decide si es válida.
	var cambio domain.CambioEstado
	ahora := time.Now()
	switch accion {
	case Aceptar:
		cambio, err = pedido.Aceptar(actor, ahora)
	case Rechazar:
		cambio, err = pedido.Rechazar(actor, ahora)
	case Avanzar:
		cambio, err = pedido.Avanzar(actor, ahora)
	case Cancelar:
		cambio, err = pedido.Cancelar(actor, ahora)
	case ConfirmarEntrega:
		cambio, err = pedido.ConfirmarEntrega(actor, ahora)
	default:
		return domain.Pedido{}, domain.ErrDatosInvalidos
	}
	if err != nil {
		return domain.Pedido{}, err
	}

	// 3. Guardamos solo si nadie lo cambió mientras tanto.
	if err := s.Repo.GuardarCambio(ctx, pedido, cambio); err != nil {
		return domain.Pedido{}, err
	}
	return pedido, nil
}
