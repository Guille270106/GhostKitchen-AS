// Comando api arranca el servicio Usuarios (puerto 8081).
//
// En Docker no se ejecuta directo: el ENTRYPOINT es mysql-startup, que primero aplica las
// migraciones de migrations/ y recién después arranca este binario (infra/mysql/README.md).
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
	"strconv"
	"syscall"
	"time"

	"github.com/go-sql-driver/mysql"

	"ghostkitchen/usuarios/internal/handlers"
	repomysql "ghostkitchen/usuarios/internal/repositories/mysql"
	"ghostkitchen/usuarios/internal/services"
)

func valor(nombre, defecto string) string {
	if v := os.Getenv(nombre); v != "" {
		return v
	}
	return defecto
}

// abrirMySQL arma la conexión con límites cortos: si MySQL no contesta, la solicitud falla
// rápido (503) en lugar de quedar colgada. La contraseña viene del entorno, nunca del código.
func abrirMySQL() (*sql.DB, error) {
	cfg := mysql.NewConfig()
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(valor("DB_HOST", "mysql"), valor("DB_PORT", "3306"))
	cfg.User = valor("DB_USER", "usuarios")
	cfg.Passwd = os.Getenv("DB_PASSWORD")
	cfg.DBName = valor("DB_NAME", "usuarios_db")
	if cfg.Passwd == "" {
		return nil, errors.New("falta DB_PASSWORD")
	}
	cfg.ParseTime = true
	cfg.Timeout = time.Second
	cfg.ReadTimeout = 2 * time.Second
	cfg.WriteTimeout = 2 * time.Second
	conn, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, err
	}
	db := sql.OpenDB(conn)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	return db, nil
}

// tokens lee JWT_SECRET y JWT_TTL_MINUTES (120 por defecto). Sin una clave de al menos 32
// bytes el servicio no arranca: es preferible a firmar tokens que se puedan adivinar.
func tokens() (*services.Tokens, error) {
	minutos, err := strconv.Atoi(valor("JWT_TTL_MINUTES", "120"))
	if err != nil {
		return nil, errors.New("JWT_TTL_MINUTES tiene que ser un número de minutos")
	}
	return services.NuevosTokens(os.Getenv("JWT_SECRET"), time.Duration(minutos)*time.Minute)
}

func run(log *slog.Logger) error {
	tk, err := tokens()
	if err != nil {
		log.Error("configuración inválida", "motivo", err.Error())
		return err
	}
	db, err := abrirMySQL()
	if err != nil {
		return err
	}
	defer db.Close()

	svc := services.Nuevo(repomysql.Nuevo(db), tk)
	puerto := valor("PORT", "8081")
	srv := &http.Server{
		Addr:              ":" + puerto,
		Handler:           handlers.Nuevo(svc, log),
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer parar()
	fallos := make(chan error, 1)
	go func() { fallos <- srv.ListenAndServe() }()
	log.Info("escuchando", "servicio", "usuarios", "puerto", puerto)

	select {
	case err := <-fallos:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		// Docker manda SIGTERM al parar: se terminan las solicitudes en curso (hasta 10 s).
		fin, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(fin)
	}
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		// No se registra err: el driver puede incluir la dirección o datos de la conexión.
		log.Error("Usuarios no pudo continuar; revisar la configuración y MySQL")
		os.Exit(1)
	}
}
