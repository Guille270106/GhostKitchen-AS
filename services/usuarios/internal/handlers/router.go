// Package handlers recibe las solicitudes HTTP y responde. Las reglas están en services.
package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"ghostkitchen/usuarios/internal/metricas"
	"ghostkitchen/usuarios/internal/services"
	"ghostkitchen/usuarios/internal/trazas"
)

// PlazoSolicitud es el máximo que puede tardar una solicitud adentro del servicio (ADR-005).
const PlazoSolicitud = 3 * time.Second

// Un X-Correlation-Id que no cumple este formato se reemplaza: así nadie mete en los logs
// texto largo o caracteres raros a través de ese header.
var correlacionValida = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)

// Nuevo arma el router: las rutas técnicas y las de cuentas.
func Nuevo(svc *services.Usuarios, log *slog.Logger) *gin.Engine {
	r := gin.New()
	r.Use(metricas.Middleware(), trazas.Middleware(), gin.Recovery(), registrar(log))
	r.GET(metricas.Ruta, metricas.Handler()) // Prometheus

	// Vivo: el proceso responde. No mira la base, para que Docker no lo reinicie si MySQL cae.
	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"estado": "ok"}) })
	// Listo: puede atender (MySQL responde y la tabla existe).
	r.GET("/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
		defer cancel()
		if err := svc.Listo(ctx); err != nil {
			problema(c, http.StatusServiceUnavailable, "SERVICIO_NO_DISPONIBLE", "MySQL no disponible o sin migrar")
			return
		}
		c.JSON(http.StatusOK, gin.H{"estado": "listo"})
	})

	registrarRutasAuth(r, svc)
	registrarRutasAdmin(r, svc)

	r.NoRoute(func(c *gin.Context) { problema(c, http.StatusNotFound, "NO_ENCONTRADO", "Recurso no encontrado") })
	return r
}

// registrar pone el correlation id, el plazo de la solicitud y escribe una línea de log JSON
// por solicitud. Guarda la ruta como plantilla (c.FullPath), nunca el cuerpo: ahí pueden
// venir el correo o la contraseña (ADR-011).
func registrar(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		cid := c.GetHeader("X-Correlation-Id")
		if !correlacionValida.MatchString(cid) {
			cid = uuid.NewString()
		}
		c.Set("cid", cid)
		c.Header("X-Correlation-Id", cid)
		trazas.MarcarCorrelacion(c.Request.Context(), cid) // para buscar la traza en Jaeger por este id
		ctx, cancel := context.WithTimeout(c.Request.Context(), PlazoSolicitud)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)

		inicio := time.Now()
		c.Next()
		traceID, spanID := trazas.IDs(c.Request.Context())
		log.Info("solicitud", "servicio", "usuarios", "correlation_id", cid, "trace_id", traceID, "span_id", spanID,
			"ruta", c.FullPath(), "metodo", c.Request.Method,
			"estado", c.Writer.Status(), "duracion_ms", time.Since(inicio).Milliseconds())
	}
}

// problema responde un error en formato Problem Details (RFC 9457), igual que los otros
// servicios. El detalle es un texto fijo: nunca el error de la base.
func problema(c *gin.Context, estado int, codigo, detalle string) {
	c.Header("Content-Type", "application/problem+json")
	c.AbortWithStatusJSON(estado, gin.H{
		"type":          "https://ghostkitchen.dev/errores/" + strings.ToLower(strings.ReplaceAll(codigo, "_", "-")),
		"title":         http.StatusText(estado),
		"status":        estado,
		"code":          codigo,
		"detail":        detalle,
		"correlationId": c.GetString("cid"),
	})
}
