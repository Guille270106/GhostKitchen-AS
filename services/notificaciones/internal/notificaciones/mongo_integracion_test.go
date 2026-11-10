package notificaciones

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Contra un MongoDB real (MONGO_TEST_URI, por ejemplo mongodb://localhost:27017): el mismo evento
// registrado dos veces es un solo documento, mientras un intento lo tiene "reclamado" nadie más
// lo puede mandar, y una vez ENVIADO no se vuelve a mandar. Sin la variable, se saltea.
func TestMongoNoMandaDosVecesElMismoAviso(t *testing.T) {
	uri := os.Getenv("MONGO_TEST_URI")
	if uri == "" {
		t.Skip("sin MONGO_TEST_URI: se saltea el test contra MongoDB real")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cliente, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	defer cliente.Disconnect(context.Background())
	base := cliente.Database("notificaciones_prueba_" + uuid.NewString()[:8])
	defer base.Drop(context.Background())
	c := base.Collection("notificaciones")

	p := Pendiente{EventoID: uuid.NewString(), PedidoID: "ped-1", Destinatario: "ana@example.test",
		EstadoPedido: "ACEPTADO", Estado: "PENDIENTE", CreadoEn: time.Now().UTC(), CorrelationID: "cid-1"}

	// 1. El mismo evento dos veces: un solo documento.
	for i := 0; i < 2; i++ {
		if err := Registrar(ctx, c, p); err != nil {
			t.Fatal(err)
		}
	}
	if n, _ := c.CountDocuments(ctx, bson.M{"_id": p.EventoID}); n != 1 {
		t.Fatalf("documentos = %d", n)
	}

	// 2. Un intento lo reclama; otro intento al mismo tiempo no puede.
	if _, err := Reclamar(ctx, c, p.EventoID, "intento-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := Reclamar(ctx, c, p.EventoID, "intento-b"); !errors.Is(err, ErrOcupado) {
		t.Fatal("un segundo intento no tenía que poder reclamarlo:", err)
	}
	if err := CerrarIntento(ctx, c, p.EventoID, "intento-b", true); !errors.Is(err, ErrOcupado) {
		t.Fatal("solo el que lo reclamó puede cerrarlo:", err)
	}

	// 3. Se manda y se marca ENVIADO; si el evento vuelve, se ve ENVIADO y no se reenvía.
	if err := CerrarIntento(ctx, c, p.EventoID, "intento-a", true); err != nil {
		t.Fatal(err)
	}
	if err := Registrar(ctx, c, p); err != nil {
		t.Fatal(err)
	}
	otra, err := Reclamar(ctx, c, p.EventoID, "intento-c")
	if err != nil || otra.Estado != "ENVIADO" || otra.Intentos != 1 || otra.CorrelationID != "cid-1" {
		t.Fatalf("tenía que verse ENVIADO con un intento: %+v %v", otra, err)
	}
}
