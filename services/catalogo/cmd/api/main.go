// Catálogo: cocinas, restaurantes y platos (qué se vende y a qué precio), sobre MongoDB.
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

	"ghostkitchen/catalogo/internal/config"
	"ghostkitchen/catalogo/internal/handlers"
	repomongo "ghostkitchen/catalogo/internal/repositories/mongo"
	"ghostkitchen/catalogo/internal/router"
	"ghostkitchen/catalogo/internal/seed"
	"ghostkitchen/catalogo/internal/services"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("servicio", "catalogo")
	cfg := config.Cargar()

	ctx, parar := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer parar()

	// El cliente se conecta de forma perezosa: si MongoDB todavía no arrancó, las operaciones
	// fallan con 503 hasta que responda. Timeout corto para no dejar esperando a nadie (ADR-005).
	cliente, err := mongo.Connect(ctx, options.Client().
		ApplyURI(cfg.MongoURI).
		SetServerSelectionTimeout(cfg.MongoTimeout).
		SetTimeout(cfg.MongoTimeout))
	if err != nil {
		log.Error("la configuración de MongoDB es inválida", "error", err.Error())
		os.Exit(1)
	}
	defer func() { _ = cliente.Disconnect(context.Background()) }()

	repo := repomongo.Nuevo(cliente, cfg.MongoBase)
	go prepararBase(ctx, log, repo, cfg.SeedAlIniciar)

	r := router.Nuevo(log, handlers.Nuevo(services.Nuevo(repo), repo.Ping))
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

// prepararBase espera a MongoDB, crea los índices y carga los datos iniciales que falten.
func prepararBase(ctx context.Context, log *slog.Logger, repo *repomongo.Repositorio, conSeed bool) {
	espera := time.Second
	for intento := 1; ; intento++ {
		err := func() error {
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
