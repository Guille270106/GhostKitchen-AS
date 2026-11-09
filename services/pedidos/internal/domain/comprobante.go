package domain

import (
	"errors"
	"fmt"
)

// Estado de la factura del pedido, que emite el grupo proveedor (L15).
type EstadoComprobante string

const (
	ComprobantePendiente EstadoComprobante = "PENDIENTE" // todavía no se pidió o el proveedor no contestó
	ComprobanteEmitido   EstadoComprobante = "EMITIDO"   // el proveedor la emitió y la manda por correo
	ComprobanteRechazado EstadoComprobante = "RECHAZADO" // el proveedor dijo que no: reintentar no lo arregla
)

var (
	// ErrProveedorCaido: el proveedor no contestó a tiempo o está caído. No sabemos si emitió:
	// se reintenta con la misma clave, así no hay dos facturas.
	ErrProveedorCaido = errors.New("el proveedor de comprobantes no está disponible")
	// ErrProveedorRechazo: el proveedor rechazó la solicitud (datos inválidos, regla de ellos).
	ErrProveedorRechazo = errors.New("el proveedor de comprobantes rechazó la solicitud")
)

// Comprobante es cómo va la factura de un pedido. Se muestra en el detalle del pedido.
type Comprobante struct {
	Estado EstadoComprobante `json:"estado"`
	Numero string            `json:"numero,omitempty"`
}

// LineaComprobante es una línea de la factura: qué se compró y cuánto se pagó por esa línea.
type LineaComprobante struct {
	Descripcion string
	Importe     int64 // en pesos, ya multiplicado por la cantidad
}

// LineasDeComprobante arma las líneas de la factura con los platos del pedido, con el precio que
// se copió al confirmar (no el de Catálogo de hoy).
func (p Pedido) LineasDeComprobante() []LineaComprobante {
	lineas := make([]LineaComprobante, 0, len(p.Items))
	for _, it := range p.Items {
		lineas = append(lineas, LineaComprobante{
			Descripcion: fmt.Sprintf("%s x%d", it.Nombre, it.Cantidad),
			Importe:     it.Subtotal(),
		})
	}
	return lineas
}
