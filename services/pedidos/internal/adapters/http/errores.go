package http

import (
	"errors"
	"net/http"
	"strings"

	"ghostkitchen/pedidos/internal/domain"
	"github.com/gin-gonic/gin"
)

// responderError traduce los errores del dominio a códigos HTTP.
func responderError(ctx *gin.Context, err error) {
	switch {
	// Identidad y permisos.
	case errors.Is(err, domain.ErrSinIdentidad):
		fallo(ctx, 401, "NO_AUTENTICADO", "Falta la identidad verificada por el gateway")
	case errors.Is(err, domain.ErrSinPermiso):
		fallo(ctx, 403, "SIN_PERMISO", err.Error())

	// No existe.
	case errors.Is(err, domain.ErrNoEncontrado):
		fallo(ctx, 404, "PEDIDO_NO_ENCONTRADO", "El pedido no existe")
	case errors.Is(err, domain.ErrPlatoNoEncontrado):
		fallo(ctx, 404, "PLATO_NO_ENCONTRADO", err.Error())
	case errors.Is(err, domain.ErrNoEstaEnElCarrito):
		fallo(ctx, 404, "NO_ESTA_EN_EL_CARRITO", err.Error())

	// Datos mal enviados.
	case errors.Is(err, domain.ErrDatosInvalidos):
		fallo(ctx, 400, "DATOS_INVALIDOS", err.Error())
	case errors.Is(err, domain.ErrCantidadInvalida):
		fallo(ctx, 400, "CANTIDAD_INVALIDA", err.Error())

	// La regla de negocio no lo permite ahora.
	case errors.Is(err, domain.ErrOtraCocina):
		fallo(ctx, 409, "OTRA_COCINA", err.Error())
	case errors.Is(err, domain.ErrSinStock):
		fallo(ctx, 409, "SIN_STOCK", err.Error())
	case errors.Is(err, domain.ErrPlatoNoDisponible):
		fallo(ctx, 409, "PLATO_NO_DISPONIBLE", err.Error())
	case errors.Is(err, domain.ErrCarritoVencido):
		fallo(ctx, 409, "CARRITO_VENCIDO", err.Error())
	case errors.Is(err, domain.ErrCarritoModificado):
		fallo(ctx, 409, "CARRITO_MODIFICADO", err.Error())
	case errors.Is(err, domain.ErrTransicionInvalida):
		fallo(ctx, 409, "TRANSICION_INVALIDA", err.Error())
	case errors.Is(err, domain.ErrConflictoVersion):
		fallo(ctx, 409, "PEDIDO_MODIFICADO", "Otro cambio se guardó primero; volvé a consultar el pedido")

	// Una dependencia no responde: 503 honesto, nunca un dato inventado.
	case errors.Is(err, domain.ErrCatalogoCaido), errors.Is(err, domain.ErrInventarioCaido):
		ctx.Header("Retry-After", "5")
		fallo(ctx, 503, "SERVICIO_NO_DISPONIBLE", err.Error())
	default:
		_ = ctx.Error(err)
		ctx.Header("Retry-After", "5")
		fallo(ctx, 503, "SERVICIO_NO_DISPONIBLE", "No se pudo completar la operación")
	}
}

// fallo responde con Problem Details, con el mismo formato que Inventario y Búsqueda.
func fallo(ctx *gin.Context, estado int, codigo, detalle string) {
	ctx.Header("Content-Type", "application/problem+json")
	ctx.AbortWithStatusJSON(estado, gin.H{
		"type":          "https://ghostkitchen.dev/errores/" + strings.ToLower(strings.ReplaceAll(codigo, "_", "-")),
		"title":         http.StatusText(estado),
		"status":        estado,
		"code":          codigo,
		"detail":        detalle,
		"correlationId": ctx.GetString("cid"),
	})
}
