// Pedidos: carrito y pedidos de GhostKitchen. Arquitectura hexagonal: los controllers (adaptador
// de entrada) llaman a los servicios (casos de uso), que usan los puertos; MySQL, Catálogo e
// Inventario son adaptadores de salida. Las migraciones las aplica mysql-startup antes de
// arrancar este programa (ver Dockerfile).
package main

import (
	"context"
	"database/sql"
	"ghostkitchen/pedidos/internal/trazas"
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
	"ghostkitchen/pedidos/internal/adapters/proveedor"
	"ghostkitchen/pedidos/internal/adapters/rabbitmq"
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

	// Trazas (ADR-011): cada solicitud se ve en Jaeger, junto con las llamadas a otros servicios.
	cerrarTrazas, err := trazas.Iniciar(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	defer cerrarTrazas(context.Background())

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

	devolucionesRepo := mysqladapter.DevolucionesMySQL{DB: db}

	// Descontar y devolver stock (V10 de Inventario). Con CONSUMO_INVENTARIO=real (lo que usa
	// docker-compose) se llama a Inventario; con "simulado" no, para probar Pedidos solo.
	var consumo ports.Consumo = inventario.ConsumoSimulado{}
	var devolucion ports.Devolucion = inventario.DevolucionSimulada{}
	if valor("CONSUMO_INVENTARIO", "simulado") == "real" {
		consumo = inventarioCliente
		devolucion = inventarioCliente
	}
	logger.Info("descuento y devolución de stock", "modo", valor("CONSUMO_INVENTARIO", "simulado"))

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
	devolucionService := application.DevolucionService{Repo: devolucionesRepo, Inventario: devolucion}

	// 4. Controllers (adaptador de entrada) y rutas.
	router := httpadapter.Rutas(logger,
		httpadapter.SaludController{Repo: pedidosRepo},
		httpadapter.PedidosController{Service: pedidosService},
		httpadapter.CarritoController{Service: carritoService},
		httpadapter.ConfirmacionController{Service: confirmacionService},
	)

	// Métricas del admin (HU22, G21): solo lectura sobre pedido_db, y Catálogo dice qué
	// cocinas son de su localidad.
	httpadapter.RegistrarMetricas(router, httpadapter.MetricasController{Service: application.MetricasService{
		Cocinas: catalogoCliente,
		Repo:    mysqladapter.MetricasMySQL{DB: db},
	}})

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

	// Recupera reservas inciertas y liberaciones pendientes independientemente de las confirmaciones.
	go func() {
		for range time.Tick(10 * time.Second) {
			if err := carritoService.RecuperarCambiosPendientes(context.Background()); err != nil {
				logger.Error("no se pudieron recuperar cambios del carrito", "error", err)
			}
		}
	}()

	// 6. Otro proceso en segundo plano: cada 5 s devuelve los ingredientes de los pedidos cancelados.
	go func() {
		for range time.Tick(5 * time.Second) {
			hechas, err := devolucionService.ProcesarPendientes(context.Background())
			if err != nil {
				logger.Error("no se pudieron procesar las devoluciones", "error", err.Error())
			} else if hechas > 0 {
				logger.Info("ingredientes devueltos a Inventario", "pedidos", hechas)
			}
		}
	}()

	// 7. Y otro: cada 10 s le pide al grupo proveedor (Grupo 4) las facturas pendientes (L15).
	//    El pedido no espera a la factura: si el proveedor está caído, quedan pendientes.
	if url := os.Getenv("PROVIDER_BASE_URL"); url != "" {
		comprobanteService := application.ComprobanteService{
			Repo:      mysqladapter.ComprobantesMySQL{DB: db},
			Pedidos:   pedidosRepo,
			Proveedor: proveedor.Nuevo(url, os.Getenv("PROVIDER_API_KEY")),
		}
		go func() {
			for range time.Tick(10 * time.Second) {
				emitidos, err := comprobanteService.EmitirPendientes(context.Background())
				if err != nil {
					logger.Error("no se pudieron pedir las facturas", "error", err.Error())
				} else if emitidos > 0 {
					logger.Info("facturas emitidas por el proveedor", "cantidad", emitidos)
				}
			}
		}()
	} else {
		logger.Warn("falta PROVIDER_BASE_URL: las facturas quedan pendientes")
	}

	// 8. Y otro: cada 1 s publica en RabbitMQ los mensajes del outbox (pedido.estado_cambiado).
	//    Si RabbitMQ está caído, los mensajes esperan en MySQL y Pedidos sigue atendiendo.
	if url := os.Getenv("RABBITMQ_URL"); url != "" {
		publicador := &rabbitmq.PublicadorRabbit{URL: url}
		defer publicador.Cerrar()
		outboxService := application.OutboxService{Repo: mysqladapter.OutboxMySQL{DB: db}, Publicador: publicador}
		go func() {
			for range time.Tick(time.Second) {
				if _, err := outboxService.PublicarPendientes(context.Background()); err != nil {
					logger.Warn("no se pudo publicar el outbox; se reintenta en 1 s", "error", err.Error())
				}
			}
		}()

		// 9. Y escucha la cola pedidos.reservas: cuando Inventario avisa que una reserva venció
		//    (reserva.vencida), el carrito se vacía y queda marcado como vencido (L14).
		consumidor := &rabbitmq.Consumidor{
			URL:         url,
			Cola:        "pedidos.reservas",
			Manejar:     rabbitmq.ReservaVencida(carritoService),
			Log:         logger,
			EsperaError: 5 * time.Second,
		}
		go consumidor.Escuchar(context.Background())
	} else {
		logger.Warn("falta RABBITMQ_URL: los mensajes quedan en el outbox sin publicar y no se escuchan vencimientos")
	}

	// 10. Servidor, con tiempos máximos para no quedar colgado con clientes lentos.
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
