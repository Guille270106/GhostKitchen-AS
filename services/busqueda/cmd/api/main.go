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

	"ghostkitchen/busqueda/internal/config"
	"ghostkitchen/busqueda/internal/handlers"
	"ghostkitchen/busqueda/internal/repositories/solr"
	"ghostkitchen/busqueda/internal/router"
	"ghostkitchen/busqueda/internal/seed"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("servicio", "busqueda")
	cfg := config.Cargar()
	if len(cfg.APIKeys) == 0 {
		log.Warn("PUBLIC_API_KEYS está vacía: el gateway va a rechazar todas las solicitudes a /public/v1")
	}

	repo := solr.NuevoRepositorio(solr.NuevoCliente(cfg.SolrURL, cfg.SolrTimeout))

	var listo atomic.Bool
	ctx, parar := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer parar()
	go inicializarIndice(ctx, log, repo, cfg.SeedAlIniciar, &listo)

	// La capacidad publicada (/v1) se agrega en el siguiente paso.
	r := router.Nuevo(log, handlers.NuevoInterno(cfg.APIKeys, repo.Ping, &listo))
	srv := &http.Server{
		Addr:              ":" + cfg.Puerto,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
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

// inicializarIndice espera a que Solr responda, crea los campos que falten y carga los datos
// iniciales. Mientras tanto /ready responde 503 y las consultas responden 503.
func inicializarIndice(ctx context.Context, log *slog.Logger, repo *solr.Repositorio, conSeed bool, listo *atomic.Bool) {
	espera := time.Second
	for intento := 1; ; intento++ {
		err := preparar(ctx, log, repo, conSeed)
		if err == nil {
			listo.Store(true)
			log.Info("índice listo", "intentos", intento)
			return
		}
		log.Warn("Solr todavía no está listo", "intento", intento, "error", err.Error())
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

func preparar(ctx context.Context, log *slog.Logger, repo *solr.Repositorio, conSeed bool) error {
	c, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
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
