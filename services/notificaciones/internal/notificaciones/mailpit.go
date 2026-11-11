package notificaciones

import (
	"context"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// TextoDelMail arma el asunto y el cuerpo del aviso: el pedido con su código corto (los primeros 8
// caracteres, igual que en la pantalla) y el estado en palabras, no el código interno.
func TextoDelMail(p Pendiente) (asunto, cuerpo string) {
	codigo := strings.ToUpper(p.PedidoID)
	if len(codigo) > 8 {
		codigo = codigo[:8]
	}
	estado, detalle := "cambió de estado", ""
	switch p.EstadoPedido {
	case "RECIBIDO":
		estado, detalle = "fue recibido", "La cocina lo está revisando. Mientras no lo acepte, podés cancelarlo."
	case "ACEPTADO":
		estado, detalle = "fue aceptado", "La cocina ya lo tiene en su lista."
	case "EN_PREPARACION":
		estado, detalle = "está en preparación", "La cocina está cocinando tu pedido."
	case "LISTO":
		estado, detalle = "está listo", "En un rato sale para la dirección de entrega."
	case "EN_CAMINO":
		estado, detalle = "está en camino", "Cuando lo recibas, confirmalo en Mis pedidos."
	case "ENTREGADO":
		estado, detalle = "fue entregado", "¡Que lo disfrutes!"
	case "CANCELADO":
		estado, detalle = "fue cancelado", "Los ingredientes que tenía reservados volvieron a la cocina."
	}
	asunto = fmt.Sprintf("Tu pedido #%s %s", codigo, estado)
	cuerpo = fmt.Sprintf("Hola,\r\n\r\nTu pedido #%s %s. %s\r\n\r\nGhostKitchen", codigo, estado, detalle)
	return asunto, cuerpo
}

// Mailpit envía por SMTP a un servidor local de pruebas sin autenticación.
type Mailpit struct {
	Direccion string
	Remitente mail.Address
}

func (m Mailpit) Enviar(ctx context.Context, p Pendiente) error {
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", m.Direccion)
	if err != nil {
		return err
	}
	defer conn.Close()
	detener := context.AfterFunc(ctx, func() { conn.Close() })
	defer detener()
	if err = conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	host, _, err := net.SplitHostPort(m.Direccion)
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer c.Close()
	if err = c.Mail(m.Remitente.Address); err != nil {
		return err
	}
	if err = c.Rcpt(p.Destinatario); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	destinatario := mail.Address{Address: p.Destinatario}
	asunto, cuerpo := TextoDelMail(p)
	// El asunto lleva tildes: va codificado (RFC 2047) para que se vea bien en cualquier correo.
	_, err = fmt.Fprintf(w, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n",
		m.Remitente.String(), destinatario.String(), mime.QEncoding.Encode("utf-8", asunto), cuerpo)
	if err != nil {
		return err
	}
	return w.Close()
}
