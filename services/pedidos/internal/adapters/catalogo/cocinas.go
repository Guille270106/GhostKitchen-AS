package catalogo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"ghostkitchen/pedidos/internal/adapters/correlacion"
	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/ports"
)

// CocinasDeLocalidad devuelve las cocinas de una localidad, activas o no (implementa
// ports.Cocinas, G21). Usa la API interna de Catálogo, que trae todas las cocinas con su
// localidad: son pocas (una por barrio), así que se filtran acá. Es una lectura: se puede
// reintentar una vez sin riesgo, como ObtenerPlato.
func (c CatalogoHTTP) CocinasDeLocalidad(ctx context.Context, localidad string) ([]ports.CocinaLocalidad, error) {
	cocinas, err := c.pedirCocinas(ctx, localidad)
	if errors.Is(err, domain.ErrCatalogoCaido) {
		cocinas, err = c.pedirCocinas(ctx, localidad)
	}
	return cocinas, err
}

func (c CatalogoHTTP) pedirCocinas(ctx context.Context, localidad string) ([]ports.CocinaLocalidad, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/internal/cocinas", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Correlation-Id", correlacion.Desde(ctx))
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, domain.ErrCatalogoCaido
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, domain.ErrCatalogoCaido
	}
	var datos struct {
		Items []struct {
			ID        string `json:"id"`
			Nombre    string `json:"nombre"`
			Localidad string `json:"localidad"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&datos); err != nil {
		return nil, domain.ErrCatalogoCaido
	}
	res := []ports.CocinaLocalidad{}
	for _, it := range datos.Items {
		if it.Localidad == localidad {
			res = append(res, ports.CocinaLocalidad{ID: it.ID, Nombre: it.Nombre})
		}
	}
	return res, nil
}
