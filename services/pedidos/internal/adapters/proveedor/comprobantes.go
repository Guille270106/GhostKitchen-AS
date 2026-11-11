// Package proveedor es el adaptador de salida hacia el grupo proveedor: la API de comprobantes
// del Grupo 4 (docs/contracts/proveedor). Implementa ports.Proveedor y traduce entre nuestras
// palabras y su contrato (capa anticorrupción, ADR-009): si ellos cambian algo, se arregla acá.
package proveedor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"ghostkitchen/pedidos/internal/adapters/correlacion"
	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/ports"
	"ghostkitchen/pedidos/internal/trazas"
	"github.com/sony/gobreaker"
)

// Comprobantes llama a POST /v1/comprobantes del Grupo 4.
type Comprobantes struct {
	BaseURL string // PROVIDER_BASE_URL
	APIKey  string // PROVIDER_API_KEY: va en X-API-Key, nunca en el repo ni en los logs
	Client  *http.Client
	Breaker *gobreaker.CircuitBreaker
}

// Nuevo arma el cliente: 5 s por intento (lo que recomienda el Grupo 4) y un circuit breaker
// propio, con los mismos valores que el de Inventario (ADR-010): si en 10 llamadas falla la
// mitad, deja de llamarlos 10 s.
func Nuevo(baseURL, apiKey string) Comprobantes {
	return Comprobantes{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Client:  &http.Client{Timeout: 5 * time.Second, Transport: trazas.Transporte(nil)},
		Breaker: gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name:        "proveedor-comprobantes",
			MaxRequests: 3,
			Interval:    60 * time.Second,
			Timeout:     10 * time.Second,
			ReadyToTrip: func(c gobreaker.Counts) bool { return c.Requests >= 10 && c.TotalFailures*2 >= c.Requests },
			// Que rechacen una solicitud es una respuesta normal, no una falla de ellos.
			IsSuccessful: func(err error) bool { return !errors.Is(err, domain.ErrProveedorCaido) },
		}),
	}
}

// Lo que pide su contrato (SolicitudComprobante e Item): sin campos de más.
type solicitudJSON struct {
	Email string     `json:"email"`
	Items []itemJSON `json:"items"`
}
type itemJSON struct {
	Descripcion string  `json:"descripcion"`
	Precio      float64 `json:"precio"`
}

// La parte de su Comprobante que usamos.
type comprobanteJSON struct {
	ID     string `json:"id"`
	Numero string `json:"numero"`
}

// Su Problem Details (Problema).
type problemaJSON struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

// Emitir pide la factura. Respuestas de su contrato:
//   - 201 (emitida) o 200 (misma clave y mismo cuerpo: la que ya estaba): sale bien;
//   - 400, 409 o 422: la rechazan; reintentar no lo arregla;
//   - 401 (clave mal configurada), 429, 503, timeout o cualquier otra cosa: no sabemos, se
//     reintenta después con la misma Idempotency-Key.
func (c Comprobantes) Emitir(ctx context.Context, clave string, s ports.SolicitudComprobante) (ports.ComprobanteEmitido, error) {
	var emitido ports.ComprobanteEmitido
	_, err := c.Breaker.Execute(func() (any, error) {
		var err error
		emitido, err = c.intentar(ctx, clave, s)
		return nil, err
	})
	if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
		return ports.ComprobanteEmitido{}, domain.ErrProveedorCaido
	}
	return emitido, err
}

func (c Comprobantes) intentar(ctx context.Context, clave string, s ports.SolicitudComprobante) (ports.ComprobanteEmitido, error) {
	// 1. Traducimos a su contrato: el importe va en pesos, como número.
	cuerpo := solicitudJSON{Email: s.Email, Items: []itemJSON{}}
	for _, l := range s.Lineas {
		cuerpo.Items = append(cuerpo.Items, itemJSON{Descripcion: l.Descripcion, Precio: float64(l.Importe)})
	}
	datos, err := json.Marshal(cuerpo)
	if err != nil {
		return ports.ComprobanteEmitido{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/comprobantes", bytes.NewReader(datos))
	if err != nil {
		return ports.ComprobanteEmitido{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.APIKey)
	req.Header.Set("Idempotency-Key", clave)
	req.Header.Set("X-Correlation-Id", correlacion.Desde(ctx))

	// 2. La mandamos. Sin respuesta: no sabemos si la emitieron.
	resp, err := c.Client.Do(req)
	if err != nil {
		return ports.ComprobanteEmitido{}, domain.ErrProveedorCaido
	}
	defer resp.Body.Close()

	// 3. Traducimos su respuesta a la nuestra.
	switch resp.StatusCode {
	case http.StatusCreated, http.StatusOK:
		var comp comprobanteJSON
		if err := json.NewDecoder(resp.Body).Decode(&comp); err != nil || comp.ID == "" {
			return ports.ComprobanteEmitido{}, domain.ErrProveedorCaido // respuesta que no entendemos
		}
		return ports.ComprobanteEmitido{ID: comp.ID, Numero: comp.Numero}, nil
	case http.StatusBadRequest, http.StatusConflict, http.StatusUnprocessableEntity:
		var p problemaJSON
		_ = json.NewDecoder(resp.Body).Decode(&p)
		return ports.ComprobanteEmitido{}, fmt.Errorf("%w: %d %s %s", domain.ErrProveedorRechazo, resp.StatusCode, p.Title, p.Detail)
	default:
		return ports.ComprobanteEmitido{}, domain.ErrProveedorCaido
	}
}
