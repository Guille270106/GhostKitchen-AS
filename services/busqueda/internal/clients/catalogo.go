// Package clients tiene los clientes HTTP hacia otros servicios.
// Búsqueda solo los usa para reconstruir su índice: nunca entra a las bases de otros (ADR-001).
package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"ghostkitchen/busqueda/internal/domain"
	"ghostkitchen/busqueda/internal/middleware"
)

// CocinaCatalogo es una cocina tal como la devuelve Catálogo en /internal/cocinas.
type CocinaCatalogo struct {
	ID               string           `json:"id"`
	Nombre           string           `json:"nombre"`
	Localidad        string           `json:"localidad"`
	Direccion        string           `json:"direccion"`
	Ubicacion        domain.Ubicacion `json:"ubicacion"`
	TiempoEntregaMin int              `json:"tiempoEntregaMin"`
	TiempoEntregaMax int              `json:"tiempoEntregaMax"`
	CostoEnvio       int              `json:"costoEnvio"`
	Activa           bool             `json:"activa"`
	Restaurantes     []struct {
		ID     string `json:"id"`
		Nombre string `json:"nombre"`
		Activo bool   `json:"activo"`
	} `json:"restaurantes"`
}

// PlatoCatalogo es un plato tal como lo devuelve Catálogo en /internal/platos.
type PlatoCatalogo struct {
	ID            string    `json:"id"`
	CocinaID      string    `json:"cocinaId"`
	RestauranteID string    `json:"restauranteId"`
	Nombre        string    `json:"nombre"`
	Descripcion   string    `json:"descripcion"`
	Categoria     string    `json:"categoria"`
	Etiquetas     []string  `json:"etiquetas"`
	Precio        int       `json:"precio"`
	Moneda        string    `json:"moneda"`
	FotoURL       string    `json:"fotoUrl"`
	Activo        bool      `json:"activo"`
	ActualizadoEn time.Time `json:"actualizadoEn"`
}

// PaginaPlatos es una página de /internal/platos.
type PaginaPlatos struct {
	Items      []PlatoCatalogo `json:"items"`
	Page       int             `json:"page"`
	TotalPages int             `json:"totalPages"`
}

// Catalogo llama a la API interna de Catálogo. Es trabajo de fondo: hasta 5 s por intento
// y 3 reintentos con espera creciente (ADR-005), separado del plazo de 3 s del usuario.
type Catalogo struct {
	base      string
	http      *http.Client
	reintento []time.Duration
}

// NuevoCatalogo crea el cliente. base es, por ejemplo, http://catalogo:8082.
func NuevoCatalogo(base string) *Catalogo {
	return &Catalogo{
		base: strings.TrimRight(base, "/"),
		// otelhttp crea un span por llamada y agrega el header traceparent: así la traza
		// sigue en Catálogo. Los parámetros de estas rutas (page, pageSize) no son sensibles.
		http: &http.Client{
			Timeout: 5 * time.Second,
			Transport: otelhttp.NewTransport(http.DefaultTransport,
				otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
					return "catalogo " + r.Method + " " + r.URL.Path
				})),
		},
		reintento: []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second},
	}
}

// Cocinas devuelve todas las cocinas (activas o no).
func (c *Catalogo) Cocinas(ctx context.Context) ([]CocinaCatalogo, error) {
	var r struct {
		Items []CocinaCatalogo `json:"items"`
	}
	err := c.obtener(ctx, "/internal/cocinas", &r)
	return r.Items, err
}

// Platos devuelve una página de todos los platos (activos o no).
func (c *Catalogo) Platos(ctx context.Context, page, pageSize int) (PaginaPlatos, error) {
	var r PaginaPlatos
	err := c.obtener(ctx, fmt.Sprintf("/internal/platos?page=%d&pageSize=%d", page, pageSize), &r)
	return r, err
}

func (c *Catalogo) obtener(ctx context.Context, ruta string, destino any) error {
	var ultimo error
	for intento := 0; ; intento++ {
		ultimo = c.unaVez(ctx, ruta, destino)
		if ultimo == nil || intento >= len(c.reintento) {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.reintento[intento]):
		}
	}
	if ultimo != nil {
		return fmt.Errorf("catálogo %s: %w", ruta, ultimo)
	}
	return nil
}

func (c *Catalogo) unaVez(ctx context.Context, ruta string, destino any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+ruta, nil)
	if err != nil {
		return err
	}
	if id := middleware.IDDesdeContexto(ctx); id != "" {
		req.Header.Set(middleware.HeaderCorrelacion, id) // seguir la reconstrucción en los logs de ambos
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("respondió %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(destino)
}
