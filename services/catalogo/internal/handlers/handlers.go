// Package handlers traduce HTTP a llamadas al servicio de Catálogo.
//
// Es la capa de entrada (ADR-002): lee parámetros, llama a services y elige el código HTTP.
// No sabe que existe MongoDB.
package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
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
	// fuente lee directo de MongoDB, sin caché. La usan las rutas internas con las que otros
	// servicios toman una decisión (el precio al agregar al carrito, la cocina de una cuenta):
	// ahí no puede haber un dato viejo de la caché (ADR-007).
	fuente *services.Catalogo
}

// Nuevo crea los handlers.
func Nuevo(svc *services.Catalogo, ping func(context.Context) error) *Catalogo {
	return &Catalogo{svc: svc, ping: ping, fuente: svc}
}

// ConFuente indica de dónde leen las rutas internas de un plato o una cocina (MongoDB sin
// caché). Sin llamarla, leen de lo mismo que las públicas.
func (h *Catalogo) ConFuente(fuente *services.Catalogo) *Catalogo {
	h.fuente = fuente
	return h
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
	// Un plato o una cocina, leídos de la fuente de verdad (sin caché). Responden lo mismo que
	// las públicas: lo desactivado da 404.
	r.GET("/internal/platos/:id", h.obtenerPlatoFuente)
	r.GET("/internal/cocinas/:id", h.obtenerCocinaFuente)
}

// preparado atiende GET /ready: MongoDB responde en menos de 1 s. /health, en cambio, solo
// dice que el proceso está vivo, porque reiniciar Catálogo no arregla MongoDB (ADR-010).
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

func (h *Catalogo) obtenerPlatoFuente(c *gin.Context) {
	p, err := h.fuente.ObtenerPlato(c.Request.Context(), c.Param("id"))
	if err != nil {
		responderError(c, err)
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *Catalogo) obtenerCocinaFuente(c *gin.Context) {
	cocina, err := h.fuente.ObtenerCocina(c.Request.Context(), c.Param("id"))
	if err != nil {
		responderError(c, err)
		return
	}
	c.JSON(http.StatusOK, cocina)
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

// GET /internal/platos?page=1&pageSize=100. La usa Búsqueda para reconstruir su índice
// sin entrar a catalogo_db (ADR-001). El tamaño de página tiene un máximo para que nadie
// pueda pedir toda la colección de una vez.
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

// enteroQuery lee un entero opcional de la query y controla que esté entre min y max.
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

// responderError traduce los errores del servicio. Lo que no es "no existe" es una falla de
// MongoDB y se responde 503 con Retry-After, nunca una lista vacía (ADR-010).
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

// titulos es el título legible de cada código, en castellano: el mismo formato que Búsqueda,
// Pedidos e Inventario. Si falta uno, se usa uno general según el código HTTP.
var titulos = map[string]string{
	"COCINA_NO_ENCONTRADA":      "Cocina no encontrada",
	"RESTAURANTE_NO_ENCONTRADO": "Restaurante no encontrado",
	"PLATO_NO_ENCONTRADO":       "Plato no encontrado",
	"COCINA_LLENA":              "La cocina ya tiene 4 restaurantes",
	"COCINA_REPETIDA":           "Cocina repetida",
	"RESTAURANTE_REPETIDO":      "Restaurante repetido",
	"CONFLICTO":                 "Conflicto",
	"DATOS_INVALIDOS":           "Datos inválidos",
	"PARAMETRO_INVALIDO":        "Parámetro inválido",
	"NO_AUTENTICADO":            "No autenticado",
	"SIN_PERMISO":               "Sin permiso",
	"OTRA_COCINA":               "Es de otra cocina",
	"OTRA_LOCALIDAD":            "Es de otra localidad",
	"SIN_RECETA":                "El plato no tiene receta",
	"INVENTARIO_NO_DISPONIBLE":  "Inventario no disponible",
	"SERVICIO_NO_DISPONIBLE":    "Servicio no disponible",
}

// titulosPorEstado se usa si un código no está en titulos.
var titulosPorEstado = map[int]string{400: "Datos inválidos", 401: "No autenticado", 403: "Sin permiso",
	404: "No encontrado", 409: "Conflicto", 503: "Servicio no disponible"}

// responderProblema escribe el error en formato Problem Details (RFC 9457), con el
// correlationId para que quien llama pueda encontrar la solicitud en nuestros logs.
// "type" termina en el código en minúsculas y con guiones (SIN_RECETA -> sin-receta) y
// "title" va en castellano, igual que en el resto de los servicios.
func responderProblema(c *gin.Context, estado int, codigo, detalle string) {
	titulo, ok := titulos[codigo]
	if !ok {
		titulo = titulosPorEstado[estado]
	}
	c.Header("Content-Type", "application/problem+json")
	c.AbortWithStatusJSON(estado, gin.H{
		"type":          "https://ghostkitchen.dev/errores/" + strings.ToLower(strings.ReplaceAll(codigo, "_", "-")),
		"title":         titulo,
		"status":        estado,
		"code":          codigo,
		"detail":        detalle,
		"correlationId": middleware.IDCorrelacion(c),
	})
}
