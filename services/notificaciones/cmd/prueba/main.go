package main

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	"ghostkitchen/notificaciones/internal/notificaciones"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	uri, broker := os.Getenv("MONGO_URI"), os.Getenv("RABBITMQ_URL")
	if uri == "" || broker == "" {
		return errors.New("configurá MONGO_URI y RABBITMQ_URL de prueba")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return errors.New("no se pudo conectar a MongoDB")
	}
	defer db.Disconnect(ctx)
	if err = db.Ping(ctx, nil); err != nil {
		return errors.New("MongoDB no respondió")
	}
	conn, err := amqp.Dial(broker)
	if err != nil {
		return errors.New("no se pudo conectar a RabbitMQ")
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	// La herramienta de prueba usa su propia cola. V14 completará el flujo
	// del worker y confirmará el mensaje después de guardar ENVIADO.
	d, ok, err := ch.Get("notificaciones.v13.prueba", false)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("la cola de prueba está vacía")
	}
	pendiente, err := notificaciones.LeerEvento(d.Body, d.RoutingKey)
	if err != nil {
		return err
	}
	if err = notificaciones.Registrar(ctx, db.Database("notificaciones_v13_prueba").Collection("notificaciones"), pendiente); err != nil {
		return errors.New("no se pudo registrar el evento")
	}
	log.Printf("registrado PENDIENTE: %s", pendiente.EventoID)
	return d.Nack(false, true)
}
