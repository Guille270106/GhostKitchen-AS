// Package clients habla con otros servicios por HTTP. Usuarios solo le pregunta a Catálogo
// por una cocina, cuando un admin crea una cuenta Cocina.
package clients

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ghostkitchen/usuarios/internal/trazas"
)

// Límites de la consulta a Catálogo (README de Usuarios y ADR-005): 800 ms por intento y un
// solo reintento, así entra holgada en el plazo de 3 s de la solicitud.
const (
	TimeoutIntento  = 800 * time.Millisecond
	EsperaReintento = 100 * time.Millisecond
)

var (
	// ErrCocinaNoExiste: Catálogo respondió 404. Puede no existir o estar inactiva: Catálogo
	// solo muestra cocinas activas, y para crear una cuenta da lo mismo.
	ErrCocinaNoExiste = errors.New("la cocina no existe o está inactiva")
	// ErrCatalogoNoDisponible: no se pudo saber. No se toma como "no existe" (README).
	ErrCatalogoNoDisponible = errors.New("catálogo no disponible")
)

// Cocina es lo que Usuarios necesita saber de una cocina.
type Cocina struct {
	ID        string `json:"id"`
	Localidad string `json:"localidad"`
	Activa    bool   `json:"activa"`
}

// Catalogo es el cliente HTTP de Catálogo.
type Catalogo struct {
	base string
	http *http.Client
}

// NuevoCatalogo crea el cliente. baseURL es, por ejemplo, http://catalogo:8082.
func NuevoCatalogo(baseURL string) *Catalogo {
	return &Catalogo{base: strings.TrimRight(baseURL, "/"), http: &http.Client{Timeout: TimeoutIntento, Transport: trazas.Transporte(nil)}}
}

// Cocina pide una cocina activa a Catálogo (GET /cocinas/:id). Reintenta una vez si no hubo
// respuesta o Catálogo respondió 5xx; un 404 es una respuesta y no se reintenta.
func (c *Catalogo) Cocina(ctx context.Context, id, correlationID string) (Cocina, error) {
	var ultimo error
	for intento := 1; intento <= 2; intento++ {
		cocina, reintentar, err := c.pedir(ctx, id, correlationID)
		if err == nil || !reintentar {
			return cocina, err
		}
		ultimo = err
		if intento == 1 {
			select {
			case <-ctx.Done():
				return Cocina{}, fmt.Errorf("%w: %v", ErrCatalogoNoDisponible, ctx.Err())
			case <-time.After(EsperaReintento):
			}
		}
	}
	return Cocina{}, ultimo
}

func (c *Catalogo) pedir(ctx context.Context, id, correlationID string) (Cocina, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/internal/cocinas/"+url.PathEscape(id), nil)
	if err != nil {
		return Cocina{}, false, fmt.Errorf("%w: armar solicitud", ErrCatalogoNoDisponible)
	}
	if correlationID != "" {
		req.Header.Set("X-Correlation-Id", correlationID)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// Sin la URL: el error de Go la incluye completa.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return Cocina{}, true, fmt.Errorf("%w: %v", ErrCatalogoNoDisponible, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return Cocina{}, false, ErrCocinaNoExiste
	case resp.StatusCode >= 500:
		return Cocina{}, true, fmt.Errorf("%w: respondió %d", ErrCatalogoNoDisponible, resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return Cocina{}, false, fmt.Errorf("%w: respondió %d", ErrCatalogoNoDisponible, resp.StatusCode)
	}
	var cocina Cocina
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&cocina); err != nil || cocina.ID == "" {
		return Cocina{}, false, fmt.Errorf("%w: respuesta inválida", ErrCatalogoNoDisponible)
	}
	return cocina, false, nil
}
