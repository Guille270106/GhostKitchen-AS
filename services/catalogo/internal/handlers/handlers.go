// Package handlers traduce HTTP a llamadas al servicio de Catálogo.
package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"ghostkitchen/catalogo/internal/domain"
	"ghostkitchen/catalogo/internal/middleware"
	"ghostkitchen/catalogo/internal/services"
)

// Catalogo atiende las lecturas públicas, las internas y las rutas técnicas.
type Catalogo struct {
	svc  *services.Catalogo
	ping func(context.Context) error
}

// Nuevo crea los handlers.
func Nuevo(svc *services.Catalogo, ping func(context.Context) error) *Catalogo {
	return &Catalogo{svc: svc, ping: ping}
}

// Registrar agrega las rutas. Las de /internal/ solo se usan dentro de la red de Docker:
// el gateway no las publica.
func (h *Catalogo) Registrar(r gin.IRoutes) {
	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"estado": "ok"}) })
	r.GET("/ready", h.preparado)

	r.GET("/cocinas", h.listarCocinas)
	r.GET("/cocinas/:id", h.obtenerCocina)
	r.GET("/cocinas/:id/platos", h.menu)
	r.GET("/platos/:id", h.obtenerPlato)

	r.GET("/internal/cocinas", h.todasLasCocinas)
	r.GET("/internal/platos", h.paginaPlatos)
}

func (h *Catalogo) preparado(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
	defer cancel()
	if err := h.ping(ctx); err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"estado": "mongodb_no_responde"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"estado": "listo"})
}

func (h *Catalogo) listarCocinas(c *gin.Context) {
	cocinas, err := h.svc.ListarCocinas(c.Request.Context())
	if err != nil {
		responderError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": cocinas})
}

func (h *Catalogo) obtenerCocina(c *gin.Context) {
	cocina, err := h.svc.ObtenerCocina(c.Request.Context(), c.Param("id"))
	if err != nil {
		responderError(c, err)
		return
	}
	c.JSON(http.StatusOK, cocina)
}

func (h *Catalogo) menu(c *gin.Context) {
	cocina, platos, err := h.svc.Menu(c.Request.Context(), c.Param("id"))
	if err != nil {
		responderError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"cocina": gin.H{"id": cocina.ID, "nombre": cocina.Nombre}, "items": platos})
}

func (h *Catalogo) obtenerPlato(c *gin.Context) {
	p, err := h.svc.ObtenerPlato(c.Request.Context(), c.Param("id"))
	if err != nil {
		responderError(c, err)
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *Catalogo) todasLasCocinas(c *gin.Context) {
	cocinas, err := h.svc.TodasLasCocinas(c.Request.Context())
	if err != nil {
		responderError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": cocinas})
}

// GET /internal/platos?page=1&pageSize=100
func (h *Catalogo) paginaPlatos(c *gin.Context) {
	page, err1 := enteroQuery(c, "page", 1, 1, 1_000_000)
	size, err2 := enteroQuery(c, "pageSize", 100, 1, services.MaxTamPagina)
	if err := errors.Join(err1, err2); err != nil {
		responderProblema(c, http.StatusBadRequest, "PARAMETRO_INVALIDO", err.Error())
		return
	}
	pag, err := h.svc.PaginaPlatos(c.Request.Context(), page, size)
	if err != nil {
		responderError(c, err)
		return
	}
	c.JSON(http.StatusOK, pag)
}

func enteroQuery(c *gin.Context, nombre string, porDefecto, min, max int) (int, error) {
	v, ok := c.GetQuery(nombre)
	if !ok {
		return porDefecto, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < min || n > max {
		return 0, fmt.Errorf("%s debe ser un entero entre %d y %d", nombre, min, max)
	}
	return n, nil
}

func responderError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrCocinaNoEncontrada):
		responderProblema(c, http.StatusNotFound, "COCINA_NO_ENCONTRADA", "La cocina no existe o no está activa.")
	case errors.Is(err, domain.ErrPlatoNoEncontrado):
		responderProblema(c, http.StatusNotFound, "PLATO_NO_ENCONTRADO", "El plato no existe o fue desactivado.")
	default:
		_ = c.Error(err)
		c.Header("Retry-After", "5")
		responderProblema(c, http.StatusServiceUnavailable, "SERVICIO_NO_DISPONIBLE", "El catálogo no está disponible en este momento.")
	}
}

func responderProblema(c *gin.Context, estado int, codigo, detalle string) {
	c.Header("Content-Type", "application/problem+json")
	c.AbortWithStatusJSON(estado, gin.H{
		"type":          "https://ghostkitchen.dev/errores/" + codigo,
		"title":         http.StatusText(estado),
		"status":        estado,
		"code":          codigo,
		"detail":        detalle,
		"correlationId": middleware.IDCorrelacion(c),
	})
}
