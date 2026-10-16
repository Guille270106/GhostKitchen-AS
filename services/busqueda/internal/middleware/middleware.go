// Package middleware tiene lo que se aplica a todas las solicitudes:
// el identificador de correlación y el log estructurado de cada solicitud.
package middleware

import (
	"context"
	"log/slog"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// HeaderCorrelacion es el header que acompaña una solicitud por todos los servicios.
const HeaderCorrelacion = "X-Correlation-Id"

const claveCorrelacion = "correlation_id"

type claveContexto struct{}

// idValido acepta ids de hasta 64 caracteres seguros, como dice el contrato.
var idValido = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)

// Correlacion toma el X-Correlation-Id recibido (o genera uno), lo devuelve en la respuesta
// y lo deja disponible en el contexto para los logs y las llamadas a otros servicios.
func Correlacion() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(HeaderCorrelacion)
		if !idValido.MatchString(id) {
			id = uuid.NewString()
		}
		c.Set(claveCorrelacion, id)
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), claveContexto{}, id))
		c.Header(HeaderCorrelacion, id)
		c.Next()
	}
}

// IDCorrelacion devuelve el id de correlación de la solicitud.
func IDCorrelacion(c *gin.Context) string { return c.GetString(claveCorrelacion) }

// ConCorrelacion guarda un id de correlación en el contexto. Sirve para trabajos que no
// nacen de una solicitud HTTP, como la reconstrucción del índice al arrancar.
func ConCorrelacion(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, claveContexto{}, id)
}

// IDDesdeContexto devuelve el id de correlación guardado en un context.Context.
func IDDesdeContexto(ctx context.Context) string {
	id, _ := ctx.Value(claveContexto{}).(string)
	return id
}

// Log escribe una línea JSON por solicitud. Registra la ruta sin parámetros ni cuerpo:
// así no quedan coordenadas ni claves en los logs.
func Log(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		inicio := time.Now()
		c.Next()
		ruta := c.FullPath() // plantilla de la ruta, ej. /v1/sedes/:sedeId/platos
		if ruta == "" {
			ruta = "sin_ruta"
		}
		nivel := slog.LevelInfo
		if c.Writer.Status() >= 500 {
			nivel = slog.LevelError
		}
		attrs := []slog.Attr{
			slog.String("correlation_id", IDCorrelacion(c)),
			slog.String("metodo", c.Request.Method),
			slog.String("ruta", ruta),
			slog.Int("estado", c.Writer.Status()),
			slog.Float64("duracion_ms", float64(time.Since(inicio).Microseconds())/1000),
		}
		if len(c.Errors) > 0 {
			attrs = append(attrs, slog.String("error", c.Errors.String()))
		}
		log.LogAttrs(c.Request.Context(), nivel, "solicitud", attrs...)
	}
}
