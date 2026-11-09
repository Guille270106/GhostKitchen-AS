package http

import (
	"net/http"

	"ghostkitchen/pedidos/internal/application"
	"github.com/gin-gonic/gin"
)

// MetricasController atiende la pantalla de métricas del admin (HU22, G21).
type MetricasController struct {
	Service application.MetricasService
}

// RegistrarMetricas suma GET /admin/metricas (el gateway la publica en /api/v1/admin/metricas).
func RegistrarMetricas(router *gin.Engine, c MetricasController) {
	router.GET("/admin/metricas", c.Obtener)
}

// Obtener devuelve cuántos pedidos tuvo cada cocina de la localidad del admin en los últimos
// 7 días y el detalle de una (?cocinaId=, o la primera). Solo para admins.
func (c MetricasController) Obtener(ctx *gin.Context) {
	m, err := c.Service.Obtener(ctx.Request.Context(), identidad(ctx), ctx.Query("cocinaId"))
	if err != nil {
		responderError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, m)
}
