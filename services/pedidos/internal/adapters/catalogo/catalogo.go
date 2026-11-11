// Package catalogo es el adaptador de salida hacia Catálogo. Implementa ports.Catalogo.
package catalogo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"ghostkitchen/pedidos/internal/adapters/correlacion"
	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/trazas"
)

// CatalogoHTTP le pregunta a Catálogo por HTTP (dentro de la red de Docker).
type CatalogoHTTP struct {
	BaseURL string // por ejemplo http://catalogo:8082
	Client  *http.Client
}

// Nuevo arma el cliente con el tiempo máximo de cada intento (500 ms, ADR-005).
func Nuevo(baseURL string) CatalogoHTTP {
	return CatalogoHTTP{BaseURL: baseURL, Client: &http.Client{Timeout: 500 * time.Millisecond, Transport: trazas.Transporte(nil)}}
}

// platoJSON es la parte del plato de Catálogo que usamos.
type platoJSON struct {
	ID            string `json:"id"`
	CocinaID      string `json:"cocinaId"`
	RestauranteID string `json:"restauranteId"`
	Nombre        string `json:"nombre"`
	Precio        int64  `json:"precio"`
	Activo        bool   `json:"activo"`
}

// ObtenerPlato busca el plato. Catálogo ya responde 404 si el plato, su restaurante o su
// cocina están desactivados. Como es una lectura, se puede reintentar una vez sin riesgo.
func (c CatalogoHTTP) ObtenerPlato(ctx context.Context, platoID string) (domain.PlatoCatalogo, error) {
	plato, err := c.pedirPlato(ctx, platoID)
	if errors.Is(err, domain.ErrCatalogoCaido) {
		// Un solo reintento: si Catálogo sigue sin responder, avisamos que no está disponible.
		plato, err = c.pedirPlato(ctx, platoID)
	}
	return plato, err
}

// pedirPlato hace un intento.
func (c CatalogoHTTP) pedirPlato(ctx context.Context, platoID string) (domain.PlatoCatalogo, error) {
	// 1. Armamos el pedido, con el correlation id para seguirlo en los logs.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/internal/platos/"+url.PathEscape(platoID), nil)
	if err != nil {
		return domain.PlatoCatalogo{}, err
	}
	req.Header.Set("X-Correlation-Id", correlacion.Desde(ctx))

	// 2. Lo mandamos. Si no contesta a tiempo o se cae la conexión, Catálogo no está disponible.
	resp, err := c.Client.Do(req)
	if err != nil {
		return domain.PlatoCatalogo{}, domain.ErrCatalogoCaido
	}
	defer resp.Body.Close()

	// 3. Leemos la respuesta.
	if resp.StatusCode == http.StatusNotFound {
		return domain.PlatoCatalogo{}, domain.ErrPlatoNoEncontrado
	}
	if resp.StatusCode != http.StatusOK {
		return domain.PlatoCatalogo{}, domain.ErrCatalogoCaido
	}
	var datos platoJSON
	if err := json.NewDecoder(resp.Body).Decode(&datos); err != nil {
		return domain.PlatoCatalogo{}, domain.ErrCatalogoCaido
	}
	return domain.PlatoCatalogo{
		ID: datos.ID, CocinaID: datos.CocinaID, RestauranteID: datos.RestauranteID,
		Nombre: datos.Nombre, Precio: datos.Precio, Activo: datos.Activo,
	}, nil
}
