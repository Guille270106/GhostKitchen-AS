package handlers

import (
	"context"
	"crypto/subtle"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// Interno atiende las rutas técnicas: salud, preparado y verificación de API key.
// Ninguna se publica: el gateway bloquea /internal/ desde afuera.
type Interno struct {
	claves [][]byte
	ping   func(context.Context) error
	listo  *atomic.Bool
}

// NuevoInterno crea los handlers internos. claves son las API keys válidas (PUBLIC_API_KEYS);
// si no hay ninguna, se rechazan todas (falla cerrada).
func NuevoInterno(claves []string, ping func(context.Context) error, listo *atomic.Bool) *Interno {
	bs := make([][]byte, 0, len(claves))
	for _, k := range claves {
		bs = append(bs, []byte(k))
	}
	return &Interno{claves: bs, ping: ping, listo: listo}
}

// Registrar agrega las rutas internas.
func (h *Interno) Registrar(r gin.IRoutes) {
	r.GET("/health", h.salud)
	r.GET("/ready", h.preparado)
	r.GET("/internal/apikey/verificar", h.verificarAPIKey)
}

// GET /health: el proceso está vivo. No consulta a Solr a propósito: si Solr se cae,
// reiniciar Búsqueda no lo arregla. Eso lo dice /ready (ADR-010, salud y reinicio).
func (h *Interno) salud(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"estado": "ok"})
}

// GET /ready: el índice está preparado y Solr responde.
func (h *Interno) preparado(c *gin.Context) {
	if !h.listo.Load() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"estado": "iniciando"})
		return
	}
	// Un chequeo de salud tiene que contestar rápido: si Solr tarda más de 1 s, para
	// nosotros no está listo.
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
	defer cancel()
	if err := h.ping(ctx); err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"estado": "solr_no_responde"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"estado": "listo"})
}

// GET /internal/apikey/verificar: el gateway la consulta con auth_request antes de dejar
// pasar una solicitud a /public/v1. Responde 204 si la clave es válida y 401 si no.
// La clave nunca se escribe en el log ni en la traza (ADR-011).
func (h *Interno) verificarAPIKey(c *gin.Context) {
	clave := []byte(c.GetHeader("X-API-Key"))
	if len(clave) > 0 {
		for _, k := range h.claves {
			// ConstantTimeCompare tarda lo mismo acierte o no. Con un == común, el tiempo
			// de respuesta dejaría adivinar la clave carácter por carácter.
			if subtle.ConstantTimeCompare(clave, k) == 1 {
				c.Status(http.StatusNoContent)
				return
			}
		}
	}
	c.Status(http.StatusUnauthorized)
}
