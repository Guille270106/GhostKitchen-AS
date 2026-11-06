// mysql-startup aplica las migraciones antes de ejecutar el servicio.
// Sin comando sirve como trabajo de migración de Compose durante la ronda 1.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

type config struct {
	host, port, user, password, database, directory string
}

func cargar() (config, error) {
	c := config{
		host: os.Getenv("DB_HOST"), port: os.Getenv("DB_PORT"),
		user: os.Getenv("DB_USER"), password: os.Getenv("DB_PASSWORD"),
		database: os.Getenv("DB_NAME"), directory: os.Getenv("MIGRATIONS_DIR"),
	}
	if c.host == "" {
		c.host = "mysql"
	}
	if c.port == "" {
		c.port = "3306"
	}
	if c.directory == "" {
		c.directory = "/migrations"
	}
	if c.user == "" || c.password == "" || c.database == "" {
		return c, errors.New("se requieren DB_USER, DB_PASSWORD y DB_NAME")
	}
	return c, nil
}

func conectar(c config) (*sql.DB, error) {
	cfg := mysql.NewConfig()
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(c.host, c.port)
	cfg.User, cfg.Passwd, cfg.DBName = c.user, c.password, c.database
	cfg.MultiStatements = true
	cfg.ParseTime = true
	cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = 5*time.Second, 30*time.Second, 30*time.Second
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, err
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(2)
	return db, nil
}

func aplicar(c config) error {
	db, err := conectar(c)
	if err != nil {
		return err
	}
	defer db.Close()
	// La base puede estar levantándose fuera de Compose; la espera también tiene tope.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for {
		if err = db.PingContext(ctx); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("MySQL no disponible dentro de 30 s: %w", ctx.Err())
		case <-time.After(time.Second):
		}
	}
	// WithInstance usa una conexión dedicada para GET_LOCK/RELEASE_LOCK.
	driver, err := migratemysql.WithInstance(db, &migratemysql.Config{
		DatabaseName: c.database, StatementTimeout: 20 * time.Second,
	})
	if err != nil {
		return err
	}
	source, err := iofs.New(os.DirFS(c.directory), ".")
	if err != nil {
		_ = driver.Close()
		return err
	}
	m, err := migrate.NewWithInstance("iofs", source, c.database, driver)
	if err != nil {
		_ = source.Close()
		_ = driver.Close()
		return err
	}
	defer m.Close()
	m.LockTimeout = 10 * time.Second
	err = m.Up()
	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return err
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	c, err := cargar()
	if err == nil {
		err = aplicar(c)
	}
	if err != nil {
		// No se registra el error del driver: puede contener SQL o datos sensibles.
		log.Error("falló la migración; el servicio no se inicia", "base", c.database,
			"accion", "revisar conectividad, archivos y schema_migrations; no forzar una versión sin reparar la base")
		os.Exit(1)
	}
	log.Info("migraciones al día", "base", c.database)
	if len(os.Args) > 1 {
		if err := ejecutar(os.Args[1:]); err != nil {
			log.Error("no se pudo iniciar el servicio")
			os.Exit(1)
		}
	}
}
