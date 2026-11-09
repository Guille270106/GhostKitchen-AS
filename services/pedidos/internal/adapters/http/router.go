// Package http es el adaptador de entrada: traduce HTTP a acciones de application y errores
// del dominio a Problem Details. No tiene reglas de negocio.
package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"ghostkitchen/pedidos/internal/application"
	"ghostkitchen/pedidos/internal/domain"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var cidValido = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)

// identidad lee quién hace la solicitud. Estos headers los pone el gateway después de verificar
// el token (G9) y pisa los que mande el cliente; Pedidos no se publica fuera de Docker, así
// nadie los puede mandar salteando el gateway. Nunca se toma la identidad del cuerpo.
func identidad(c *gin.Context) domain.Actor {
	return domain.Actor{
		UsuarioID: c.GetHeader("X-Usuario-Id"),
		Rol:       strings.ToLower(c.GetHeader("X-Rol")),
		CocinaID:  c.GetHeader("X-Cocina-Id"),
	}
}

// Nuevo arma el servidor con el log JSON por solicitud, el correlation id y el plazo de 3 s.
func Nuevo(pedidos *application.Pedidos, ready func(context.Context) error, log *slog.Logger) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(func(c *gin.Context) {
		cid := c.GetHeader("X-Correlation-Id")
		if !cidValido.MatchString(cid) {
			cid = uuid.NewString()
		}
		c.Set("cid", cid)
		c.Header("X-Correlation-Id", cid)
		inicio := time.Now()
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
		log.Info("solicitud", "servicio", "pedidos", "correlation_id", cid, "ruta", c.FullPath(),
			"metodo", c.Request.Method, "estado", c.Writer.Status(), "duracion_ms", time.Since(inicio).Milliseconds())
	})

	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"estado": "ok"}) })
	r.GET("/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
		defer cancel()
		if ready(ctx) != nil {
			fallo(c, 503, "SERVICIO_NO_DISPONIBLE", "MySQL o esquema no disponible")
			return
		}
		c.JSON(200, gin.H{"estado": "listo"})
	})

	// Detalle de un pedido: el cliente ve los suyos y la cocina los de su cocina.
	r.GET("/pedidos/:id", func(c *gin.Context) {
		p, err := pedidos.Obtener(c.Request.Context(), identidad(c), c.Param("id"))
		if err != nil {
			responderError(c, err)
			return
		}
		c.JSON(200, p)
	})

	r.NoRoute(func(c *gin.Context) { fallo(c, 404, "NO_ENCONTRADO", "Recurso no encontrado") })
	return r
}

func responderError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrSinIdentidad):
		fallo(c, 401, "NO_AUTENTICADO", "Falta la identidad verificada por el gateway")
	case errors.Is(err, domain.ErrSinPermiso):
		fallo(c, 403, "SIN_PERMISO", err.Error())
	case errors.Is(err, domain.ErrNoEncontrado):
		fallo(c, 404, "PEDIDO_NO_ENCONTRADO", "El pedido no existe")
	case errors.Is(err, domain.ErrDatosInvalidos):
		fallo(c, 400, "DATOS_INVALIDOS", err.Error())
	case errors.Is(err, domain.ErrTransicionInvalida):
		fallo(c, 409, "TRANSICION_INVALIDA", err.Error())
	case errors.Is(err, domain.ErrConflictoVersion):
		fallo(c, 409, "PEDIDO_MODIFICADO", "Otro cambio se guardó primero; volvé a consultar el pedido")
	default:
		_ = c.Error(err)
		c.Header("Retry-After", "5")
		fallo(c, 503, "SERVICIO_NO_DISPONIBLE", "No se pudo completar la operación")
	}
}

// fallo responde con Problem Details, con el mismo formato que Inventario y Búsqueda.
func fallo(c *gin.Context, status int, code, detail string) {
	c.Header("Content-Type", "application/problem+json")
	c.AbortWithStatusJSON(status, gin.H{
		"type":          "https://ghostkitchen.dev/errores/" + strings.ToLower(strings.ReplaceAll(code, "_", "-")),
		"title":         http.StatusText(status),
		"status":        status,
		"code":          code,
		"detail":        detail,
		"correlationId": c.GetString("cid"),
	})
}
