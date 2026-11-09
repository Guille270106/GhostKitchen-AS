// Pedidos: carrito y pedidos de GhostKitchen. Arquitectura hexagonal: los controllers (adaptador
// de entrada) llaman a los servicios (casos de uso), que usan los puertos; MySQL, Catálogo e
// Inventario son adaptadores de salida. Las migraciones las aplica mysql-startup antes de
// arrancar este programa (ver Dockerfile).
package main

import (
	"database/sql"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"ghostkitchen/pedidos/internal/adapters/catalogo"
	httpadapter "ghostkitchen/pedidos/internal/adapters/http"
	"ghostkitchen/pedidos/internal/adapters/inventario"
	mysqladapter "ghostkitchen/pedidos/internal/adapters/mysql"
	"ghostkitchen/pedidos/internal/application"
	"github.com/go-sql-driver/mysql"
)

// valor lee una variable de entorno o usa el valor por defecto.
func valor(nombre, defecto string) string {
	if v := os.Getenv(nombre); v != "" {
		return v
	}
	return defecto
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// 1. Infraestructura: la conexión a MySQL (pedidos_db, con su propio usuario).
	cfg := mysql.NewConfig()
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(valor("DB_HOST", "mysql"), valor("DB_PORT", "3306"))
	cfg.User = valor("DB_USER", "pedidos")
	cfg.Passwd = os.Getenv("DB_PASSWORD")
	cfg.DBName = valor("DB_NAME", "pedidos_db")
	cfg.ParseTime = true
	cfg.Timeout = time.Second
	cfg.ReadTimeout = 2 * time.Second
	cfg.WriteTimeout = 2 * time.Second
	if cfg.Passwd == "" {
		log.Fatal("falta DB_PASSWORD")
	}
	conector, err := mysql.NewConnector(cfg)
	if err != nil {
		log.Fatal(err)
	}
	db := sql.OpenDB(conector)
	defer db.Close()
	db.SetMaxOpenConns(10)

	// 2. Adaptadores de salida (implementan los puertos).
	pedidosRepo := mysqladapter.PedidosMySQL{DB: db}
	carritosRepo := mysqladapter.CarritosMySQL{DB: db}
	catalogoCliente := catalogo.Nuevo(valor("CATALOGO_URL", "http://catalogo:8082"))
	inventarioCliente := inventario.Nuevo(valor("INVENTARIO_URL", "http://inventario:8084"))

	// 3. Casos de uso: reciben las interfaces, no MySQL ni HTTP.
	pedidosService := application.PedidosService{Repo: pedidosRepo}
	carritoService := application.CarritoService{
		Repo:       carritosRepo,
		Catalogo:   catalogoCliente,
		Inventario: inventarioCliente,
	}

	// 4. Controllers (adaptador de entrada) y rutas.
	router := httpadapter.Rutas(logger,
		httpadapter.SaludController{Repo: pedidosRepo},
		httpadapter.PedidosController{Service: pedidosService},
		httpadapter.CarritoController{Service: carritoService},
	)

	// 5. Servidor, con tiempos máximos para no quedar colgado con clientes lentos.
	puerto := valor("PORT", "8085")
	servidor := &http.Server{
		Addr:              ":" + puerto,
		Handler:           router,
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
	}
	logger.Info("escuchando", "servicio", "pedidos", "puerto", puerto)
	if err := servidor.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
