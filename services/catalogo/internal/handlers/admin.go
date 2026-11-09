package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"ghostkitchen/catalogo/internal/domain"
	"ghostkitchen/catalogo/internal/services"
)

// Admin atiende las escrituras del admin sobre cocinas y restaurantes (HU14, HU16).
// El gateway las publica en /api/v1/admin/cocinas y antes verifica el token (G9).
type Admin struct {
	svc *services.AdminCatalogo
}

// NuevoAdmin crea los handlers del admin.
func NuevoAdmin(svc *services.AdminCatalogo) *Admin { return &Admin{svc: svc} }

// Registrar agrega las rutas del admin.
func (h *Admin) Registrar(r gin.IRoutes) {
	r.GET("/admin/cocinas", h.listar)
	r.POST("/admin/cocinas", h.crear)
	r.PATCH("/admin/cocinas/:id", h.modificar)
	r.POST("/admin/cocinas/:id/restaurantes", h.agregarRestaurante)
	r.PATCH("/admin/cocinas/:id/restaurantes/:rid", h.modificarRestaurante)
}

// quien lee la identidad que puso el gateway después de verificar el token. Catálogo no se
// publica fuera de Docker, así que nadie puede mandar estos headers salteando el gateway.
// X-Localidad viene codificada para URL ("Nueva%20C%C3%B3rdoba"): un header solo admite ASCII.
func quien(c *gin.Context) (services.Admin, bool) {
	a := services.Admin{UsuarioID: c.GetHeader("X-Usuario-Id"), Rol: strings.ToLower(c.GetHeader("X-Rol"))}
	if a.UsuarioID == "" || a.Rol == "" {
		responderProblema(c, http.StatusUnauthorized, "NO_AUTENTICADO", "Iniciá sesión para continuar.")
		return services.Admin{}, false
	}
	if loc, err := url.PathUnescape(c.GetHeader("X-Localidad")); err == nil {
		a.Localidad = loc
	}
	return a, true
}

func (h *Admin) listar(c *gin.Context) {
	admin, ok := quien(c)
	if !ok {
		return
	}
	cocinas, err := h.svc.CocinasDelAdmin(c.Request.Context(), admin)
	if err != nil {
		responderErrorAdmin(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": cocinas})
}

func (h *Admin) crear(c *gin.Context) {
	admin, ok := quien(c)
	if !ok {
		return
	}
	var datos domain.CambiosCocina
	if !leerJSON(c, &datos) {
		return
	}
	cocina, err := h.svc.CrearCocina(c.Request.Context(), admin, datos)
	if err != nil {
		responderErrorAdmin(c, err)
		return
	}
	c.Header("Location", "/cocinas/"+cocina.ID)
	c.JSON(http.StatusCreated, cocina)
}

func (h *Admin) modificar(c *gin.Context) {
	admin, ok := quien(c)
	if !ok {
		return
	}
	var cambios domain.CambiosCocina
	if !leerJSON(c, &cambios) {
		return
	}
	cocina, err := h.svc.ModificarCocina(c.Request.Context(), admin, c.Param("id"), cambios)
	if err != nil {
		responderErrorAdmin(c, err)
		return
	}
	c.JSON(http.StatusOK, cocina)
}

func (h *Admin) agregarRestaurante(c *gin.Context) {
	admin, ok := quien(c)
	if !ok {
		return
	}
	var datos struct {
		Nombre string `json:"nombre"`
	}
	if !leerJSON(c, &datos) {
		return
	}
	r, err := h.svc.AgregarRestaurante(c.Request.Context(), admin, c.Param("id"), datos.Nombre)
	if err != nil {
		responderErrorAdmin(c, err)
		return
	}
	c.JSON(http.StatusCreated, r)
}

func (h *Admin) modificarRestaurante(c *gin.Context) {
	admin, ok := quien(c)
	if !ok {
		return
	}
	var cambios domain.CambiosRestaurante
	if !leerJSON(c, &cambios) {
		return
	}
	r, err := h.svc.ModificarRestaurante(c.Request.Context(), admin, c.Param("id"), c.Param("rid"), cambios)
	if err != nil {
		responderErrorAdmin(c, err)
		return
	}
	c.JSON(http.StatusOK, r)
}

// leerJSON lee un solo objeto JSON de hasta 16 KiB, sin campos desconocidos: un
// "localidad" mandado por el cliente da 400 en vez de ignorarse en silencio.
func leerJSON(c *gin.Context, destino any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(destino); err != nil {
		responderProblema(c, http.StatusBadRequest, "DATOS_INVALIDOS", "El cuerpo tiene que ser un objeto JSON con los campos esperados.")
		return false
	}
	if dec.Decode(new(any)) != io.EOF {
		responderProblema(c, http.StatusBadRequest, "DATOS_INVALIDOS", "Se espera un único objeto JSON.")
		return false
	}
	return true
}

// responderErrorAdmin traduce los errores de las escrituras. Lo que no es una regla del
// negocio es una falla de MongoDB: 503, como en las lecturas.
func responderErrorAdmin(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrDatosInvalidos):
		_, motivo, _ := strings.Cut(err.Error(), ": ")
		if motivo == "" {
			motivo = "Datos inválidos."
		}
		responderProblema(c, http.StatusBadRequest, "DATOS_INVALIDOS", motivo)
	case errors.Is(err, domain.ErrSinPermiso):
		responderProblema(c, http.StatusForbidden, "SIN_PERMISO", "Solo un admin puede administrar cocinas y restaurantes.")
	case errors.Is(err, domain.ErrOtraLocalidad):
		responderProblema(c, http.StatusForbidden, "OTRA_LOCALIDAD", "La cocina no es de tu localidad.")
	case errors.Is(err, domain.ErrCocinaNoEncontrada):
		responderProblema(c, http.StatusNotFound, "COCINA_NO_ENCONTRADA", "La cocina no existe.")
	case errors.Is(err, domain.ErrRestauranteNoEncontrado):
		responderProblema(c, http.StatusNotFound, "RESTAURANTE_NO_ENCONTRADO", "El restaurante no existe en esa cocina.")
	case errors.Is(err, domain.ErrYaExiste):
		responderProblema(c, http.StatusConflict, "COCINA_REPETIDA", "Ya existe una cocina con ese nombre.")
	case errors.Is(err, domain.ErrCocinaLlena):
		responderProblema(c, http.StatusConflict, "COCINA_LLENA", "La cocina ya tiene 4 restaurantes.")
	case errors.Is(err, domain.ErrRestauranteRepetido):
		responderProblema(c, http.StatusConflict, "RESTAURANTE_REPETIDO", "Ya hay un restaurante con ese nombre en la cocina.")
	case errors.Is(err, domain.ErrConflicto):
		c.Header("Retry-After", "1")
		responderProblema(c, http.StatusConflict, "CONFLICTO", "La cocina cambió mientras la modificabas. Probá de nuevo.")
	default:
		responderError(c, err)
	}
}
