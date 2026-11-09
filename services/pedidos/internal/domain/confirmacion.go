package domain

import (
	"errors"
	"time"
)

// Confirmar un pedido toca dos servicios (Pedidos e Inventario), así que no se puede hacer en
// una sola transacción de base. Por eso es una SAGA: se hace paso a paso y se anota en qué
// paso va, para poder seguir desde ahí si algo se corta (ADR-004).

// EstadoConfirmacion dice cómo terminó (o si sigue en curso) una confirmación.
type EstadoConfirmacion string

const (
	ConfirmacionEnCurso    EstadoConfirmacion = "EN_CURSO"
	ConfirmacionCompletada EstadoConfirmacion = "COMPLETADA"
	ConfirmacionFallida    EstadoConfirmacion = "FALLIDA"
)

// PasoConfirmacion es el próximo paso que falta hacer.
type PasoConfirmacion string

const (
	// PasoDescontar: pedirle a Inventario que descuente lo reservado.
	PasoDescontar PasoConfirmacion = "DESCONTAR"
	// PasoCrearPedido: Inventario ya descontó; falta crear el pedido en RECIBIDO.
	// (Cuando la cátedra asigne el grupo proveedor, su paso va entre estos dos: L15.)
	PasoCrearPedido PasoConfirmacion = "CREAR_PEDIDO"
)

var (
	ErrCarritoVacio  = errors.New("el carrito está vacío: agregá platos antes de confirmar")
	ErrClaveInvalida = errors.New("falta el header Idempotency-Key (hasta 128 caracteres visibles)")
)

// Confirmacion es una confirmación de pedido en curso o terminada. Guarda una copia del
// carrito, así el pedido se puede crear aunque el carrito cambie o el servicio se reinicie.
type Confirmacion struct {
	ID           string             `json:"id"`
	ClienteID    string             `json:"-"`
	ClienteEmail string             `json:"-"`
	Clave        string             `json:"-"` // la Idempotency-Key que mandó el cliente
	CarritoID    string             `json:"-"`
	ReservaID    string             `json:"-"`
	CocinaID     string             `json:"cocinaId"`
	PedidoID     string             `json:"pedidoId,omitempty"`
	Items        []Item             `json:"-"`
	Estado       EstadoConfirmacion `json:"estado"`
	Paso         PasoConfirmacion   `json:"-"`
	Motivo       string             `json:"motivo,omitempty"`
	CreadaEn     time.Time          `json:"creadaEn"`
}

// NuevaConfirmacion arranca la saga a partir del carrito del cliente.
func NuevaConfirmacion(id, pedidoID string, cliente Actor, clave string, carrito Carrito, ahora time.Time) (Confirmacion, error) {
	// 1. Quién confirma y con qué clave.
	if err := cliente.Validar(); err != nil {
		return Confirmacion{}, err
	}
	if cliente.Rol != RolCliente {
		return Confirmacion{}, ErrSinPermiso
	}
	if !ClaveValida(clave) {
		return Confirmacion{}, ErrClaveInvalida
	}

	// 2. El carrito tiene que tener platos y una reserva que no haya vencido.
	if carrito.Vacio() || carrito.ReservaID == "" {
		return Confirmacion{}, ErrCarritoVacio
	}
	if carrito.EstaVencido(ahora) {
		return Confirmacion{}, ErrCarritoVencido
	}

	// 3. Copiamos los platos del carrito con el precio que tenían al agregarlos.
	items := []Item{}
	for _, it := range carrito.Items {
		items = append(items, Item{
			PlatoID: it.PlatoID, Nombre: it.Nombre, RestauranteID: it.RestauranteID,
			PrecioUnitario: it.PrecioUnitario, Cantidad: it.Cantidad,
		})
	}
	return Confirmacion{
		ID: id, ClienteID: cliente.UsuarioID, ClienteEmail: cliente.Email, Clave: clave,
		CarritoID: carrito.ID, ReservaID: carrito.ReservaID, CocinaID: carrito.CocinaID,
		PedidoID: pedidoID, Items: items, Estado: ConfirmacionEnCurso, Paso: PasoDescontar,
		CreadaEn: ahora.UTC(),
	}, nil
}

// CrearPedido arma el pedido en RECIBIDO con lo que se guardó al confirmar.
func (c Confirmacion) CrearPedido(ahora time.Time) (Pedido, error) {
	cliente := Actor{UsuarioID: c.ClienteID, Rol: RolCliente}
	return NuevoPedido(c.PedidoID, cliente, c.ClienteEmail, c.CocinaID, c.Items, ahora)
}

// ClaveValida: la Idempotency-Key tiene de 1 a 128 caracteres ASCII visibles (igual que Inventario).
func ClaveValida(clave string) bool {
	if len(clave) == 0 || len(clave) > 128 {
		return false
	}
	for _, c := range clave {
		if c < 33 || c > 126 {
			return false
		}
	}
	return true
}
