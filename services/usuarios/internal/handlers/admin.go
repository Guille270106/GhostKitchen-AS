package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"ghostkitchen/usuarios/internal/services"
)

// registrarRutasAdmin suma las rutas del admin. El gateway las publica en /api/v1.
func registrarRutasAdmin(r *gin.Engine, svc *services.Usuarios) {
	// El admin crea la cuenta de una cocina de su localidad.
	r.POST("/admin/cuentas-cocina", func(c *gin.Context) {
		admin, ok := identidad(c, svc)
		if !ok {
			return
		}
		var d struct {
			Nombre   string `json:"nombre"`
			Email    string `json:"email"`
			Password string `json:"password"`
			CocinaID string `json:"cocinaId"`
		}
		if !leerJSON(c, &d) {
			return
		}
		u, err := svc.CrearCuentaCocina(c.Request.Context(), admin, services.DatosCuentaCocina{
			Nombre: d.Nombre, Email: d.Email, Password: d.Password, CocinaID: d.CocinaID,
		}, c.GetString("cid"))
		if err != nil {
			responderError(c, err)
			return
		}
		c.Header("Location", "/usuarios/"+u.ID)
		c.JSON(http.StatusCreated, u)
	})
}
