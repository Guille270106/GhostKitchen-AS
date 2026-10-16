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
