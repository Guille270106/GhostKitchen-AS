package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ghostkitchen/pedidos/internal/application"
	"ghostkitchen/pedidos/internal/domain"
)

type repoFijo struct {
	p     domain.Pedido
	falla error
}

func (r repoFijo) Crear(context.Context, domain.Pedido) error { return nil }
func (r repoFijo) Obtener(_ context.Context, id string) (domain.Pedido, error) {
	if r.falla != nil {
		return domain.Pedido{}, r.falla
	}
	if id != r.p.ID {
		return domain.Pedido{}, domain.ErrNoEncontrado
	}
	return r.p, nil
}
func (r repoFijo) GuardarCambio(context.Context, domain.Pedido, domain.CambioEstado) error {
	return nil
}
func (r repoFijo) Ready(context.Context) error { return r.falla }

func servidor(t *testing.T, falla error) http.Handler {
	t.Helper()
	cliente := domain.Actor{UsuarioID: "u-1", Rol: domain.RolCliente}
	p, err := domain.NuevoPedido("ped-1", cliente, "c@ejemplo.com", "cerro",
		[]domain.Item{{PlatoID: "p17", Nombre: "Burger", RestauranteID: "r5", PrecioUnitario: 100, Cantidad: 1}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	repo := repoFijo{p: p, falla: falla}
	return Nuevo(application.Nuevo(repo), repo.Ready, slog.New(slog.NewJSONHandler(io.Discard, nil)))
}

func pedir(h http.Handler, ruta string, headers map[string]string) (*httptest.ResponseRecorder, map[string]any) {
	req := httptest.NewRequest("GET", ruta, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	return w, m
}

func TestDetalleDePedidoSegunIdentidad(t *testing.T) {
	h := servidor(t, nil)
	casos := []struct {
		nombre  string
		headers map[string]string
		estado  int
	}{
		{"el cliente dueño", map[string]string{"X-Usuario-Id": "u-1", "X-Rol": "cliente"}, 200},
		{"la cocina del pedido", map[string]string{"X-Usuario-Id": "u-c", "X-Rol": "cocina", "X-Cocina-Id": "cerro"}, 200},
		{"otro cliente", map[string]string{"X-Usuario-Id": "u-2", "X-Rol": "cliente"}, 404},
		{"otra cocina", map[string]string{"X-Usuario-Id": "u-c", "X-Rol": "cocina", "X-Cocina-Id": "nueva-cordoba"}, 404},
		{"sin identidad", nil, 401},
		{"sin rol", map[string]string{"X-Usuario-Id": "u-1"}, 401},
	}
	for _, c := range casos {
		w, m := pedir(h, "/pedidos/ped-1", c.headers)
		if w.Code != c.estado {
			t.Errorf("%s: esperaba %d, obtuvo %d %s", c.nombre, c.estado, w.Code, w.Body.String())
		}
		if c.estado != 200 && (m["code"] == nil || m["correlationId"] == nil) {
			t.Errorf("%s: el error no es Problem Details: %s", c.nombre, w.Body.String())
		}
	}
	if _, m := pedir(h, "/pedidos/ped-1", map[string]string{"X-Usuario-Id": "u-1", "X-Rol": "cliente"}); m["estado"] != "RECIBIDO" || m["total"] != 100.0 {
		t.Fatalf("detalle: %v", m)
	}
}

func TestReadyYBaseCaida(t *testing.T) {
	if w, _ := pedir(servidor(t, nil), "/ready", nil); w.Code != 200 {
		t.Fatal(w.Code)
	}
	h := servidor(t, errors.New("mysql caído"))
	if w, _ := pedir(h, "/ready", nil); w.Code != 503 {
		t.Fatal("ready con la base caída:", w.Code)
	}
	w, m := pedir(h, "/pedidos/ped-1", map[string]string{"X-Usuario-Id": "u-1", "X-Rol": "cliente"})
	if w.Code != 503 || m["code"] != "SERVICIO_NO_DISPONIBLE" || w.Header().Get("Retry-After") == "" {
		t.Fatal("con la base caída es 503, nunca un 404 inventado:", w.Code, w.Body.String())
	}
}

func TestCorrelationIdSeRespetaOSeGenera(t *testing.T) {
	h := servidor(t, nil)
	if w, _ := pedir(h, "/health", map[string]string{"X-Correlation-Id": "prueba-1"}); w.Header().Get("X-Correlation-Id") != "prueba-1" {
		t.Fatal("no respetó el correlation id")
	}
	if w, _ := pedir(h, "/health", map[string]string{"X-Correlation-Id": "malo con espacios"}); w.Header().Get("X-Correlation-Id") == "malo con espacios" {
		t.Fatal("aceptó un correlation id inválido")
	}
}
