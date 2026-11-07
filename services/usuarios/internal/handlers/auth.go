package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"ghostkitchen/usuarios/internal/domain"
	"ghostkitchen/usuarios/internal/services"
)

// TopeCuerpo es el tamaño máximo de un cuerpo JSON. Un registro ocupa menos de 1 KB.
const TopeCuerpo = 16 << 10

// registrarRutasAuth suma las rutas de cuentas propias. El gateway las publica en /api/v1 (G9).
func registrarRutasAuth(r *gin.Engine, svc *services.Usuarios) {
	r.POST("/auth/registro", func(c *gin.Context) {
		var d struct {
			Nombre    string `json:"nombre"`
			Email     string `json:"email"`
			Password  string `json:"password"`
			Direccion string `json:"direccion"`
		}
		if !leerJSON(c, &d) {
			return
		}
		u, err := svc.Registrar(c.Request.Context(), services.DatosRegistro{
			Nombre: d.Nombre, Email: d.Email, Password: d.Password, Direccion: d.Direccion,
		})
		if err != nil {
			responderError(c, err)
			return
		}
		c.Header("Location", "/usuarios/"+u.ID)
		c.JSON(http.StatusCreated, u) // PasswordHash tiene json:"-": nunca sale
	})
}

// leerJSON lee un solo objeto JSON, con tope de tamaño y sin campos desconocidos: así un
// "rol":"admin" mandado en el registro da 400 en lugar de ignorarse en silencio.
func leerJSON(c *gin.Context, destino any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, TopeCuerpo)
	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(destino); err != nil {
		problema(c, http.StatusBadRequest, "DATOS_INVALIDOS", "El cuerpo tiene que ser un objeto JSON con los campos esperados")
		return false
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		problema(c, http.StatusBadRequest, "DATOS_INVALIDOS", "Se espera un único objeto JSON")
		return false
	}
	return true
}

// responderError traduce los errores del servicio a HTTP. Los de datos inválidos llevan un
// texto armado por nosotros (qué dato falta), nunca lo que mandó la persona ni el error de
// la base.
func responderError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrDatosInvalidos):
		problema(c, http.StatusBadRequest, "DATOS_INVALIDOS", motivo(err))
	case errors.Is(err, domain.ErrEmailRepetido):
		// No se dice nada de la otra cuenta (HU01).
		problema(c, http.StatusConflict, "EMAIL_REPETIDO", "Ya existe una cuenta con ese correo")
	case errors.Is(err, domain.ErrNoEncontrado):
		problema(c, http.StatusNotFound, "NO_ENCONTRADO", "Recurso no encontrado")
	default:
		problema(c, http.StatusServiceUnavailable, "SERVICIO_NO_DISPONIBLE", "No se pudo completar la operación; probá de nuevo en unos segundos")
	}
}

// motivo devuelve la parte que explica qué dato falló ("la dirección es obligatoria"). Los
// errores de datos se arman con errors.Join(ErrDatosInvalidos, motivo).
func motivo(err error) string {
	partes := strings.Split(err.Error(), "\n")
	if len(partes) > 1 {
		return partes[len(partes)-1]
	}
	return "Datos inválidos"
}
