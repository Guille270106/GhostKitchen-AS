package application

import (
	"context"
	"errors"
	"testing"

	"ghostkitchen/pedidos/internal/ports"
)

// OutboxMock tiene los mensajes en orden y anota cuáles se marcaron publicados.
type OutboxMock struct {
	mensajes   []ports.Mensaje
	publicados map[string]bool
}

func (m *OutboxMock) Pendientes(_ context.Context, limite int) ([]ports.Mensaje, error) {
	lista := []ports.Mensaje{}
	for _, msg := range m.mensajes {
		if !m.publicados[msg.ID] && len(lista) < limite {
			lista = append(lista, msg)
		}
	}
	return lista, nil
}
func (m *OutboxMock) MarcarPublicado(_ context.Context, id string) error {
	m.publicados[id] = true
	return nil
}

// PublicadorMock es RabbitMQ: anota lo que recibió y puede fallar a partir de cierto mensaje.
type PublicadorMock struct {
	recibidos []string
	fallaEn   string // id del mensaje que no confirma
}

func (p *PublicadorMock) Publicar(_ context.Context, m ports.Mensaje) error {
	if m.ID == p.fallaEn {
		return errors.New("RabbitMQ no confirmó")
	}
	p.recibidos = append(p.recibidos, m.ID)
	return nil
}

func entornoOutbox() (OutboxService, *OutboxMock, *PublicadorMock) {
	repo := &OutboxMock{publicados: map[string]bool{}, mensajes: []ports.Mensaje{
		{ID: "ev-1", Tipo: "pedido.estado_cambiado"}, {ID: "ev-2", Tipo: "pedido.estado_cambiado"}, {ID: "ev-3", Tipo: "pedido.estado_cambiado"},
	}}
	publicador := &PublicadorMock{}
	return OutboxService{Repo: repo, Publicador: publicador}, repo, publicador
}

func TestPublicaElOutboxEnOrdenYMarcaCadaMensaje(t *testing.T) {
	svc, repo, publicador := entornoOutbox()
	n, err := svc.PublicarPendientes(context.Background())
	if err != nil || n != 3 || len(repo.publicados) != 3 {
		t.Fatal(err, n, repo.publicados)
	}
	if publicador.recibidos[0] != "ev-1" || publicador.recibidos[2] != "ev-3" {
		t.Fatal("tienen que salir en el orden en que se guardaron:", publicador.recibidos)
	}
	// La próxima vuelta no manda nada de nuevo.
	if n, _ := svc.PublicarPendientes(context.Background()); n != 0 || len(publicador.recibidos) != 3 {
		t.Fatal("no se publica dos veces:", publicador.recibidos)
	}
}

// Si RabbitMQ no confirma uno, para ahí: no se adelanta un estado posterior del mismo pedido,
// y el que falló (y los siguientes) se mandan en la próxima vuelta.
func TestSiRabbitMQNoConfirmaParaYReintenta(t *testing.T) {
	svc, repo, publicador := entornoOutbox()
	publicador.fallaEn = "ev-2"
	n, err := svc.PublicarPendientes(context.Background())
	if err == nil || n != 1 || !repo.publicados["ev-1"] || repo.publicados["ev-2"] || repo.publicados["ev-3"] {
		t.Fatal(err, n, repo.publicados)
	}

	publicador.fallaEn = ""
	if n, err := svc.PublicarPendientes(context.Background()); err != nil || n != 2 {
		t.Fatal(err, n)
	}
	if len(publicador.recibidos) != 3 || publicador.recibidos[1] != "ev-2" || publicador.recibidos[2] != "ev-3" {
		t.Fatal(publicador.recibidos)
	}
}
