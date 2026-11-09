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

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("Inventario no pudo continuar; revisar configuración y MySQL")
		os.Exit(1)
	}
}
