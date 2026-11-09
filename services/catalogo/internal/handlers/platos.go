package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"ghostkitchen/catalogo/internal/domain"
	"ghostkitchen/catalogo/internal/middleware"
	"ghostkitchen/catalogo/internal/services"
)

// Platos atiende las escrituras de la cuenta Cocina sobre sus platos (HU17). El gateway las
// publica en /api/v1/cocina/platos y /api/v1/cocina/restaurantes/{rid}/platos (G9).
type Platos struct {
	svc *services.PlatosCocina
}

// NuevoPlatos crea los handlers de la cocina.
func NuevoPlatos(svc *services.PlatosCocina) *Platos { return &Platos{svc: svc} }

// Registrar agrega las rutas de la cocina.
func (h *Platos) Registrar(r gin.IRoutes) {
	r.GET("/cocina/platos", h.listar)
	r.POST("/cocina/restaurantes/:rid/platos", h.crear)
	r.PATCH("/cocina/platos/:id", h.modificar)
}

// cuentaCocina lee la identidad que puso el gateway. La cocina sale de X-Cocina-Id, nunca
// del cuerpo: una cuenta Cocina solo toca los platos de su cocina.
func cuentaCocina(c *gin.Context) (services.Cocina, bool) {
	q := services.Cocina{UsuarioID: c.GetHeader("X-Usuario-Id"), Rol: strings.ToLower(c.GetHeader("X-Rol")), CocinaID: c.GetHeader("X-Cocina-Id")}
	if q.UsuarioID == "" || q.Rol == "" {
		responderProblema(c, http.StatusUnauthorized, "NO_AUTENTICADO", "Iniciá sesión para continuar.")
		return services.Cocina{}, false
	}
	return q, true
}

func (h *Platos) listar(c *gin.Context) {
	quien, ok := cuentaCocina(c)
	if !ok {
		return
	}
	platos, err := h.svc.MisPlatos(c.Request.Context(), quien)
	if err != nil {
		responderErrorAdmin(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": platos})
}

func (h *Platos) crear(c *gin.Context) {
	quien, ok := cuentaCocina(c)
	if !ok {
		return
	}
	var datos domain.CambiosPlato
	if !leerJSON(c, &datos) {
		return
	}
	p, err := h.svc.CrearPlato(c.Request.Context(), quien, c.Param("rid"), datos)
	if err != nil {
		responderErrorAdmin(c, err)
		return
	}
	c.Header("Location", "/platos/"+p.ID)
	c.JSON(http.StatusCreated, p)
}

func (h *Platos) modificar(c *gin.Context) {
	quien, ok := cuentaCocina(c)
	if !ok {
		return
	}
	var cambios domain.CambiosPlato
	if !leerJSON(c, &cambios) {
		return
	}
	p, err := h.svc.ModificarPlato(c.Request.Context(), quien, c.Param("id"), cambios, middleware.IDCorrelacion(c))
	if err != nil {
		responderErrorAdmin(c, err)
		return
	}
	c.JSON(http.StatusOK, p)
}
