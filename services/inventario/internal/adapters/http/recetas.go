package http

import (
	"encoding/json"
	"io"
	"net/http"

	"ghostkitchen/inventario/internal/application"
	"ghostkitchen/inventario/internal/domain"
	"github.com/gin-gonic/gin"
)

func autenticarCocina(c *gin.Context) {
	if err := identidad(c).Validar(); err != nil {
		responderError(c, err)
		c.Abort()
		return
	}
	c.Next()
}

func RegistrarRecetas(r *gin.Engine, svc *application.Recetas) {
	cocina := r.Group("/cocina", autenticarCocina)
	cocina.PUT("/platos/:id/receta", func(c *gin.Context) {
		var datos struct {
			Items []domain.RecetaItem `json:"items"`
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64*1024)
		decoder := json.NewDecoder(c.Request.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&datos); err != nil {
			responderError(c, domain.ErrRecetaInvalida)
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			responderError(c, domain.ErrRecetaInvalida)
			return
		}
		receta, err := svc.Guardar(c.Request.Context(), identidad(c), c.Param("id"), datos.Items)
		if err != nil {
			responderError(c, err)
			return
		}
		c.JSON(200, receta)
	})
	cocina.GET("/platos/:id/receta", func(c *gin.Context) {
		receta, err := svc.Obtener(c.Request.Context(), identidad(c), c.Param("id"))
		if err != nil {
			responderError(c, err)
			return
		}
		c.JSON(200, receta)
	})
	cocina.GET("/recetas", func(c *gin.Context) {
		recetas, err := svc.Listar(c.Request.Context(), identidad(c))
		if err != nil {
			responderError(c, err)
			return
		}
		c.JSON(200, gin.H{"items": recetas})
	})
}
