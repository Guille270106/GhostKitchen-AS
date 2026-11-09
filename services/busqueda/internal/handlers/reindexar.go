package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"ghostkitchen/busqueda/internal/indexer"
	"ghostkitchen/busqueda/internal/middleware"
)

// Códigos de POST /internal/reindexar. Es una ruta técnica: no está en el contrato público,
// pero responde con el mismo formato de error (Problem Details).
const (
	CodigoReconstruccionEnCurso = "RECONSTRUCCION_EN_CURSO"
	CodigoSinCatalogo           = "SIN_CATALOGO"
	segundosReintentoEnCurso    = "10"
)

// PlazoReindexar es el tiempo máximo de una reconstrucción manual. Es menor que el
// WriteTimeout del servidor (30 s), para que la respuesta llegue siempre.
const PlazoReindexar = 25 * time.Second

func init() {
	titulos[CodigoReconstruccionEnCurso] = struct{ slug, titulo string }{"reconstruccion-en-curso", "Reconstrucción en curso"}
	titulos[CodigoSinCatalogo] = struct{ slug, titulo string }{"sin-catalogo", "Sin Catálogo configurado"}
}

// Reindexar atiende POST /internal/reindexar: vuelve a armar el índice desde Catálogo e
// Inventario. Es una operación técnica: el gateway no la publica (bloquea /internal/).
type Reindexar struct {
	excl *indexer.Exclusiva
}

// NuevoReindexar crea el handler. Si excl es nil (no hay CATALOGO_URL), la ruta responde 409.
// La misma Exclusiva la usa el arranque: nunca corren dos reconstrucciones a la vez (G22).
func NuevoReindexar(excl *indexer.Exclusiva) *Reindexar {
	return &Reindexar{excl: excl}
}

// Registrar agrega la ruta.
func (h *Reindexar) Registrar(r gin.IRoutes) {
	r.POST("/internal/reindexar", h.reindexar)
}

// problemaReindexar es el Problem Details con lo que se llegó a guardar si falló a mitad.
type problemaReindexar struct {
	Problema
	Parcial *indexer.Resultado `json:"parcial,omitempty"`
}

// reindexar corre la reconstrucción con su propio contexto (G22): si quien la pidió corta la
// conexión, no se interrumpe a mitad, y tiene su propio plazo. Conserva el correlation_id,
// así se sigue en los logs de Búsqueda, Catálogo e Inventario.
//
// Responde 200 con lo indexado; 409 si ya hay otra en curso (con Retry-After) o si no hay
// Catálogo configurado; 503 con lo que se llegó a guardar ("parcial") si Catálogo o Solr
// fallan: el índice anterior sigue sirviendo.
func (h *Reindexar) reindexar(c *gin.Context) {
	if h.excl == nil {
		responderReindexar(c, http.StatusConflict, CodigoSinCatalogo, "No hay CATALOGO_URL configurado: el índice usa los datos iniciales.", nil)
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), PlazoReindexar)
	defer cancel()
	res, err := h.excl.Intentar(ctx)
	switch {
	case errors.Is(err, indexer.ErrEnCurso):
		c.Header("Retry-After", segundosReintentoEnCurso)
		responderReindexar(c, http.StatusConflict, CodigoReconstruccionEnCurso, "Ya hay una reconstrucción en curso. Probá de nuevo cuando termine.", nil)
	case err != nil:
		_ = c.Error(err)
		responderReindexar(c, http.StatusServiceUnavailable, CodigoNoDisponible, "No se pudo reconstruir el índice: el índice anterior sigue sirviendo.", &res)
	default:
		c.JSON(http.StatusOK, res)
	}
}

func responderReindexar(c *gin.Context, estado int, codigo, detalle string, parcial *indexer.Resultado) {
	t := titulos[codigo]
	if estado == http.StatusServiceUnavailable {
		c.Header("Retry-After", segundosReintentoNoDisponib)
	}
	c.Header("Content-Type", tipoContenidoProblema)
	c.AbortWithStatusJSON(estado, problemaReindexar{
		Problema: Problema{
			Type: baseTipos + t.slug, Title: t.titulo, Status: estado, Code: codigo,
			Detail: detalle, CorrelationID: middleware.IDCorrelacion(c),
		},
		Parcial: parcial,
	})
}
