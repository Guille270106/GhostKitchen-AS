package http

import (
	"context"
	"net/http"

	"ghostkitchen/inventario/internal/domain"
	"github.com/gin-gonic/gin"
)

func RegistrarDisponibilidad(r *gin.Engine, listar func(context.Context) ([]domain.Disponibilidad, error)) {
	r.GET("/platos/disponibilidad", func(c *gin.Context) {
		items, err := listar(c.Request.Context())
		if err != nil {
			responderError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	})
}
