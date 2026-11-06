// Package router arma el servidor HTTP con sus middlewares y rutas. Es parte de la capa de
// entrada: junta los grupos de handlers que le pasa main, sin saber qué hace cada uno.
package router

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"ghostkitchen/busqueda/internal/middleware"
	"ghostkitchen/busqueda/internal/trazas"
)

// Registrador es un grupo de handlers que agrega sus rutas (internas, públicas, etc.).
type Registrador interface {
	Registrar(r gin.IRoutes)
}

// Nuevo crea el servidor con el middleware común y las rutas de cada grupo.
func Nuevo(log *slog.Logger, grupos ...Registrador) *gin.Engine {
	// gin.New y no gin.Default: Default agrega el log de texto de Gin, que escribe la URL
	// completa (con coordenadas). Usamos nuestro log JSON, que no la registra.
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	// Primero la traza (crea el span), después el id de correlación (lo anota en el span)
	// y el log (que escribe ambos ids).
	r.Use(trazas.Middleware(), middleware.Correlacion(), middleware.Log(log), gin.Recovery())
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
