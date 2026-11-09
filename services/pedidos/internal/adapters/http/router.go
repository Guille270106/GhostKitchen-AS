// Package http es el adaptador de entrada: los controllers reciben la solicitud de Gin, llaman
// al caso de uso y eligen el código HTTP. No tienen reglas de negocio.
package http

import (
	"log/slog"

	"github.com/gin-gonic/gin"
)

// Rutas arma el servidor con sus middlewares y todas las rutas de Pedidos.
// Ninguna se publica directo: el frontend entra por el gateway en /api/v1.
func Rutas(log *slog.Logger, salud SaludController, pedidos PedidosController, carrito CarritoController, confirmacion ConfirmacionController) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()

	// Middlewares: recuperarse de un panic, correlation id, log JSON y plazo de 3 s.
	router.Use(gin.Recovery())
	router.Use(Correlacion)
	router.Use(Logs(log))
	router.Use(Plazo)

	// Chequeos de Docker.
	router.GET("/health", salud.Health)
	router.GET("/ready", salud.Ready)

	// Carrito del cliente.
	router.GET("/carrito", carrito.Ver)
	router.POST("/carrito/items", carrito.Agregar)
	router.PATCH("/carrito/items/:platoId", carrito.CambiarCantidad)
	router.DELETE("/carrito/items/:platoId", carrito.Quitar)
	router.DELETE("/carrito", carrito.Vaciar)

	// Confirmar el pedido (la saga) y ver cómo va una confirmación.
	router.POST("/pedidos", confirmacion.Confirmar)
	router.GET("/confirmaciones/:id", confirmacion.Ver)

	// Pedidos.
	router.GET("/pedidos", pedidos.Listar)
	router.GET("/pedidos/:id", pedidos.Obtener)
	router.POST("/pedidos/:id/entregado", pedidos.ConfirmarEntrega)

	router.NoRoute(func(ctx *gin.Context) { fallo(ctx, 404, "NO_ENCONTRADO", "Recurso no encontrado") })
	return router
}
