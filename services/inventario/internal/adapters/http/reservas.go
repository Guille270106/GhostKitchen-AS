package http

import (
	"encoding/json"
	"io"
	"net/http"

	"ghostkitchen/inventario/internal/application"
	"ghostkitchen/inventario/internal/domain"
	"github.com/gin-gonic/gin"
)

func decodificarReserva(c *gin.Context, datos any, permitirVacio bool) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64*1024)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(datos)
	if permitirVacio && err == io.EOF {
		return true
	}
	if err != nil {
		responderError(c, domain.ErrReservaInvalida)
		return false
	}
	if err = decoder.Decode(new(any)); err != io.EOF {
		responderError(c, domain.ErrReservaInvalida)
		return false
	}
	return true
}

// Rutas internas para Pedidos: no se publican en el gateway. La pertenencia del
// carrito al cliente se controla en Pedidos, antes de llamar a Inventario.
func RegistrarReservas(r *gin.Engine, svc *application.Reservas) {
	r.POST("/reservas", func(c *gin.Context) {
		var datos domain.SolicitudReserva
		if !decodificarReserva(c, &datos, false) {
			return
		}
		res, err := svc.Crear(c.Request.Context(), c.GetHeader("Idempotency-Key"), datos)
		if err != nil {
			responderError(c, err)
			return
		}
		c.Header("Location", "/reservas/"+res.ID)
		c.JSON(201, res)
	})
	r.PATCH("/reservas/:id", func(c *gin.Context) {
		var datos struct {
			Items *[]domain.PlatoSolicitado `json:"items"`
		}
		if !decodificarReserva(c, &datos, false) {
			return
		}
		if datos.Items == nil {
			responderError(c, domain.ErrReservaInvalida)
			return
		}
		res, err := svc.Ajustar(c.Request.Context(), c.GetHeader("Idempotency-Key"), c.Param("id"), *datos.Items)
		if err != nil {
			responderError(c, err)
			return
		}
		c.JSON(200, res)
	})
	r.POST("/reservas/:id/liberar", func(c *gin.Context) {
		if !decodificarReserva(c, &struct{}{}, true) {
			return
		}
		res, err := svc.Liberar(c.Request.Context(), c.GetHeader("Idempotency-Key"), c.Param("id"))
		if err != nil {
			responderError(c, err)
			return
		}
		c.JSON(200, res)
	})
	r.GET("/reservas/:id", func(c *gin.Context) {
		res, err := svc.Obtener(c.Request.Context(), c.Param("id"))
		if err != nil {
			responderError(c, err)
			return
		}
		c.JSON(200, res)
	})
}
