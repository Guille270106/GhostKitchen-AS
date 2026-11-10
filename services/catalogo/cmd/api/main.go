// Package main arranca Catálogo: cocinas, restaurantes y platos (qué se vende y a qué
// precio), sobre MongoDB.
//
// Es la raíz de composición del servicio: crea el cliente de MongoDB y el repositorio
// concreto, y los conecta con las capas handlers → services → repositories (ADR-002).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/contrib/instrumentation/go.mongodb.org/mongo-driver/mongo/otelmongo"

	"ghostkitchen/catalogo/internal/clients"
	"ghostkitchen/catalogo/internal/config"
	"ghostkitchen/catalogo/internal/handlers"
	"ghostkitchen/catalogo/internal/mensajeria"
	"ghostkitchen/catalogo/internal/middleware"
	"ghostkitchen/catalogo/internal/repositories"
	"ghostkitchen/catalogo/internal/repositories/cache"
	repomongo "ghostkitchen/catalogo/internal/repositories/mongo"
	"ghostkitchen/catalogo/internal/router"
	"ghostkitchen/catalogo/internal/seed"
	"ghostkitchen/catalogo/internal/services"
	"ghostkitchen/catalogo/internal/trazas"
)

func main() {
	if err := correr(); err != nil {
		os.Exit(1) // correr ya registró el error, y sus defer (las últimas trazas) ya se ejecutaron
	}
}

// correr arma y levanta el servicio. Devuelve el error en vez de llamar a os.Exit: así se
// ejecutan los defer antes de salir, entre ellos el que manda a Jaeger las últimas trazas (L22).
func correr() error {
	cfg := config.Cargar()
	// Cada línea de log lleva la copia (catalogo-1 o catalogo-2), y cada respuesta también (G17).
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("servicio", "catalogo", "instancia", cfg.Instancia)
	middleware.Instancia = cfg.Instancia

	cerrarTrazas, err := trazas.Iniciar(context.Background())
	if err != nil {
		log.Error("no se pudo configurar OpenTelemetry", "error", err.Error())
		return err
	}
	defer func() {
		fin, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = cerrarTrazas(fin) // manda los spans pendientes antes de salir
	}()

	ctx, parar := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer parar()

	// El cliente se conecta de forma perezosa: si MongoDB todavía no arrancó, las operaciones
	// fallan con 503 hasta que responda. Timeout corto para no dejar esperando a nadie (ADR-005).
	cliente, err := mongo.Connect(ctx, options.Client().
		ApplyURI(cfg.MongoURI).
		SetServerSelectionTimeout(cfg.MongoTimeout).
		SetTimeout(cfg.MongoTimeout).
		// Un span por cada comando a MongoDB (find, count, insert), dentro de la traza
		// de la solicitud. El monitor no registra el contenido de los documentos.
		SetMonitor(otelmongo.NewMonitor()))
	if err != nil {
		log.Error("la configuración de MongoDB es inválida", "error", err.Error())
		return err
	}
	defer func() { _ = cliente.Disconnect(context.Background()) }()

	// Inyección de dependencias: services.Nuevo recibe el repositorio de MongoDB como
	// repositories.Repositorio (una interfaz), y los tests le pasan uno en memoria.
	// La base se prepara en segundo plano para que /health conteste mientras MongoDB arranca.
	repo := repomongo.Nuevo(cliente, cfg.MongoBase)
	go prepararBase(ctx, log, repo, cfg.SeedAlIniciar)

	// Publicador del outbox (G13): manda a RabbitMQ los eventos que cada escritura guardó junto
	// con su cambio. Sin RABBITMQ_URL los eventos se siguen guardando y salen cuando se configure.
	// Con 2 copias (G17), publica una sola (PUBLICAR_OUTBOX=true solo en catalogo-1): si
	// publicaran las dos, el mismo evento saldría dos veces y en otro orden. Las dos escriben en
	// el mismo outbox, así que los cambios hechos en cualquiera de las copias salen igual.
	switch {
	case !cfg.PublicarOutbox:
		log.Info("esta copia no publica el outbox (PUBLICAR_OUTBOX=false)")
	case cfg.RabbitMQURL == "":
		log.Warn("sin RABBITMQ_URL: los eventos quedan en el outbox sin publicar")
	default:
		pub := mensajeria.NuevoPublicador(repo, func() (mensajeria.Canal, error) { return mensajeria.Conectar(cfg.RabbitMQURL) }, log)
		go pub.Correr(ctx)
	}

	// Caché (G16, ADR-007). Las lecturas públicas pasan por el decorator de Memcached. Las
	// escrituras también pasan por él para invalidar; pero las lecturas que deciden algo (la
	// cocina o el plato que se va a modificar) van directo a MongoDB, la fuente de verdad.
	var lecturas repositories.Repositorio = repo
	var escritor interface {
		repositories.Escritor
		repositories.EscritorPlatos
	} = repo
	if cfg.CacheHabilitada {
		dec := cache.Nuevo(repo, repo, repo, cache.NuevoMemcached(cfg.MemcachedAddr), log)
		lecturas, escritor = dec, dec
		log.Info("caché habilitada", "memcached", cfg.MemcachedAddr)
	} else {
		log.Info("caché deshabilitada (CACHE_HABILITADA=false)")
	}

	r := router.Nuevo(log,
		handlers.Nuevo(services.Nuevo(lecturas), repo.Ping),
		handlers.NuevoAdmin(services.NuevoAdmin(repo, escritor)),
		handlers.NuevoPlatos(services.NuevoPlatosCocina(repo, escritor, clients.NuevoInventario(cfg.InventarioURL))))
	srv := &http.Server{
		Addr:              ":" + cfg.Puerto,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}
	errServidor := make(chan error, 1)
	go func() {
		log.Info("escuchando", "puerto", cfg.Puerto)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errServidor <- err
		}
	}()

	// Apagado ordenado: se dejan terminar las solicitudes en curso hasta 10 s.
	select {
	case <-ctx.Done():
	case err := <-errServidor:
		log.Error("el servidor se detuvo", "error", err.Error())
		return err
	}
	log.Info("apagando")
	fin, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelar()
	_ = srv.Shutdown(fin)
	return nil
}

// prepararBase espera a MongoDB, crea los índices y carga los datos iniciales que falten.
// Reintenta sin límite, con una espera que se duplica en cada intento, porque en Docker
// Compose MongoDB puede tardar más que Catálogo en aceptar conexiones (ADR-010). Se puede
// repetir sin problema: crear índices y cargar el seed no duplican nada.
func prepararBase(ctx context.Context, log *slog.Logger, repo *repomongo.Repositorio, conSeed bool) {
	espera := time.Second
	for intento := 1; ; intento++ {
		err := func() error {
			// Plazo más largo que el de una solicitud: es trabajo de fondo, nadie lo espera.
			c, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			if err := repo.CrearIndices(c); err != nil {
				return err
			}
			if !conSeed {
				return nil
			}
			cocinas, platos, err := seed.Cargar(c, repo, time.Now())
			if err == nil {
				log.Info("datos iniciales verificados", "cocinas", cocinas, "platos", platos)
			}
			return err
		}()
		if err == nil {
			log.Info("base lista", "intentos", intento)
			return
		}
		log.Warn("MongoDB todavía no está listo", "intento", intento, "error", err.Error())
		select {
		case <-ctx.Done():
			return
		case <-time.After(espera):
		}
		if espera < 10*time.Second {
			espera *= 2
		}
	}
}
