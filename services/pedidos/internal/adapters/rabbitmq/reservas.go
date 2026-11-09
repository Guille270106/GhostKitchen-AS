package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"

	"ghostkitchen/pedidos/internal/adapters/correlacion"
	"ghostkitchen/pedidos/internal/application"
	"ghostkitchen/pedidos/internal/domain"
)

// avisoReservaVencida es la parte del sobre de reserva.vencida que usamos
// (docs/contracts/eventos/README.md). Los campos que no conocemos se ignoran.
type avisoReservaVencida struct {
	Tipo          string `json:"tipo"`
	CorrelationID string `json:"correlation_id"`
	Datos         struct {
		ReservaID string `json:"reserva_id"`
		CarritoID string `json:"carrito_id"`
	} `json:"datos"`
}

// ReservaVencida arma el manejador de la cola pedidos.reservas (L14): lee el aviso de Inventario
// y le pide al carrito que se marque vencido. Es un adaptador de entrada, como los controllers:
// traduce el mensaje y llama al caso de uso.
func ReservaVencida(carritos application.CarritoService) func(ctx context.Context, cuerpo []byte) error {
	return func(ctx context.Context, cuerpo []byte) error {
		var aviso avisoReservaVencida
		if err := json.Unmarshal(cuerpo, &aviso); err != nil || aviso.Tipo != "reserva.vencida" {
			return ErrMensajeInvalido
		}
		// El mismo correlation id de la solicitud que causó el vencimiento, para seguirlo en los logs.
		ctx = correlacion.Con(ctx, aviso.CorrelationID)
		err := carritos.ReservaVencida(ctx, aviso.Datos.ReservaID, aviso.Datos.CarritoID)
		if errors.Is(err, domain.ErrDatosInvalidos) {
			return ErrMensajeInvalido
		}
		return err
	}
}
