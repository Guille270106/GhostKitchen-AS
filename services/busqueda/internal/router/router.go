// Package router arma el servidor HTTP con sus middlewares y rutas.
package router

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"ghostkitchen/busqueda/internal/middleware"
)

// Registrador es un grupo de handlers que agrega sus rutas (internas, públicas, etc.).
type Registrador interface {
	Registrar(r gin.IRoutes)
}

// Nuevo crea el servidor con el middleware común y las rutas de cada grupo.
func Nuevo(log *slog.Logger, grupos ...Registrador) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(middleware.Correlacion(), middleware.Log(log), gin.Recovery())
	r.HandleMethodNotAllowed = true

	for _, g := range grupos {
		g.Registrar(r)
	}

	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"detail": "Ruta inexistente."})
	})
	r.NoMethod(func(c *gin.Context) {
		c.JSON(http.StatusMethodNotAllowed, gin.H{"detail": "Método no permitido."})
	})
	return r
}
