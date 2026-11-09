package http

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"ghostkitchen/pedidos/internal/application"
	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/ports"
	"github.com/gin-gonic/gin"
)

// SaludController atiende los chequeos de Docker.
type SaludController struct {
	Repo ports.PedidosRepo
}

// Health dice que el proceso está vivo.
func (c SaludController) Health(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{"estado": "ok"})
}

// Ready dice si se puede atender: MySQL responde y las tablas existen.
func (c SaludController) Ready(ctx *gin.Context) {
	conPlazo, cancelar := context.WithTimeout(ctx.Request.Context(), time.Second)
	defer cancelar()
	if err := c.Repo.Ready(conPlazo); err != nil {
		fallo(ctx, http.StatusServiceUnavailable, "SERVICIO_NO_DISPONIBLE", "MySQL o esquema no disponible")
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"estado": "listo"})
}

// PedidosController atiende las rutas de pedidos.
type PedidosController struct {
	Service application.PedidosService
}

// Obtener devuelve un pedido: el cliente ve los suyos y la cocina los de su cocina.
func (c PedidosController) Obtener(ctx *gin.Context) {
	pedido, err := c.Service.Obtener(ctx.Request.Context(), identidad(ctx), ctx.Param("id"))
	if err != nil {
		responderError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, pedido)
}

// CarritoController atiende las rutas del carrito del cliente.
type CarritoController struct {
	Service application.CarritoService
}

// Ver devuelve el carrito (vacío si todavía no tiene platos).
func (c CarritoController) Ver(ctx *gin.Context) {
	carrito, err := c.Service.Ver(ctx.Request.Context(), identidad(ctx))
	if err != nil {
		responderError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, carrito)
}

// Agregar suma un plato: {"platoId": "p17", "cantidad": 2}.
func (c CarritoController) Agregar(ctx *gin.Context) {
	var datos struct {
		PlatoID  string `json:"platoId"`
		Cantidad int64  `json:"cantidad"`
	}
	if !leerJSON(ctx, &datos) || datos.PlatoID == "" {
		fallo(ctx, http.StatusBadRequest, "DATOS_INVALIDOS", `Enviá {"platoId": "...", "cantidad": 1}`)
		return
	}
	carrito, err := c.Service.AgregarPlato(ctx.Request.Context(), identidad(ctx), datos.PlatoID, datos.Cantidad)
	if err != nil {
		responderError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, carrito)
}

// CambiarCantidad deja una cantidad final: {"cantidad": 3}.
func (c CarritoController) CambiarCantidad(ctx *gin.Context) {
	var datos struct {
		Cantidad int64 `json:"cantidad"`
	}
	if !leerJSON(ctx, &datos) {
		fallo(ctx, http.StatusBadRequest, "DATOS_INVALIDOS", `Enviá {"cantidad": 1}`)
		return
	}
	carrito, err := c.Service.CambiarCantidad(ctx.Request.Context(), identidad(ctx), ctx.Param("platoId"), datos.Cantidad)
	if err != nil {
		responderError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, carrito)
}

// Quitar saca un plato del carrito.
func (c CarritoController) Quitar(ctx *gin.Context) {
	carrito, err := c.Service.QuitarPlato(ctx.Request.Context(), identidad(ctx), ctx.Param("platoId"))
	if err != nil {
		responderError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, carrito)
}

// Vaciar saca todos los platos y libera la reserva.
func (c CarritoController) Vaciar(ctx *gin.Context) {
	carrito, err := c.Service.Vaciar(ctx.Request.Context(), identidad(ctx))
	if err != nil {
		responderError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, carrito)
}

// leerJSON lee el cuerpo (hasta 16 KiB) y rechaza campos que no conocemos.
func leerJSON(ctx *gin.Context, destino any) bool {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 16*1024)
	decoder := json.NewDecoder(ctx.Request.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(destino) == nil
}

// ConfirmacionController atiende la confirmación del pedido (la saga).
type ConfirmacionController struct {
	Service application.ConfirmacionService
}

// Confirmar es "Confirmar pedido". Pide el header Idempotency-Key: si el cliente aprieta dos
// veces (o el navegador reintenta), la misma clave devuelve el mismo pedido.
//   - 201: el pedido quedó creado en RECIBIDO.
//   - 202: "Estamos confirmando tu pedido": no sabemos todavía si Inventario descontó; se sigue
//     en segundo plano. Se consulta en GET /confirmaciones/{id}.
func (c ConfirmacionController) Confirmar(ctx *gin.Context) {
	resultado, err := c.Service.Confirmar(ctx.Request.Context(), identidad(ctx), ctx.GetHeader("Idempotency-Key"))
	if err != nil {
		responderError(ctx, err)
		return
	}
	if resultado.Pedido != nil {
		ctx.Header("Location", "/pedidos/"+resultado.Pedido.ID)
		ctx.JSON(http.StatusCreated, resultado.Pedido)
		return
	}
	ctx.Header("Location", "/confirmaciones/"+resultado.Confirmacion.ID)
	ctx.JSON(http.StatusAccepted, gin.H{
		"estado":         "CONFIRMANDO",
		"confirmacionId": resultado.Confirmacion.ID,
		"mensaje":        "Estamos confirmando tu pedido",
	})
}

// Ver dice cómo va una confirmación: EN_CURSO, COMPLETADA (con el pedidoId) o FALLIDA.
func (c ConfirmacionController) Ver(ctx *gin.Context) {
	conf, err := c.Service.Ver(ctx.Request.Context(), identidad(ctx), ctx.Param("id"))
	if err != nil {
		responderError(ctx, err)
		return
	}
	if conf.Estado != domain.ConfirmacionCompletada {
		conf.PedidoID = "" // el pedido todavía no existe
	}
	ctx.JSON(http.StatusOK, conf)
}
