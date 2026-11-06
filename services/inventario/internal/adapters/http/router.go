package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"ghostkitchen/inventario/internal/application"
	"ghostkitchen/inventario/internal/domain"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var cidValido = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)

// Estas cabeceras son identidad interna del gateway, nunca parámetros del cliente.
// V5 no publica el servicio al host ni agrega rutas del gateway (G9 pendiente).
func identidad(c *gin.Context) domain.Identidad {
	return domain.Identidad{UsuarioID: c.GetHeader("X-Usuario-Id"), Rol: strings.ToLower(c.GetHeader("X-Rol")), CocinaID: c.GetHeader("X-Cocina-Id")}
}

func Nuevo(svc *application.Ingredientes, ready func(context.Context) error, log *slog.Logger) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(func(c *gin.Context) {
		cid := c.GetHeader("X-Correlation-Id")
		if !cidValido.MatchString(cid) {
			cid = uuid.NewString()
		}
		c.Set("cid", cid)
		c.Header("X-Correlation-Id", cid)
		inicio := time.Now()
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
		log.Info("solicitud", "servicio", "inventario", "correlation_id", cid, "ruta", c.FullPath(), "metodo", c.Request.Method, "estado", c.Writer.Status(), "duracion_ms", time.Since(inicio).Milliseconds())
	})
	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"estado": "ok"}) })
	r.GET("/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
		defer cancel()
		if ready(ctx) != nil {
			fallo(c, 503, "SERVICIO_NO_DISPONIBLE", "MySQL o esquema no disponible")
			return
		}
		c.JSON(200, gin.H{"estado": "listo"})
	})
	cocina := r.Group("/cocina", autenticarCocina)
	cocina.POST("/ingredientes", func(c *gin.Context) {
		var datos struct {
			Nombre       string `json:"nombre"`
			Unidad       string `json:"unidad"`
			StockInicial *int64 `json:"stockInicial"`
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64*1024)
		decoder := json.NewDecoder(c.Request.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&datos); err != nil {
			fallo(c, 400, "DATOS_INVALIDOS", "Enviá nombre, unidad y stockInicial entero no negativo")
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF || datos.StockInicial == nil {
			fallo(c, 400, "DATOS_INVALIDOS", "Se requiere un único objeto JSON con stockInicial")
			return
		}
		i, err := svc.Crear(c.Request.Context(), identidad(c), datos.Nombre, datos.Unidad, *datos.StockInicial)
		if err != nil {
			responderError(c, err)
			return
		}
		c.Header("Location", "/cocina/ingredientes/"+i.ID)
		c.JSON(201, i)
	})
	cocina.GET("/ingredientes", func(c *gin.Context) {
		items, err := svc.Listar(c.Request.Context(), identidad(c))
		if err != nil {
			responderError(c, err)
			return
		}
		c.JSON(200, gin.H{"items": items})
	})
	cocina.GET("/ingredientes/:id", func(c *gin.Context) {
		item, err := svc.Obtener(c.Request.Context(), identidad(c), c.Param("id"))
		if err != nil {
			responderError(c, err)
			return
		}
		c.JSON(200, item)
	})
	r.NoRoute(func(c *gin.Context) { fallo(c, 404, "NO_ENCONTRADO", "Recurso no encontrado") })
	return r
}

func responderError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrRecetaInvalida):
		fallo(c, 400, "RECETA_INVALIDA", err.Error())
	case errors.Is(err, domain.ErrDatosInvalidos):
		fallo(c, 400, "DATOS_INVALIDOS", err.Error())
	case errors.Is(err, domain.ErrSinIdentidad):
		fallo(c, 401, "NO_AUTENTICADO", err.Error())
	case errors.Is(err, domain.ErrSinPermiso):
		fallo(c, 403, "SIN_PERMISO", err.Error())
	case errors.Is(err, domain.ErrNoEncontrado):
		fallo(c, 404, "NO_ENCONTRADO", "Recurso no encontrado")
	default:
		fallo(c, 503, "SERVICIO_NO_DISPONIBLE", "No se pudo completar la operación")
	}
}

func fallo(c *gin.Context, status int, code, detail string) {
	c.Header("Content-Type", "application/problem+json")
	c.JSON(status, gin.H{"type": "https://ghostkitchen.dev/errores/" + strings.ToLower(strings.ReplaceAll(code, "_", "-")), "title": http.StatusText(status), "status": status, "code": code, "detail": detail, "correlationId": c.GetString("cid")})
}
