package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"ghostkitchen/notificaciones/internal/metricas"
	"ghostkitchen/notificaciones/internal/notificaciones"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// registro escribe una línea JSON por mensaje, con el evento, el pedido y el correlation_id,
// igual que los logs de los demás servicios (ADR-011).
// escuchando dice si el worker está conectado a la cola (lo usa /ready).
var escuchando atomic.Bool

var registro = slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("servicio", "notificaciones")

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
	go servirSalud(ctx, db)
	for ctx.Err() == nil {
		err = consumir(ctx, broker, coleccion, mailer)
		if ctx.Err() != nil {
			break
		}
		registro.Warn("se interrumpió el consumidor; reconecta en 5 s", "error", err.Error())
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
	metricas.Conectado(true)
	escuchando.Store(true)
	defer func() { metricas.Conectado(false); escuchando.Store(false) }()
	registro.Info("escuchando la cola notificaciones.mailer")
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
		metricas.Mensaje(metricas.Invalido)
		registro.Warn("mensaje inválido: va a la DLQ", "resultado", metricas.Invalido)
		return d.Reject(false)
	}
	reg := registro.With("evento_id", p.EventoID, "pedido_id", p.PedidoID, "correlation_id", p.CorrelationID)
	registro, cancel := context.WithTimeout(ctx, 5*time.Second)
	err = notificaciones.Registrar(registro, c, p)
	cancel()
	if err != nil {
		return reintentar(ctx, d, reg, "no se pudo registrar el aviso")
	}
	token := uuid.NewString()
	consulta, cancel := context.WithTimeout(ctx, 5*time.Second)
	correlacion := p.CorrelationID
	p, err = notificaciones.Reclamar(consulta, c, p.EventoID, token)
	cancel()
	if err != nil {
		return reintentar(ctx, d, reg, "no se pudo reclamar el aviso")
	}
	p.CorrelationID = correlacion
	if p.Estado == "ENVIADO" {
		metricas.Mensaje(metricas.Duplicado)
		reg.Info("el aviso ya se había mandado: no se reenvía", "resultado", metricas.Duplicado)
		return d.Ack(false)
	}

	envio, cancel := context.WithTimeout(ctx, 10*time.Second)
	inicio := time.Now()
	err = mailer.Enviar(envio, p)
	metricas.Envio(inicio)
	cancel()
	persistencia, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if cierre := notificaciones.CerrarIntento(persistencia, c, p.EventoID, token, err == nil); cierre != nil {
		return reintentar(ctx, d, reg, "no se pudo guardar el resultado del envío")
	}
	if err != nil {
		return reintentar(ctx, d, reg, "falló el envío del mail")
	}
	metricas.Mensaje(metricas.Enviado)
	reg.Info("mail enviado", "resultado", metricas.Enviado, "estado_pedido", p.EstadoPedido, "duracion_ms", time.Since(inicio).Milliseconds())
	return d.Ack(false)
}

func reintentar(ctx context.Context, d amqp.Delivery, reg *slog.Logger, motivo string) error {
	reintentos, _ := d.Headers["x-delivery-count"].(int64)
	pausas := []time.Duration{5 * time.Second, 30 * time.Second, 2 * time.Minute}
	if reintentos < 0 || reintentos >= int64(len(pausas)) {
		metricas.Mensaje(metricas.DLQ)
		reg.Error(motivo+": se agotaron los intentos, va a la DLQ", "resultado", metricas.DLQ, "intento", reintentos+1)
		return d.Reject(false)
	}
	metricas.Mensaje(metricas.Reintento)
	reg.Warn(motivo+": se reintenta", "resultado", metricas.Reintento, "intento", reintentos+1, "espera", pausas[reintentos].String())
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

// servirSalud atiende las rutas técnicas en el puerto 8086 (solo dentro de Docker):
//   - /health: el proceso está vivo;
//   - /ready: llega a MongoDB y está escuchando la cola de RabbitMQ;
//   - /metrics: las métricas para Prometheus.
func servirSalud(ctx context.Context, db *mongo.Client) {
	rutas := http.NewServeMux()
	rutas.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"estado":"ok"}`))
	})
	rutas.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		ping, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if err := db.Ping(ping, nil); err != nil || !escuchando.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"estado":"no listo"}`))
			return
		}
		_, _ = w.Write([]byte(`{"estado":"listo"}`))
	})
	rutas.Handle(metricas.Ruta, metricas.Handler())
	puerto := os.Getenv("PORT")
	if puerto == "" {
		puerto = "8086"
	}
	srv := &http.Server{Addr: ":" + puerto, Handler: rutas, ReadHeaderTimeout: 3 * time.Second}
	go func() {
		<-ctx.Done()
		fin, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(fin)
	}()
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		registro.Error("no se pudo atender /health y /metrics", "error", err.Error())
	}
}
