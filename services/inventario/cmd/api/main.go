package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	httpadapter "ghostkitchen/inventario/internal/adapters/http"
	mysqladapter "ghostkitchen/inventario/internal/adapters/mysql"
	"ghostkitchen/inventario/internal/adapters/rabbitmq"
	"ghostkitchen/inventario/internal/application"
	"github.com/go-sql-driver/mysql"
)

func valor(nombre, defecto string) string {
	if v := os.Getenv(nombre); v != "" {
		return v
	}
	return defecto
}

func run(log *slog.Logger) error {
	cfg := mysql.NewConfig()
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(valor("DB_HOST", "mysql"), valor("DB_PORT", "3306"))
	cfg.User = valor("DB_USER", "inventario")
	cfg.Passwd = os.Getenv("DB_PASSWORD")
	cfg.DBName = valor("DB_NAME", "inventario_db")
	if cfg.Passwd == "" {
		return errors.New("falta DB_PASSWORD")
	}
	cfg.ParseTime = true
	cfg.Timeout = time.Second
	cfg.ReadTimeout = 2 * time.Second
	cfg.WriteTimeout = 2 * time.Second
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return err
	}
	db := sql.OpenDB(connector)
	defer db.Close()
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	repo := mysqladapter.Nuevo(db)
	ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer parar()
	go periodico(ctx, log, "vencer reservas", 15*time.Second, 10*time.Second, repo.VencerReservas)
	if usuario := os.Getenv("RABBITMQ_USER"); usuario != "" {
		publicar := rabbitmq.NuevoPublicador(valor("RABBITMQ_HOST", "rabbitmq"), usuario, os.Getenv("RABBITMQ_PASSWORD"))
		go periodico(ctx, log, "publicar outbox", time.Second, 5*time.Second, func(ctx context.Context) error {
			for range 20 {
				enviado, err := repo.PublicarSiguiente(ctx, publicar)
				if err != nil || !enviado {
					return err
				}
			}
			return nil
		})
	}
	router := httpadapter.Nuevo(application.Nuevo(repo), repo.Ready, log)
	httpadapter.RegistrarRecetas(router, application.NuevasRecetas(repo))
	httpadapter.RegistrarReservas(router, application.NuevasReservas(repo))
	srv := &http.Server{Addr: ":" + valor("PORT", "8084"), Handler: router, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	fallos := make(chan error, 1)
	go func() { fallos <- srv.ListenAndServe() }()
	log.Info("escuchando", "servicio", "inventario", "puerto", valor("PORT", "8084"))
	select {
	case err := <-fallos:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		fin, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(fin)
	}
}

func periodico(ctx context.Context, log *slog.Logger, tarea string, intervalo, plazo time.Duration, hacer func(context.Context) error) {
	ticker := time.NewTicker(intervalo)
	defer ticker.Stop()
	for ctx.Err() == nil {
		intento, cancelar := context.WithTimeout(ctx, plazo)
		err := hacer(intento)
		cancelar()
		if err != nil && ctx.Err() == nil {
			log.Warn("falló tarea de Inventario", "tarea", tarea, "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("Inventario no pudo continuar; revisar configuración y MySQL")
		os.Exit(1)
	}
}
