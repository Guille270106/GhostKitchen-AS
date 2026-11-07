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

	"ghostkitchen/usuarios/internal/clients"
	"ghostkitchen/usuarios/internal/handlers"
	repomysql "ghostkitchen/usuarios/internal/repositories/mysql"
	"ghostkitchen/usuarios/internal/seed"
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

// sembrarAdmins carga las cuentas de admin que falten, con la contraseña de ADMIN_PASSWORD.
// Sin esa variable no se crea ninguna (y se avisa): así no queda un admin con una contraseña
// conocida escrita en el repositorio. Si falla, el servicio arranca igual: registro y login
// de las cuentas que ya existen siguen andando.
func sembrarAdmins(log *slog.Logger, svc *services.Usuarios) {
	password := os.Getenv("ADMIN_PASSWORD")
	if password == "" {
		log.Warn("sin ADMIN_PASSWORD: no se cargan las cuentas de admin iniciales")
		return
	}
	datos, err := seed.Admins()
	if err != nil {
		log.Error("no se pudieron leer los admins iniciales")
		return
	}
	admins := make([]services.AdminInicial, len(datos))
	for i, a := range datos {
		admins[i] = services.AdminInicial{Nombre: a.Nombre, Email: a.Email, Localidad: a.Localidad}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	creados, err := svc.SembrarAdmins(ctx, password, admins)
	if err != nil {
		log.Error("no se pudieron cargar todos los admins iniciales", "creados", creados)
		return
	}
	log.Info("admins iniciales al día", "creados", creados, "total", len(admins))
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

	svc := services.Nuevo(repomysql.Nuevo(db), tk).
		ConCatalogo(clients.NuevoCatalogo(valor("CATALOGO_URL", "http://catalogo:8082")))
	sembrarAdmins(log, svc)
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
