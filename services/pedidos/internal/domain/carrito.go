package domain

import (
	"errors"
	"time"
)

var (
	ErrOtraCocina        = errors.New("el carrito ya tiene platos de otra cocina; vacialo para pedir en otra")
	ErrPlatoNoEncontrado = errors.New("el plato no existe o no está activo")
	ErrPlatoNoDisponible = errors.New("el plato no se puede pedir por ahora")
	ErrSinStock          = errors.New("no alcanzan los ingredientes para esa cantidad")
	ErrCarritoVencido    = errors.New("la reserva del carrito venció; volvé a agregar los platos")
	ErrNoEstaEnElCarrito = errors.New("ese plato no está en el carrito")
	ErrCatalogoCaido     = errors.New("no podemos consultar el menú en este momento")
	ErrInventarioCaido   = errors.New("no podemos reservar los ingredientes en este momento")
	ErrCantidadInvalida  = errors.New("la cantidad tiene que ser un entero entre 1 y 50")
	ErrCarritoModificado = errors.New("el carrito cambió mientras se procesaba; volvé a intentar")
	// ErrOperacionRepetida: esa Idempotency-Key ya se aplicó a este carrito (no se suma de nuevo).
	ErrOperacionRepetida = errors.New("ese cambio del carrito ya se hizo")
)

// MaxCantidadPorPlato evita pedidos absurdos (y reservas enormes) por un error de tipeo.
const MaxCantidadPorPlato int64 = 50

// PlatoCatalogo es lo que Pedidos necesita saber de un plato de Catálogo.
type PlatoCatalogo struct {
	ID            string
	CocinaID      string
	RestauranteID string
	Nombre        string
	Precio        int64
	Activo        bool
}

// ItemCarrito es un plato del carrito. El precio se copia al agregarlo y no cambia aunque
// después cambie en Catálogo: es el precio que queda en el pedido.
type ItemCarrito struct {
	PlatoID        string `json:"platoId"`
	Nombre         string `json:"nombre"`
	RestauranteID  string `json:"restauranteId"`
	PrecioUnitario int64  `json:"precioUnitario"`
	Cantidad       int64  `json:"cantidad"`
	Subtotal       int64  `json:"subtotal"`
}

// Carrito es el carrito activo de un cliente: platos de una sola cocina y la reserva de
// Inventario que aparta sus ingredientes.
type Carrito struct {
	ID        string        `json:"id"`
	ClienteID string        `json:"-"`
	CocinaID  string        `json:"cocinaId,omitempty"`
	ReservaID string        `json:"reservaId,omitempty"`
	VenceEn   *time.Time    `json:"venceEn,omitempty"`
	Items     []ItemCarrito `json:"items"`
	Total     int64         `json:"total"`
	Version   int64         `json:"-"`

	// VencioEn es cuándo se vació el carrito porque venció su reserva (L14). Se borra con el
	// próximo cambio del cliente.
	VencioEn *time.Time `json:"vencioEn,omitempty"`

	// Se calculan al responder (L7): cuánto le queda a la reserva y si ya venció.
	SegundosRestantes int64 `json:"segundosRestantes"`
	Vencido           bool  `json:"vencido"`

	// Operacion es la Idempotency-Key del cambio que se está guardando (si el cliente la mandó).
	// Se guarda junto con el carrito, así la misma clave no se aplica dos veces.
	Operacion           string `json:"-"`
	CambioID            string `json:"-"`
	LiberacionPendiente bool   `json:"-"`
}

// Vacio indica si el carrito no tiene platos.
func (c *Carrito) Vacio() bool { return len(c.Items) == 0 }

// CalcularSegundosRestantes dice cuántos segundos le quedan a la reserva. El plazo es de 10 minutos
// desde el primer plato y agregar más no lo renueva (lo fija Inventario). Sin reserva, o si
// ya venció, devuelve 0.
func (c *Carrito) CalcularSegundosRestantes(ahora time.Time) int64 {
	if c.VenceEn == nil {
		return 0
	}
	restante := c.VenceEn.Sub(ahora)
	if restante <= 0 {
		return 0
	}
	// Redondeamos hacia arriba: con 0,4 s todavía se puede confirmar, así que mostramos 1.
	return int64((restante + time.Second - 1) / time.Second)
}

// EstaVencido indica si el carrito tiene platos y su reserva ya venció.
func (c *Carrito) EstaVencido(ahora time.Time) bool {
	return !c.Vacio() && c.VenceEn != nil && !ahora.Before(*c.VenceEn)
}

// ActualizarTiempo completa los dos campos que se muestran al cliente.
func (c *Carrito) ActualizarTiempo(ahora time.Time) {
	c.SegundosRestantes = c.CalcularSegundosRestantes(ahora)
	c.Vencido = c.EstaVencido(ahora)
}

// Agregar suma un plato o aumenta su cantidad.
func (c *Carrito) Agregar(plato PlatoCatalogo, cantidad int64) error {
	// 1. Validamos el plato y la cantidad.
	if !plato.Activo {
		return ErrPlatoNoEncontrado
	}
	if cantidad <= 0 || cantidad > MaxCantidadPorPlato {
		return ErrCantidadInvalida
	}
	// 2. Todos los platos tienen que ser de la misma cocina (pueden ser de distintos restaurantes).
	if !c.Vacio() && c.CocinaID != plato.CocinaID {
		return ErrOtraCocina
	}
	// 3. Si ya estaba, sumamos la cantidad y conservamos el precio con el que se agregó.
	for i := range c.Items {
		if c.Items[i].PlatoID == plato.ID {
			if c.Items[i].Cantidad+cantidad > MaxCantidadPorPlato {
				return ErrCantidadInvalida
			}
			c.Items[i].Cantidad += cantidad
			c.calcularTotal()
			return nil
		}
	}
	// 4. Si no estaba, lo agregamos con el precio de hoy.
	c.CocinaID = plato.CocinaID
	c.Items = append(c.Items, ItemCarrito{
		PlatoID: plato.ID, Nombre: plato.Nombre, RestauranteID: plato.RestauranteID,
		PrecioUnitario: plato.Precio, Cantidad: cantidad,
	})
	c.calcularTotal()
	return nil
}

// CambiarCantidad deja una cantidad final para un plato que ya está en el carrito.
func (c *Carrito) CambiarCantidad(platoID string, cantidad int64) error {
	if cantidad <= 0 || cantidad > MaxCantidadPorPlato {
		return ErrCantidadInvalida
	}
	for i := range c.Items {
		if c.Items[i].PlatoID == platoID {
			c.Items[i].Cantidad = cantidad
			c.calcularTotal()
			return nil
		}
	}
	return ErrNoEstaEnElCarrito
}

// Quitar saca un plato del carrito.
func (c *Carrito) Quitar(platoID string) error {
	for i := range c.Items {
		if c.Items[i].PlatoID == platoID {
			c.Items = append(c.Items[:i], c.Items[i+1:]...)
			c.calcularTotal()
			return nil
		}
	}
	return ErrNoEstaEnElCarrito
}

// Reiniciar deja el carrito vacío con un id nuevo: la próxima reserva es otra.
func (c *Carrito) Reiniciar(nuevoID string) {
	c.ID = nuevoID
	c.CocinaID = ""
	c.ReservaID = ""
	c.VenceEn = nil
	c.VencioEn = nil
	c.Items = []ItemCarrito{}
	c.Total = 0
}

func (c *Carrito) calcularTotal() {
	c.Total = 0
	for i := range c.Items {
		c.Items[i].Subtotal = c.Items[i].PrecioUnitario * c.Items[i].Cantidad
		c.Total += c.Items[i].Subtotal
	}
}
