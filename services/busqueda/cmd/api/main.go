// Búsqueda: índice de cocinas y platos (Solr) y capacidad publicada para otros grupos.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"

	"ghostkitchen/busqueda/internal/clients"
	"ghostkitchen/busqueda/internal/config"
	"ghostkitchen/busqueda/internal/handlers"
	"ghostkitchen/busqueda/internal/indexer"
	"ghostkitchen/busqueda/internal/middleware"
	"ghostkitchen/busqueda/internal/repositories/solr"
	"ghostkitchen/busqueda/internal/router"
	"ghostkitchen/busqueda/internal/seed"
	"ghostkitchen/busqueda/internal/services"
	"ghostkitchen/busqueda/internal/trazas"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("servicio", "busqueda")
	cfg := config.Cargar()

	cerrarTrazas, err := trazas.Iniciar(context.Background())
	if err != nil {
		log.Error("no se pudo configurar OpenTelemetry", "error", err.Error())
		os.Exit(1)
	}
	defer func() {
		fin, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = cerrarTrazas(fin) // manda los spans pendientes antes de salir
	}()

	if len(cfg.APIKeys) == 0 {
		log.Warn("PUBLIC_API_KEYS está vacía: el gateway va a rechazar todas las solicitudes a /public/v1")
	}

	repo := solr.NuevoRepositorio(solr.NuevoCliente(cfg.SolrURL, cfg.SolrTimeout))
	svc := services.Nueva(repo)

	// De dónde salen los datos del índice: de Catálogo si está configurado (lo normal),
	// o de los datos iniciales embebidos (para probar Búsqueda sola).
	var reconstruir func(context.Context) (indexer.Resultado, error)
	if cfg.CatalogoURL != "" {
		dispo, err := seed.DisponibilidadInicial()
		if err != nil {
			log.Error("no se pudo leer la disponibilidad inicial", "error", err.Error())
			os.Exit(1)
		}
		rec := indexer.NuevoReconstructor(clients.NuevoCatalogo(cfg.CatalogoURL), repo, dispo)
		reconstruir = func(ctx context.Context) (indexer.Resultado, error) {
			if err := repo.PrepararEsquema(ctx); err != nil {
				return indexer.Resultado{}, err
			}
			res, err := rec.Reconstruir(ctx)
			if err == nil {
				traceID, _ := trazas.IDs(ctx)
				log.Info("índice reconstruido desde Catálogo", "correlation_id", middleware.IDDesdeContexto(ctx),
					"trace_id", traceID, "sedes", res.Sedes, "platos", res.Platos)
			}
			return res, err
		}
	}

	var listo atomic.Bool
	ctx, parar := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer parar()
	go inicializarIndice(ctx, log, repo, cfg.SeedAlIniciar, reconstruir, &listo)

	r := router.Nuevo(log,
		handlers.NuevoInterno(cfg.APIKeys, repo.Ping, &listo),
		handlers.NuevoReindexar(reconstruir),
		handlers.NuevoPublico(svc),
	)
	srv := &http.Server{
		Addr:              ":" + cfg.Puerto,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second, // la reconstrucción manual puede tardar más que una consulta
	}

	go func() {
		log.Info("escuchando", "puerto", cfg.Puerto)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("el servidor se detuvo", "error", err.Error())
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("apagando")
	fin, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelar()
	_ = srv.Shutdown(fin)
}

// inicializarIndice espera a que Solr (y Catálogo, si se usa) respondan y arma el índice.
// Mientras tanto /ready responde 503 y las consultas responden 503.
func inicializarIndice(ctx context.Context, log *slog.Logger, repo *solr.Repositorio, conSeed bool,
	reconstruir func(context.Context) (indexer.Resultado, error), listo *atomic.Bool) {
	espera := time.Second
	for intento := 1; ; intento++ {
		err := preparar(ctx, log, repo, conSeed, reconstruir)
		if err == nil {
			listo.Store(true)
			log.Info("índice listo", "intentos", intento)
			return
		}
		log.Warn("el índice todavía no se pudo preparar", "intento", intento, "error", err.Error())
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

func preparar(ctx context.Context, log *slog.Logger, repo *solr.Repositorio, conSeed bool,
	reconstruir func(context.Context) (indexer.Resultado, error)) error {
	c, cancel := context.WithTimeout(middleware.ConCorrelacion(ctx, "inicio-"+uuid.NewString()), 60*time.Second)
	defer cancel()
	// Span raíz del arranque: la reconstrucción no nace de una solicitud HTTP.
	c, span := trazas.Tracer().Start(c, "preparar índice al iniciar")
	defer span.End()
	trazas.MarcarCorrelacion(c, middleware.IDDesdeContexto(c))
	if reconstruir != nil {
		_, err := reconstruir(c)
		return err
	}
	if !conSeed {
		return repo.PrepararEsquema(c)
	}
	sedes, platos, err := seed.Cargar(c, repo, time.Now())
	if err != nil {
		return err
	}
	log.Info("datos iniciales cargados", "sedes", sedes, "platos", platos)
	return nil
}
