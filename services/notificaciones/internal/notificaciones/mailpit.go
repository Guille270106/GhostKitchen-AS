package notificaciones

import (
	"context"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"time"
)

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
	_, err = fmt.Fprintf(w, "From: %s\r\nTo: %s\r\nSubject: Estado de tu pedido\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nTu pedido %s cambio de estado a %s.\r\n",
		m.Remitente.String(), destinatario.String(), p.PedidoID, p.EstadoPedido)
	if err != nil {
		return err
	}
	return w.Close()
}
