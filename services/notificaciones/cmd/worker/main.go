package main

import (
	"context"
	"errors"
	"log"
	"net/mail"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ghostkitchen/notificaciones/internal/notificaciones"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	uri, broker, smtp := os.Getenv("MONGO_URI"), os.Getenv("RABBITMQ_URL"), os.Getenv("SMTP_ADDR")
	if uri == "" && os.Getenv("NOTIFICACIONES_DB_PASSWORD") != "" {
		u := url.URL{Scheme: "mongodb", Host: "mongo:27017", Path: "/notificaciones_db",
			User:     url.UserPassword("notificaciones", os.Getenv("NOTIFICACIONES_DB_PASSWORD")),
			RawQuery: "authSource=notificaciones_db&replicaSet=rs0"}
		uri = u.String()
	}
	if broker == "" && os.Getenv("RABBITMQ_USER") != "" {
		u := url.URL{Scheme: "amqp", Host: "rabbitmq:5672", Path: "/",
			User: url.UserPassword(os.Getenv("RABBITMQ_USER"), os.Getenv("RABBITMQ_PASSWORD"))}
		broker = u.String()
	}
	if uri == "" || broker == "" || smtp == "" {
		return errors.New("configurá MONGO_URI, RABBITMQ_URL y SMTP_ADDR")
	}
	db, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return errors.New("no se pudo configurar MongoDB")
	}
	defer func() {
		fin, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		db.Disconnect(fin)
	}()
	nombreDB := os.Getenv("MONGO_DB")
	if nombreDB == "" {
		nombreDB = "notificaciones_db"
	}
	coleccion := db.Database(nombreDB).Collection("notificaciones")
	mailer := notificaciones.Mailpit{Direccion: smtp, Remitente: mail.Address{Name: "GhostKitchen", Address: "pedidos@example.test"}}
	for ctx.Err() == nil {
		err = consumir(ctx, broker, coleccion, mailer)
		if ctx.Err() != nil {
			break
		}
		log.Printf("se interrumpió el consumidor: %v", err)
		if !esperar(ctx, 5*time.Second) {
			break
		}
	}
	return nil
}

func consumir(ctx context.Context, broker string, c *mongo.Collection, mailer notificaciones.Mailpit) error {
	conn, err := amqp.DialConfig(broker, amqp.Config{Dial: amqp.DefaultDial(5 * time.Second)})
	if err != nil {
		return errors.New("RabbitMQ no disponible")
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	if err = ch.Qos(1, 0, false); err != nil {
		return err
	}
	mensajes, err := ch.Consume("notificaciones.mailer", "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case d, ok := <-mensajes:
			if !ok {
				return errors.New("RabbitMQ cerró la entrega")
			}
			if err = procesar(ctx, c, mailer, d); err != nil {
				return err
			}
		}
	}
}

func procesar(ctx context.Context, c *mongo.Collection, mailer notificaciones.Mailpit, d amqp.Delivery) error {
	p, err := notificaciones.LeerEvento(d.Body, d.RoutingKey)
	if err != nil {
		return d.Reject(false)
	}
	registro, cancel := context.WithTimeout(ctx, 5*time.Second)
	err = notificaciones.Registrar(registro, c, p)
	cancel()
	if err != nil {
		return reintentar(ctx, d)
	}
	token := uuid.NewString()
	consulta, cancel := context.WithTimeout(ctx, 5*time.Second)
	p, err = notificaciones.Reclamar(consulta, c, p.EventoID, token)
	cancel()
	if err != nil {
		return reintentar(ctx, d)
	}
	if p.Estado == "ENVIADO" {
		return d.Ack(false)
	}

	envio, cancel := context.WithTimeout(ctx, 10*time.Second)
	err = mailer.Enviar(envio, p)
	cancel()
	persistencia, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if cierre := notificaciones.CerrarIntento(persistencia, c, p.EventoID, token, err == nil); cierre != nil {
		return reintentar(ctx, d)
	}
	if err != nil {
		return reintentar(ctx, d)
	}
	return d.Ack(false)
}

func reintentar(ctx context.Context, d amqp.Delivery) error {
	reintentos, _ := d.Headers["x-delivery-count"].(int64)
	pausas := []time.Duration{5 * time.Second, 30 * time.Second, 2 * time.Minute}
	if reintentos < 0 || reintentos >= int64(len(pausas)) {
		return d.Reject(false)
	}
	if !esperar(ctx, pausas[reintentos]) {
		return ctx.Err()
	}
	return d.Nack(false, true)
}

func esperar(ctx context.Context, pausa time.Duration) bool {
	timer := time.NewTimer(pausa)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
