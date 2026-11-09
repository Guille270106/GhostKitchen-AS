package http

import (
	"net/http"

	"ghostkitchen/inventario/internal/application"
	"github.com/gin-gonic/gin"
)

func RegistrarConsumos(r *gin.Engine, svc *application.Consumos) {
	r.POST("/reservas/:id/consumir", func(c *gin.Context) {
		var datos struct {
			PedidoID string `json:"pedidoId"`
		}
		if !decodificarReserva(c, &datos, false) {
			return
		}
		res, err := svc.Consumir(c.Request.Context(), c.GetHeader("Idempotency-Key"), c.Param("id"), datos.PedidoID)
		if err != nil {
			responderError(c, err)
			return
		}
		c.JSON(http.StatusOK, res)
	})
	r.POST("/consumos/:pedidoId/devolver", func(c *gin.Context) {
		if !decodificarReserva(c, &struct{}{}, true) {
			return
		}
		res, err := svc.Devolver(c.Request.Context(), c.GetHeader("Idempotency-Key"), c.Param("pedidoId"))
		if err != nil {
			responderError(c, err)
			return
		}
		c.JSON(http.StatusOK, res)
	})
	r.GET("/consumos/:pedidoId", func(c *gin.Context) {
		res, err := svc.Obtener(c.Request.Context(), c.Param("pedidoId"))
		if err != nil {
			responderError(c, err)
			return
		}
		c.JSON(http.StatusOK, res)
	})
}
