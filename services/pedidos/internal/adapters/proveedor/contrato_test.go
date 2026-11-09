package proveedor

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/ports"
)

// Test de contrato con el grupo proveedor (L16, ADR-009).
//
// Corre contra su mock (Prism), servido desde la copia de su OpenAPI en docs/contracts/proveedor.
// Prism valida cada solicitud contra el contrato: si nuestro adaptador manda algo que no cumple
// (un campo de más, un header que falta, una clave muy corta), responde 422 y el test falla. Y
// nuestro adaptador tiene que entender la respuesta de ejemplo del contrato.
//
// Si el Grupo 4 publica una versión nueva, se reemplaza el archivo y este test avisa si algo
// dejó de ser compatible. Sin TEST_PROVEEDOR_URL se saltea (lo levanta testdata/compose.yml).
func proveedorDePrueba(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_PROVEEDOR_URL")
	if url == "" {
		t.Skip("requiere el mock del proveedor (TEST_PROVEEDOR_URL)")
	}
	return url
}

// Lo que manda nuestro adaptador cumple su contrato, y entendemos lo que responde.
func TestContratoEmitirComprobante(t *testing.T) {
	url := proveedorDePrueba(t)
	casos := map[string]ports.SolicitudComprobante{
		"un plato": {Email: "cliente@ejemplo.com", Lineas: []domain.LineaComprobante{{Descripcion: "Burger x2", Importe: 24000}}},
		"varios platos, uno gratis y nombre largo": {Email: "ana.perez@mail.com", Lineas: []domain.LineaComprobante{
			{Descripcion: strings.Repeat("a", 160) + " x50", Importe: 600000},
			{Descripcion: "Agua x1", Importe: 0},
		}},
	}
	for nombre, s := range casos {
		ctx, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
		// La clave más larga que armamos: "pedido-" + un UUID (43 caracteres; su contrato acepta 8 a 64).
		emitido, err := Nuevo(url, "clave-de-prueba").Emitir(ctx, "pedido-6b0c1d2e-3f40-4a5b-8c6d-7e8f90a1b2c3", s)
		cancelar()
		if err != nil {
			t.Errorf("%s: el proveedor no aceptó lo que mandamos: %v", nombre, err)
			continue
		}
		if emitido.ID == "" || emitido.Numero == "" {
			t.Errorf("%s: no entendimos su respuesta: %+v", nombre, emitido)
		}
	}
}

// El mock de verdad valida: sin API key o con un campo que su contrato no tiene, rechaza. Si
// esto dejara de pasar, el test de arriba no probaría nada.
func TestContratoElMockValidaLasSolicitudes(t *testing.T) {
	url := proveedorDePrueba(t)
	casos := []struct {
		nombre, cuerpo string
		apiKey         bool
		estado         int
	}{
		{"sin API key", `{"email":"c@ejemplo.com","items":[{"descripcion":"Burger x1","precio":12000}]}`, false, 401},
		{"campo que no existe en su contrato", `{"email":"c@ejemplo.com","items":[{"descripcion":"Burger x1","precio":12000,"cantidad":1}]}`, true, 422},
		{"sin items", `{"email":"c@ejemplo.com","items":[]}`, true, 422},
	}
	for _, c := range casos {
		req, _ := http.NewRequest(http.MethodPost, url+"/v1/comprobantes", strings.NewReader(c.cuerpo))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "pedido-prueba-1")
		if c.apiKey {
			req.Header.Set("X-API-Key", "clave-de-prueba")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != c.estado {
			t.Errorf("%s: esperaba %d, obtuvo %d", c.nombre, c.estado, resp.StatusCode)
		}
	}
}
