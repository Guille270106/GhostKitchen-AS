package handlers

import (
	"context"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"

	"ghostkitchen/busqueda/internal/indexer"
)

// Reindexar atiende POST /internal/reindexar: vuelve a armar el índice desde Catálogo.
// Es una operación técnica: el gateway no la publica (bloquea /internal/).
//
// enCurso permite una sola reconstrucción manual a la vez: dos en paralelo duplicarían la
// carga sobre Catálogo y Solr sin ninguna ventaja (bulkhead, ADR-010).
type Reindexar struct {
	reconstruir func(context.Context) (indexer.Resultado, error)
	enCurso     sync.Mutex
}

// NuevoReindexar crea el handler. Si reconstruir es nil, la ruta responde 409 (no hay Catálogo configurado).
func NuevoReindexar(reconstruir func(context.Context) (indexer.Resultado, error)) *Reindexar {
	return &Reindexar{reconstruir: reconstruir}
}

// Registrar agrega la ruta.
func (h *Reindexar) Registrar(r gin.IRoutes) {
	r.POST("/internal/reindexar", h.reindexar)
}

// reindexar responde 409 en vez de esperar si ya hay otra en curso, y 503 con lo que se llegó
// a guardar ("parcial") si Catálogo o Solr fallan: el índice anterior sigue sirviendo.
func (h *Reindexar) reindexar(c *gin.Context) {
	if h.reconstruir == nil {
		c.JSON(http.StatusConflict, gin.H{"detail": "No hay CATALOGO_URL configurado: el índice usa los datos iniciales."})
		return
	}
	if !h.enCurso.TryLock() {
		c.JSON(http.StatusConflict, gin.H{"detail": "Ya hay una reconstrucción en curso."})
		return
	}
	defer h.enCurso.Unlock()
	res, err := h.reconstruir(c.Request.Context())
	if err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"detail": "No se pudo reconstruir el índice.", "parcial": res})
		return
	}
	c.JSON(http.StatusOK, res)
}
