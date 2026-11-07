package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
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

	// Login de los tres roles (HU02). El cierre de sesión lo hace el frontend descartando el
	// token: no hay ruta de logout porque el token no se guarda en el servidor.
	r.POST("/auth/login", func(c *gin.Context) {
		var d struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if !leerJSON(c, &d) {
			return
		}
		sesion, err := svc.Login(c.Request.Context(), d.Email, d.Password)
		if err != nil {
			responderError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store") // el token no se guarda en cachés intermedias
		c.JSON(http.StatusOK, sesion)
	})

	// Verificación para el gateway (auth_request, G9). Responde 204 sin cuerpo y la identidad
	// en headers, que el gateway copia hacia los servicios. Con un token que no sirve, 401.
	r.GET("/auth/verify", func(c *gin.Context) {
		token, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
		if !ok || token == "" {
			noAutenticado(c)
			return
		}
		id, err := svc.Verificar(strings.TrimSpace(token))
		if err != nil {
			noAutenticado(c)
			return
		}
		c.Header("X-Usuario-Id", id.UsuarioID)
		c.Header("X-Usuario-Email", id.Email)
		c.Header("X-Rol", string(id.Rol))
		if id.CocinaID != "" {
			c.Header("X-Cocina-Id", id.CocinaID)
		}
		if id.Localidad != "" {
			// Un header solo admite ASCII: "Nueva Córdoba" viaja como "Nueva%20C%C3%B3rdoba".
			c.Header("X-Localidad", url.PathEscape(id.Localidad))
		}
		c.Status(http.StatusNoContent)
	})
}

func noAutenticado(c *gin.Context) {
	c.Header("WWW-Authenticate", `Bearer realm="ghostkitchen"`)
	problema(c, http.StatusUnauthorized, "NO_AUTENTICADO", "Falta el token o no es válido")
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
	case errors.Is(err, services.ErrCredencialesInvalidas):
		// El mismo mensaje si no existe el correo o si la contraseña está mal (HU02).
		problema(c, http.StatusUnauthorized, "CREDENCIALES_INVALIDAS", "Correo o contraseña incorrectos")
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
