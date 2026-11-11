// Package clients habla con otros servicios por HTTP. Catálogo le pregunta a Inventario si un
// plato tiene receta antes de publicarlo: las recetas son de Inventario (ADR-001).
package clients

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ghostkitchen/catalogo/internal/domain"
)

// TimeoutIntento por consulta a Inventario, con un solo reintento si no hubo respuesta o fue
// 5xx: así entra holgada en el plazo de 3 s de la solicitud (ADR-005).
const TimeoutIntento = 800 * time.Millisecond

// Inventario es el cliente HTTP de Inventario.
type Inventario struct {
	base string
	http *http.Client
}

// NuevoInventario crea el cliente. baseURL es, por ejemplo, http://inventario:8084.
func NuevoInventario(baseURL string) *Inventario {
	return &Inventario{base: strings.TrimRight(baseURL, "/"), http: &http.Client{Timeout: TimeoutIntento}}
}

// TieneReceta pregunta si el plato tiene receta en Inventario (GET /cocina/platos/{id}/receta).
// Se llama con la identidad de la cuenta Cocina que hizo el pedido, la misma que puso el
// gateway: Inventario solo muestra recetas de esa cocina. Un 404 es "no tiene receta"; si
// Inventario no contesta, domain.ErrInventarioNoDisponible (no se toma como "no tiene").
func (c *Inventario) TieneReceta(ctx context.Context, usuarioID, cocinaID, platoID, correlationID string) (bool, error) {
	var ultimo error
	for intento := 1; intento <= 2; intento++ {
		tiene, reintentar, err := c.pedir(ctx, usuarioID, cocinaID, platoID, correlationID)
		if err == nil || !reintentar {
			return tiene, err
		}
		ultimo = err
		if intento == 1 {
			select {
			case <-ctx.Done():
				return false, fmt.Errorf("%w: %v", domain.ErrInventarioNoDisponible, ctx.Err())
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	return false, ultimo
}

func (c *Inventario) pedir(ctx context.Context, usuarioID, cocinaID, platoID, correlationID string) (bool, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/cocina/platos/"+url.PathEscape(platoID)+"/receta", nil)
	if err != nil {
		return false, false, fmt.Errorf("%w: armar solicitud", domain.ErrInventarioNoDisponible)
	}
	req.Header.Set("X-Usuario-Id", usuarioID)
	req.Header.Set("X-Rol", "cocina")
	req.Header.Set("X-Cocina-Id", cocinaID)
	if correlationID != "" {
		req.Header.Set("X-Correlation-Id", correlationID)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // sin la URL
		}
		return false, true, fmt.Errorf("%w: %v", domain.ErrInventarioNoDisponible, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	switch {
	case resp.StatusCode == http.StatusOK:
		return true, false, nil
	case resp.StatusCode == http.StatusNotFound:
		return false, false, nil
	case resp.StatusCode >= 500:
		return false, true, fmt.Errorf("%w: respondió %d", domain.ErrInventarioNoDisponible, resp.StatusCode)
	default:
		return false, false, fmt.Errorf("%w: respondió %d", domain.ErrInventarioNoDisponible, resp.StatusCode)
	}
}
