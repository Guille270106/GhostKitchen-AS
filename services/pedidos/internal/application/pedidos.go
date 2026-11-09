// Package application tiene las acciones de Pedidos. Coordina el agregado con los puertos:
// carga el pedido, le pide la acción y guarda el resultado. Las reglas están en domain.
package application

import (
	"context"
	"time"

	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/ports"
)

// Accion es lo que se le pide al pedido.
type Accion string

const (
	Aceptar          Accion = "aceptar"
	Rechazar         Accion = "rechazar"
	Avanzar          Accion = "avanzar"
	Cancelar         Accion = "cancelar"
	ConfirmarEntrega Accion = "entregado"
)

// Pedidos reúne las acciones sobre pedidos ya creados.
type Pedidos struct {
	repo  ports.Pedidos
	reloj func() time.Time
}

// Nuevo crea el servicio con el reloj real.
func Nuevo(repo ports.Pedidos) *Pedidos { return &Pedidos{repo: repo, reloj: time.Now} }

// Obtener devuelve un pedido que el actor puede ver. Si no puede, responde como si no
// existiera (404), para no revelar pedidos ajenos.
func (s *Pedidos) Obtener(ctx context.Context, a domain.Actor, id string) (domain.Pedido, error) {
	if err := a.Validar(); err != nil {
		return domain.Pedido{}, err
	}
	p, err := s.repo.Obtener(ctx, id)
	if err != nil {
		return domain.Pedido{}, err
	}
	if !p.PuedeVer(a) {
		return domain.Pedido{}, domain.ErrNoEncontrado
	}
	return p, nil
}

// CambiarEstado aplica una acción y la guarda con control de versión. Si dos acciones llegan
// a la vez (por ejemplo, el cliente cancela mientras la cocina acepta), la base guarda solo la
// primera y la otra recibe domain.ErrConflictoVersion. No se reintenta sola: con el pedido ya
// cambiado, la acción puede no tener sentido.
func (s *Pedidos) CambiarEstado(ctx context.Context, a domain.Actor, id string, accion Accion) (domain.Pedido, error) {
	if err := a.Validar(); err != nil {
		return domain.Pedido{}, err
	}
	p, err := s.repo.Obtener(ctx, id)
	if err != nil {
		return domain.Pedido{}, err
	}
	var c domain.CambioEstado
	ahora := s.reloj()
	switch accion {
	case Aceptar:
		c, err = p.Aceptar(a, ahora)
	case Rechazar:
		c, err = p.Rechazar(a, ahora)
	case Avanzar:
		c, err = p.Avanzar(a, ahora)
	case Cancelar:
		c, err = p.Cancelar(a, ahora)
	case ConfirmarEntrega:
		c, err = p.ConfirmarEntrega(a, ahora)
	default:
		return domain.Pedido{}, domain.ErrDatosInvalidos
	}
	if err != nil {
		return domain.Pedido{}, err
	}
	if err := s.repo.GuardarCambio(ctx, p, c); err != nil {
		return domain.Pedido{}, err
	}
	return p, nil
}
