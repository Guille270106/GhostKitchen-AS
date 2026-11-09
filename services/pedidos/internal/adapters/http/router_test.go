package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ghostkitchen/pedidos/internal/application"
	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/ports"
)

// PedidosMock tiene un solo pedido y puede simular la base caída.
type PedidosMock struct {
	pedido domain.Pedido
	falla  error
}

func (m PedidosMock) Crear(context.Context, domain.Pedido) error { return nil }
func (m PedidosMock) Obtener(_ context.Context, id string) (domain.Pedido, error) {
	if m.falla != nil {
		return domain.Pedido{}, m.falla
	}
	if id != m.pedido.ID {
		return domain.Pedido{}, domain.ErrNoEncontrado
	}
	return m.pedido, nil
}
func (m PedidosMock) GuardarCambio(context.Context, domain.Pedido, domain.CambioEstado) error {
	return nil
}
func (m PedidosMock) Ready(context.Context) error { return m.falla }

// CarritosMock guarda carritos en un mapa.
type CarritosMock struct{ carritos map[string]domain.Carrito }

func (m *CarritosMock) Obtener(_ context.Context, clienteID string) (domain.Carrito, error) {
	c, ok := m.carritos[clienteID]
	if !ok {
		return domain.Carrito{ClienteID: clienteID}, nil
	}
	return c, nil
}
func (m *CarritosMock) Guardar(_ context.Context, c domain.Carrito, _ int64) error {
	m.carritos[c.ClienteID] = c
	return nil
}

// CatalogoMock tiene un solo plato.
type CatalogoMock struct{}

func (CatalogoMock) ObtenerPlato(_ context.Context, id string) (domain.PlatoCatalogo, error) {
	if id != "p17" {
		return domain.PlatoCatalogo{}, domain.ErrPlatoNoEncontrado
	}
	return domain.PlatoCatalogo{ID: "p17", CocinaID: "cerro", RestauranteID: "r5", Nombre: "Burger", Precio: 12000, Activo: true}, nil
}

// InventarioMock reserva siempre, salvo más de 3 unidades.
type InventarioMock struct{}

func (InventarioMock) Reservar(_ context.Context, _, carritoID, _ string, items []ports.ItemReserva) (string, time.Time, error) {
	if items[0].Cantidad > 3 {
		return "", time.Time{}, domain.ErrSinStock
	}
	return "res-" + carritoID, time.Now().Add(10 * time.Minute), nil
}
func (InventarioMock) Ajustar(_ context.Context, _, _ string, items []ports.ItemReserva) error {
	if len(items) > 0 && items[0].Cantidad > 3 {
		return domain.ErrSinStock
	}
	return nil
}
func (InventarioMock) Liberar(context.Context, string, string) error { return nil }

func servidor(t *testing.T, falla error) http.Handler {
	t.Helper()
	cliente := domain.Actor{UsuarioID: "u-1", Rol: domain.RolCliente}
	pedido, err := domain.NuevoPedido("ped-1", cliente, "c@ejemplo.com", "cerro",
		[]domain.Item{{PlatoID: "p17", Nombre: "Burger", RestauranteID: "r5", PrecioUnitario: 100, Cantidad: 1}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	pedidosRepo := PedidosMock{pedido: pedido, falla: falla}
	carritoService := application.CarritoService{
		Repo: &CarritosMock{carritos: map[string]domain.Carrito{}}, Catalogo: CatalogoMock{}, Inventario: InventarioMock{},
	}
	return Rutas(slog.New(slog.NewJSONHandler(io.Discard, nil)),
		SaludController{Repo: pedidosRepo},
		PedidosController{Service: application.PedidosService{Repo: pedidosRepo}},
		CarritoController{Service: carritoService},
	)
}

func pedir(h http.Handler, metodo, ruta, cuerpo string, headers map[string]string) (*httptest.ResponseRecorder, map[string]any) {
	req := httptest.NewRequest(metodo, ruta, strings.NewReader(cuerpo))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	return w, m
}

var clienteHeaders = map[string]string{"X-Usuario-Id": "u-1", "X-Rol": "cliente"}

func TestDetalleDePedidoSegunIdentidad(t *testing.T) {
	h := servidor(t, nil)
	casos := []struct {
		nombre  string
		headers map[string]string
		estado  int
	}{
		{"el cliente dueño", clienteHeaders, 200},
		{"la cocina del pedido", map[string]string{"X-Usuario-Id": "u-c", "X-Rol": "cocina", "X-Cocina-Id": "cerro"}, 200},
		{"otro cliente", map[string]string{"X-Usuario-Id": "u-2", "X-Rol": "cliente"}, 404},
		{"otra cocina", map[string]string{"X-Usuario-Id": "u-c", "X-Rol": "cocina", "X-Cocina-Id": "nueva-cordoba"}, 404},
		{"sin identidad", nil, 401},
		{"sin rol", map[string]string{"X-Usuario-Id": "u-1"}, 401},
	}
	for _, c := range casos {
		w, m := pedir(h, "GET", "/pedidos/ped-1", "", c.headers)
		if w.Code != c.estado {
			t.Errorf("%s: esperaba %d, obtuvo %d %s", c.nombre, c.estado, w.Code, w.Body.String())
		}
		if c.estado != 200 && (m["code"] == nil || m["correlationId"] == nil) {
			t.Errorf("%s: el error no es Problem Details: %s", c.nombre, w.Body.String())
		}
	}
}

func TestReadyYBaseCaida(t *testing.T) {
	if w, _ := pedir(servidor(t, nil), "GET", "/ready", "", nil); w.Code != 200 {
		t.Fatal(w.Code)
	}
	h := servidor(t, errors.New("mysql caído"))
	if w, _ := pedir(h, "GET", "/ready", "", nil); w.Code != 503 {
		t.Fatal("ready con la base caída:", w.Code)
	}
	w, m := pedir(h, "GET", "/pedidos/ped-1", "", clienteHeaders)
	if w.Code != 503 || m["code"] != "SERVICIO_NO_DISPONIBLE" || w.Header().Get("Retry-After") == "" {
		t.Fatal("con la base caída es 503, nunca un 404 inventado:", w.Code, w.Body.String())
	}
}

func TestCorrelationIdSeRespetaOSeGenera(t *testing.T) {
	h := servidor(t, nil)
	if w, _ := pedir(h, "GET", "/health", "", map[string]string{"X-Correlation-Id": "prueba-1"}); w.Header().Get("X-Correlation-Id") != "prueba-1" {
		t.Fatal("no respetó el correlation id")
	}
	if w, _ := pedir(h, "GET", "/health", "", map[string]string{"X-Correlation-Id": "malo con espacios"}); w.Header().Get("X-Correlation-Id") == "malo con espacios" {
		t.Fatal("aceptó un correlation id inválido")
	}
}

func TestCarritoPorHTTP(t *testing.T) {
	h := servidor(t, nil)

	// Carrito vacío.
	w, m := pedir(h, "GET", "/carrito", "", clienteHeaders)
	if w.Code != 200 || len(m["items"].([]any)) != 0 {
		t.Fatal(w.Code, w.Body.String())
	}

	// Agregar un plato.
	w, m = pedir(h, "POST", "/carrito/items", `{"platoId":"p17","cantidad":2}`, clienteHeaders)
	if w.Code != 200 || m["total"] != 24000.0 || m["reservaId"] == nil || m["venceEn"] == nil {
		t.Fatal(w.Code, w.Body.String())
	}

	// Errores con su código.
	casos := []struct {
		nombre, metodo, ruta, cuerpo string
		headers                      map[string]string
		estado                       int
		code                         string
	}{
		{"sin stock", "POST", "/carrito/items", `{"platoId":"p17","cantidad":5}`, clienteHeaders, 409, "SIN_STOCK"},
		{"plato inexistente", "POST", "/carrito/items", `{"platoId":"p999","cantidad":1}`, clienteHeaders, 404, "PLATO_NO_ENCONTRADO"},
		{"cantidad cero", "PATCH", "/carrito/items/p17", `{"cantidad":0}`, clienteHeaders, 400, "CANTIDAD_INVALIDA"},
		{"campo de más", "POST", "/carrito/items", `{"platoId":"p17","cantidad":1,"precio":1}`, clienteHeaders, 400, "DATOS_INVALIDOS"},
		{"quitar uno que no está", "DELETE", "/carrito/items/p21", "", clienteHeaders, 404, "NO_ESTA_EN_EL_CARRITO"},
		{"la cocina no tiene carrito", "GET", "/carrito", "", map[string]string{"X-Usuario-Id": "u-c", "X-Rol": "cocina", "X-Cocina-Id": "cerro"}, 403, "SIN_PERMISO"},
		{"sin identidad", "GET", "/carrito", "", nil, 401, "NO_AUTENTICADO"},
	}
	for _, c := range casos {
		w, m := pedir(h, c.metodo, c.ruta, c.cuerpo, c.headers)
		if w.Code != c.estado || m["code"] != c.code {
			t.Errorf("%s: esperaba %d %s, obtuvo %d %s", c.nombre, c.estado, c.code, w.Code, w.Body.String())
		}
	}

	// Vaciar.
	w, m = pedir(h, "DELETE", "/carrito", "", clienteHeaders)
	if w.Code != 200 || len(m["items"].([]any)) != 0 || m["reservaId"] != nil {
		t.Fatal(w.Code, w.Body.String())
	}
}
