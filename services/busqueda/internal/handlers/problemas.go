// Package handlers traduce HTTP a llamadas a los servicios y los errores a Problem Details (RFC 9457).
package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"ghostkitchen/busqueda/internal/domain"
	"ghostkitchen/busqueda/internal/middleware"
)

// Códigos estables del contrato.
const (
	CodigoParametroInvalido     = "PARAMETRO_INVALIDO"
	CodigoAPIKeyInvalida        = "API_KEY_INVALIDA"
	CodigoSedeNoEncontrada      = "SEDE_NO_ENCONTRADA"
	CodigoPlatoNoEncontrado     = "PLATO_NO_ENCONTRADO"
	CodigoSinCobertura          = "SIN_COBERTURA"
	CodigoNoDisponible          = "SERVICIO_NO_DISPONIBLE"
	CodigoSolicitudMuyGrande    = "SOLICITUD_DEMASIADO_GRANDE"
	baseTipos                   = "https://ghostkitchen.dev/errores/"
	tipoContenidoProblema       = "application/problem+json"
	segundosReintentoNoDisponib = "5"
)

// ErrorParametro describe un parámetro inválido en una respuesta 400.
type ErrorParametro struct {
	Parametro string `json:"parametro"`
	Mensaje   string `json:"mensaje"`
}

// Problema es el cuerpo de error del contrato.
type Problema struct {
	Type          string           `json:"type"`
	Title         string           `json:"title"`
	Status        int              `json:"status"`
	Code          string           `json:"code"`
	Detail        string           `json:"detail,omitempty"`
	CorrelationID string           `json:"correlationId,omitempty"`
	Errores       []ErrorParametro `json:"errores,omitempty"`
}

var titulos = map[string]struct{ slug, titulo string }{
	CodigoParametroInvalido:  {"parametro-invalido", "Parámetro inválido"},
	CodigoAPIKeyInvalida:     {"api-key-invalida", "API key inválida"},
	CodigoSedeNoEncontrada:   {"sede-no-encontrada", "Sede no encontrada"},
	CodigoPlatoNoEncontrado:  {"plato-no-encontrado", "Plato no encontrado"},
	CodigoSinCobertura:       {"sin-cobertura", "Sin cobertura"},
	CodigoNoDisponible:       {"servicio-no-disponible", "Servicio no disponible"},
	CodigoSolicitudMuyGrande: {"solicitud-demasiado-grande", "Solicitud demasiado grande"},
}

// responderProblema escribe un error con el formato del contrato y corta la cadena de handlers.
func responderProblema(c *gin.Context, estado int, codigo, detalle string, errores ...ErrorParametro) {
	t := titulos[codigo]
	if estado == http.StatusServiceUnavailable {
		c.Header("Retry-After", segundosReintentoNoDisponib)
	}
	c.Header("Content-Type", tipoContenidoProblema)
	c.AbortWithStatusJSON(estado, Problema{
		Type:          baseTipos + t.slug,
		Title:         t.titulo,
		Status:        estado,
		Code:          codigo,
		Detail:        detalle,
		CorrelationID: middleware.IDCorrelacion(c),
		Errores:       errores,
	})
}

// parametrosInvalidos responde 400 con el detalle de cada parámetro.
func parametrosInvalidos(c *gin.Context, errores []ErrorParametro) {
	responderProblema(c, http.StatusBadRequest, CodigoParametroInvalido,
		"Hay parámetros con valores no permitidos.", errores...)
}

// responderError traduce un error de los servicios. Cualquier falla del índice es 503:
// nunca se devuelve una lista vacía para disimularla.
func responderError(c *gin.Context, err error, detalle404 string) {
	switch {
	case errors.Is(err, domain.ErrSedeNoEncontrada):
		responderProblema(c, http.StatusNotFound, CodigoSedeNoEncontrada, detalle404)
	case errors.Is(err, domain.ErrPlatoNoEncontrado):
		responderProblema(c, http.StatusNotFound, CodigoPlatoNoEncontrado, detalle404)
	case errors.Is(err, domain.ErrSinCobertura):
		responderProblema(c, http.StatusNotFound, CodigoSinCobertura,
			"No hay sedes activas a menos de 25 km de esa ubicación.")
	default:
		_ = c.Error(err) // queda en el log de la solicitud
		responderProblema(c, http.StatusServiceUnavailable, CodigoNoDisponible,
			"La búsqueda no está disponible en este momento.")
	}
}
