package http

import (
	"context"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"time"

	"ghostkitchen/pedidos/internal/adapters/correlacion"
	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/trazas"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Un correlation id válido: hasta 64 caracteres seguros, como en el resto del sistema.
var cidValido = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)

// Correlacion reutiliza el X-Correlation-Id que manda el gateway o crea uno nuevo
// para seguir la solicitud entre servicios. Lo devuelve en la respuesta y lo deja en el context para
// pasárselo a Catálogo e Inventario.
func Correlacion(ctx *gin.Context) {
	cid := ctx.GetHeader("X-Correlation-Id")
	if !cidValido.MatchString(cid) {
		cid = uuid.NewString()
	}
	ctx.Set("cid", cid)
	ctx.Header("X-Correlation-Id", cid)
	trazas.MarcarCorrelacion(ctx.Request.Context(), cid) // para buscar la traza en Jaeger por este id
	ctx.Request = ctx.Request.WithContext(correlacion.Con(ctx.Request.Context(), cid))
	ctx.Next()
}

// Logs escribe una línea JSON por solicitud, con el correlation id y el trace_id (para pasar
// del log a la traza en Jaeger). No registra cuerpos.
func Logs(log *slog.Logger) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		inicio := time.Now()
		ctx.Next()
		traceID, spanID := trazas.IDs(ctx.Request.Context())
		log.Info("solicitud", "servicio", "pedidos", "correlation_id", ctx.GetString("cid"),
			"trace_id", traceID, "span_id", spanID,
			"metodo", ctx.Request.Method, "ruta", ctx.FullPath(), "estado", ctx.Writer.Status(),
			"duracion_ms", time.Since(inicio).Milliseconds())
	}
}

// Plazo corta la solicitud a los 3 s: es el tiempo máximo de una pantalla (ADR-005).
func Plazo(ctx *gin.Context) {
	conPlazo, cancelar := context.WithTimeout(ctx.Request.Context(), 3*time.Second)
	defer cancelar()
	ctx.Request = ctx.Request.WithContext(conPlazo)
	ctx.Next()
}

// identidad lee quién hace la solicitud. Estos headers los pone el gateway después de verificar
// el token (y pisa los que mande el cliente). Pedidos no se publica fuera de Docker, así nadie
// los puede mandar salteando el gateway. Nunca se toma la identidad del cuerpo.
func identidad(ctx *gin.Context) domain.Actor {
	return domain.Actor{
		UsuarioID: ctx.GetHeader("X-Usuario-Id"),
		Email:     ctx.GetHeader("X-Usuario-Email"),
		Rol:       strings.ToLower(ctx.GetHeader("X-Rol")),
		CocinaID:  ctx.GetHeader("X-Cocina-Id"),
		// La localidad viene codificada para URL ("Nueva%20C%C3%B3rdoba"): un header solo admite ASCII.
		Localidad: localidad(ctx.GetHeader("X-Localidad")),
	}
}

func localidad(h string) string {
	if v, err := url.PathUnescape(h); err == nil {
		return v
	}
	return ""
}
