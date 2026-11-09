// Pedidos: carrito y pedidos de GhostKitchen. Arquitectura hexagonal: los controllers (adaptador
// de entrada) llaman a los servicios (casos de uso), que usan los puertos; MySQL, Catálogo e
// Inventario son adaptadores de salida. Las migraciones las aplica mysql-startup antes de
// arrancar este programa (ver Dockerfile).
package main

import (
	"context"
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
	"ghostkitchen/pedidos/internal/ports"
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
	confirmacionesRepo := mysqladapter.ConfirmacionesMySQL{DB: db}
	catalogoCliente := catalogo.Nuevo(valor("CATALOGO_URL", "http://catalogo:8082"))
	inventarioCliente := inventario.Nuevo(valor("INVENTARIO_URL", "http://inventario:8084"))

	// El descuento de stock es la V10 de Inventario, que todavía no existe. Mientras tanto se
	// usa el Inventario falso; con CONSUMO_INVENTARIO=real se usa el de verdad.
	var consumo ports.Consumo = inventario.ConsumoSimulado{}
	if valor("CONSUMO_INVENTARIO", "simulado") == "real" {
		consumo = inventarioCliente
	}
	logger.Info("descuento de stock", "modo", valor("CONSUMO_INVENTARIO", "simulado"))

	// 3. Casos de uso: reciben las interfaces, no MySQL ni HTTP.
	pedidosService := application.PedidosService{Repo: pedidosRepo}
	carritoService := application.CarritoService{
		Repo:       carritosRepo,
		Catalogo:   catalogoCliente,
		Inventario: inventarioCliente,
	}
	confirmacionService := application.ConfirmacionService{
		Repo:     confirmacionesRepo,
		Carritos: carritosRepo,
		Pedidos:  pedidosRepo,
		Consumo:  consumo,
	}

	// 4. Controllers (adaptador de entrada) y rutas.
	router := httpadapter.Rutas(logger,
		httpadapter.SaludController{Repo: pedidosRepo},
		httpadapter.PedidosController{Service: pedidosService},
		httpadapter.CarritoController{Service: carritoService},
		httpadapter.ConfirmacionController{Service: confirmacionService},
	)

	// 5. Proceso en segundo plano: cada 10 s sigue las confirmaciones que quedaron a medias.
	go func() {
		for range time.Tick(10 * time.Second) {
			revisadas, err := confirmacionService.RecuperarPendientes(context.Background())
			if err != nil {
				logger.Error("no se pudieron revisar las confirmaciones pendientes", "error", err.Error())
			} else if revisadas > 0 {
				logger.Info("confirmaciones pendientes revisadas", "cantidad", revisadas)
			}
		}
	}()

	// 6. Servidor, con tiempos máximos para no quedar colgado con clientes lentos.
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
