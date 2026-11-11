package notificaciones

import (
	"bufio"
	"context"
	"mime"
	"net"
	"net/mail"
	"strings"
	"testing"
	"time"
)

// El aviso se lee como un mensaje para una persona: código corto del pedido y estado en palabras.
func TestTextoDelMail(t *testing.T) {
	asunto, cuerpo := TextoDelMail(Pendiente{PedidoID: "2207b999-2f17-4573-8140-f254330a4163", EstadoPedido: "EN_PREPARACION"})
	if asunto != "Tu pedido #2207B999 está en preparación" {
		t.Fatalf("asunto: %q", asunto)
	}
	if !strings.Contains(cuerpo, "#2207B999 está en preparación") || strings.Contains(cuerpo, "EN_PREPARACION") {
		t.Fatalf("cuerpo: %q", cuerpo)
	}
	for _, estado := range []string{"RECIBIDO", "ACEPTADO", "LISTO", "EN_CAMINO", "ENTREGADO", "CANCELADO"} {
		asunto, _ := TextoDelMail(Pendiente{PedidoID: "abc", EstadoPedido: estado})
		if strings.Contains(asunto, estado) || strings.Contains(asunto, "cambió de estado") {
			t.Errorf("%s: el asunto tiene que decirlo en palabras: %q", estado, asunto)
		}
	}
	if asunto, _ := TextoDelMail(Pendiente{PedidoID: "abc", EstadoPedido: "OTRO"}); asunto != "Tu pedido #ABC cambió de estado" {
		t.Errorf("estado desconocido: %q", asunto)
	}
	// La factura se pide al confirmar, así que el mail de entregado no la promete; y el pedido
	// puede ir a otra dirección que la de la cuenta, así que el de listo no dice "tu casa".
	if _, cuerpo := TextoDelMail(Pendiente{PedidoID: "abc", EstadoPedido: "ENTREGADO"}); strings.Contains(cuerpo, "factura") {
		t.Errorf("entregado no tiene que prometer la factura: %q", cuerpo)
	}
	if _, cuerpo := TextoDelMail(Pendiente{PedidoID: "abc", EstadoPedido: "LISTO"}); !strings.Contains(cuerpo, "dirección de entrega") {
		t.Errorf("listo tiene que hablar de la dirección de entrega: %q", cuerpo)
	}
}

// servidorSMTP es un servidor de correo mínimo para el test: acepta un mail y lo devuelve.
func servidorSMTP(t *testing.T) (string, <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	recibido := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r, w := bufio.NewReader(conn), bufio.NewWriter(conn)
		decir := func(s string) { w.WriteString(s + "\r\n"); w.Flush() }
		decir("220 prueba")
		var datos strings.Builder
		for {
			linea, err := r.ReadString('\n')
			if err != nil {
				return
			}
			switch cmd := strings.ToUpper(strings.TrimSpace(linea)); {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				decir("250 prueba")
			case strings.HasPrefix(cmd, "MAIL FROM"), strings.HasPrefix(cmd, "RCPT TO"):
				decir("250 ok")
			case cmd == "DATA":
				decir("354 adelante")
				for {
					l, err := r.ReadString('\n')
					if err != nil || l == ".\r\n" {
						break
					}
					datos.WriteString(l)
				}
				decir("250 recibido")
				recibido <- datos.String()
			case cmd == "QUIT":
				decir("221 chau")
				return
			default:
				decir("250 ok")
			}
		}
	}()
	return ln.Addr().String(), recibido
}

// Enviar manda el aviso por SMTP con el asunto codificado (lleva tildes) y el texto legible.
func TestEnviarMandaElAvisoPorSMTP(t *testing.T) {
	dir, recibido := servidorSMTP(t)
	m := Mailpit{Direccion: dir, Remitente: mail.Address{Name: "GhostKitchen", Address: "pedidos@example.test"}}
	p := Pendiente{PedidoID: "2207b999-2f17", Destinatario: "ana@example.test", EstadoPedido: "EN_CAMINO"}
	if err := m.Enviar(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	var datos string
	select {
	case datos = <-recibido:
	case <-time.After(3 * time.Second):
		t.Fatal("el servidor no recibió el mail")
	}
	msg, err := mail.ReadMessage(strings.NewReader(datos))
	if err != nil {
		t.Fatal(err)
	}
	asunto, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if err != nil || asunto != "Tu pedido #2207B999 está en camino" {
		t.Fatalf("asunto: %q %v", asunto, err)
	}
	if msg.Header.Get("To") != "<ana@example.test>" {
		t.Fatalf("destinatario: %q", msg.Header.Get("To"))
	}
}

// Si el servidor de correo no está, Enviar devuelve el error (el worker reintenta o manda a la DLQ).
func TestEnviarSinServidorDevuelveError(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	dir := ln.Addr().String()
	ln.Close() // nadie escucha en esa dirección
	m := Mailpit{Direccion: dir, Remitente: mail.Address{Address: "pedidos@example.test"}}
	if err := m.Enviar(context.Background(), Pendiente{PedidoID: "x", Destinatario: "ana@example.test", EstadoPedido: "LISTO"}); err == nil {
		t.Fatal("tenía que fallar sin servidor de correo")
	}
}
