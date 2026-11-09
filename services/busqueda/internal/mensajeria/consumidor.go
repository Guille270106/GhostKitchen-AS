// Package mensajeria recibe de RabbitMQ los mensajes que mantienen al día el índice (G15):
// cocina.* y plato.* de Catálogo, y disponibilidad.cambiada de Inventario, todos en la cola
// busqueda.indexador (infra/rabbitmq/definiciones.json).
//
// Es la segunda entrada del servicio, al lado de HTTP (ADR-002). Solo sabe hablar con
// RabbitMQ: qué hacer con cada mensaje lo decide indexer.Aplicador.
package mensajeria

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"ghostkitchen/busqueda/internal/indexer"
	"ghostkitchen/busqueda/internal/middleware"
	"ghostkitchen/busqueda/internal/trazas"
)

// Métricas del lector (G19, ADR-011: panel "Índice" y "Mensajería").
var (
	eventos = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "busqueda_eventos_total",
		Help: "Mensajes leídos de busqueda.indexador, por tipo y resultado (aplicado, rechazado, reintento).",
	}, []string{"tipo", "resultado"})

	// atraso es el tiempo desde que pasó el cambio (ocurrido_en, en Catálogo o Inventario)
	// hasta que quedó en el índice. El objetivo es que el 95 % tarde 5 s o menos.
	atraso = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "busqueda_indice_atraso_seconds",
		Help:    "Tiempo desde el cambio en Catálogo o Inventario hasta que quedó en el índice.",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 3, 5, 10, 30, 60},
	})
)

// Cola de Búsqueda.
const Cola = "busqueda.indexador"

// Procesador aplica un evento ya validado (lo implementa indexer.Aplicador).
type Procesador interface {
	Procesar(ctx context.Context, s indexer.Sobre) error
}

// Accion es lo que se le contesta a RabbitMQ por un mensaje.
type Accion int

// Respuestas posibles.
const (
	// Confirmar (ack): quedó guardado en Solr; RabbitMQ lo borra.
	Confirmar Accion = iota
	// Rechazar (reject sin reencolar): no cumple el contrato; RabbitMQ lo manda a la DLQ.
	Rechazar
	// Reintentar (nack con reencolar) después de una espera: falló algo pasajero. La cola
	// tiene x-delivery-limit 5: al quinto intento fallido RabbitMQ lo manda a la DLQ.
	Reintentar
)

// Consumidor lee la cola de a un mensaje por vez (prefetch 1). Uno solo, en orden: así los
// mensajes de una cocina y de sus platos no se pisan entre sí, y el orden de llegada es el de
// publicación salvo los reintentos (que la versión resuelve).
type Consumidor struct {
	Cola      string // busqueda.indexador (los tests usan otra)
	url       string
	proc      Procesador
	log       *slog.Logger
	PlazoUno  time.Duration // tiempo máximo para aplicar un mensaje (10 s)
	EsperaMax time.Duration // espera máxima entre reconexiones o reintentos (10 s)
}

// NuevoConsumidor crea el consumidor. La URL lleva usuario y contraseña: nunca se registra.
func NuevoConsumidor(url string, proc Procesador, log *slog.Logger) *Consumidor {
	return &Consumidor{Cola: Cola, url: url, proc: proc, log: log, PlazoUno: 10 * time.Second, EsperaMax: 10 * time.Second}
}

// Correr consume hasta que se cancela ctx. Si RabbitMQ se cae o la cola todavía no existe,
// espera cada vez más (1 s, 2 s, 4 s… hasta EsperaMax) y se vuelve a conectar. Mientras
// tanto los mensajes esperan en la cola (son persistentes) y el índice se atrasa, pero las
// búsquedas siguen andando con lo que ya tenía (ADR-010).
func (c *Consumidor) Correr(ctx context.Context) {
	espera := time.Second
	for ctx.Err() == nil {
		inicio := time.Now()
		err := c.unaConexion(ctx)
		if ctx.Err() != nil {
			return
		}
		if time.Since(inicio) > time.Minute {
			espera = time.Second // estuvo andando un rato: empezar de nuevo con la espera corta
		}
		c.log.Warn("se cortó la conexión con RabbitMQ; el índice se atrasa hasta que vuelva",
			"motivo", err.Error(), "reintento_en_s", espera.Seconds())
		select {
		case <-ctx.Done():
			return
		case <-time.After(espera):
		}
		espera = min(espera*2, c.EsperaMax)
	}
}

func (c *Consumidor) unaConexion(ctx context.Context) error {
	conn, err := amqp.DialConfig(c.url, amqp.Config{Dial: amqp.DefaultDial(5 * time.Second), Properties: amqp.Table{"connection_name": "busqueda-indexador"}})
	if err != nil {
		return errors.New("no se pudo conectar con RabbitMQ")
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		return errors.New("no se pudo abrir un canal en RabbitMQ")
	}
	if err := ch.Qos(1, 0, false); err != nil {
		return err
	}
	entregas, err := ch.Consume(c.Cola, "busqueda", false, false, false, false, nil)
	if err != nil {
		return errors.New("no se pudo leer la cola " + c.Cola + " (¿corrió rabbitmq-topologia?)")
	}
	c.log.Info("leyendo mensajes", "cola", c.Cola)
	cerrada := conn.NotifyClose(make(chan *amqp.Error, 1))
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case e := <-cerrada:
			if e != nil {
				return errors.New(e.Reason)
			}
			return errors.New("RabbitMQ cerró la conexión")
		case d, ok := <-entregas:
			if !ok {
				return errors.New("RabbitMQ cerró el canal")
			}
			if err := c.responder(ctx, d); err != nil {
				return err
			}
		}
	}
}

// responder aplica el mensaje y le contesta a RabbitMQ.
func (c *Consumidor) responder(ctx context.Context, d amqp.Delivery) error {
	accion := c.Manejar(ctx, d.Body, d.RoutingKey)
	switch accion {
	case Confirmar:
		return d.Ack(false)
	case Rechazar:
		return d.Reject(false)
	}
	// Antes de devolverlo se espera un poco, cada vez más, para no girar en falso contra un
	// Solr caído. Mientras tanto no se lee otro mensaje (prefetch 1): si Solr no anda, no
	// tiene sentido probar con los siguientes.
	intentos, _ := d.Headers["x-delivery-count"].(int64)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(EsperaReintento(intentos, c.EsperaMax)):
	}
	return d.Nack(false, true)
}

// EsperaReintento es 1 s, 2 s, 4 s… según cuántas veces se entregó el mensaje, hasta max.
func EsperaReintento(entregasPrevias int64, max time.Duration) time.Duration {
	espera := time.Second
	for i := int64(0); i < entregasPrevias && espera < max; i++ {
		espera *= 2
	}
	return min(espera, max)
}

// Manejar valida y aplica un mensaje, y dice qué contestarle a RabbitMQ. Se separa de la
// conexión para probarlo sin un RabbitMQ.
//
// El log y la traza llevan el tipo, el evento, el plato o la cocina y el correlation_id del
// cambio original, para seguirlo desde Catálogo o Inventario. Nunca el contenido.
func (c *Consumidor) Manejar(ctx context.Context, cuerpo []byte, ruta string) Accion {
	s, err := indexer.LeerSobre(cuerpo, ruta)
	if err != nil {
		c.log.Error("mensaje rechazado: no cumple el contrato", "ruta", ruta, "motivo", err.Error())
		eventos.WithLabelValues("desconocido", "rechazado").Inc() // el tipo no es confiable
		return Rechazar
	}
	ctx = middleware.ConCorrelacion(ctx, s.CorrelationID)
	ctx, span := trazas.Tracer().Start(ctx, "procesar "+s.Tipo, trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.destination.name", c.Cola),
			attribute.String("messaging.message.id", s.EventoID),
			attribute.String("evento.agregado_id", s.AgregadoID),
			attribute.Int64("evento.secuencia", s.Secuencia),
		))
	defer span.End()
	trazas.MarcarCorrelacion(ctx, s.CorrelationID)

	uno, cancel := context.WithTimeout(ctx, c.PlazoUno)
	defer cancel()
	err = c.proc.Procesar(uno, s)
	campos := []any{"tipo", s.Tipo, "evento_id", s.EventoID, "agregado_id", s.AgregadoID,
		"secuencia", s.Secuencia, "correlation_id", s.CorrelationID}
	switch {
	case err == nil:
		// demora_ms: desde que pasó el cambio hasta que quedó en el índice. El objetivo es
		// menos de 5 s (README de Búsqueda).
		demora := time.Since(s.OcurridoEn)
		c.log.Info("evento aplicado", append(campos, "demora_ms", demora.Milliseconds())...)
		eventos.WithLabelValues(s.Tipo, "aplicado").Inc()
		atraso.Observe(demora.Seconds())
		return Confirmar
	case errors.Is(err, indexer.ErrEventoInvalido):
		span.RecordError(err)
		span.SetStatus(codes.Error, "evento inválido")
		c.log.Error("mensaje rechazado: no cumple el contrato", append(campos, "motivo", err.Error())...)
		eventos.WithLabelValues(s.Tipo, "rechazado").Inc()
		return Rechazar
	default:
		span.RecordError(err)
		span.SetStatus(codes.Error, "falló al aplicar")
		c.log.Warn("no se pudo aplicar el evento; se reintenta", append(campos, "motivo", err.Error())...)
		eventos.WithLabelValues(s.Tipo, "reintento").Inc()
		return Reintentar
	}
}
