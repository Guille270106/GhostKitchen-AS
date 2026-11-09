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

// Listar devuelve el único pedido si coincide con el filtro (y solo en la primera página).
func (m PedidosMock) Listar(_ context.Context, f ports.FiltroPedidos) ([]domain.Pedido, error) {
	if m.falla != nil {
		return nil, m.falla
	}
	if f.ClienteID != "" && f.ClienteID != m.pedido.ClienteID || f.CocinaID != "" && f.CocinaID != m.pedido.CocinaID ||
		f.Estado != "" && f.Estado != m.pedido.Estado || f.Pagina > 1 {
		return []domain.Pedido{}, nil
	}
	return []domain.Pedido{m.pedido}, nil
}

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
	return servidorConConsumo(t, falla, ConsumoMock{})
}

func servidorConConsumo(t *testing.T, falla error, consumo ConsumoMock) http.Handler {
	t.Helper()
	cliente := domain.Actor{UsuarioID: "u-1", Rol: domain.RolCliente}
	pedido, err := domain.NuevoPedido("ped-1", cliente, "c@ejemplo.com", "cerro",
		[]domain.Item{{PlatoID: "p17", Nombre: "Burger", RestauranteID: "r5", PrecioUnitario: 100, Cantidad: 1}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	pedidosRepo := PedidosMock{pedido: pedido, falla: falla}
	carritos := &CarritosMock{carritos: map[string]domain.Carrito{}}
	carritoService := application.CarritoService{Repo: carritos, Catalogo: CatalogoMock{}, Inventario: InventarioMock{}}
	confirmacionService := application.ConfirmacionService{
		Repo: &ConfirmacionesMock{lista: map[string]domain.Confirmacion{}}, Carritos: carritos, Pedidos: pedidosRepo, Consumo: consumo,
	}
	return Rutas(slog.New(slog.NewJSONHandler(io.Discard, nil)),
		SaludController{Repo: pedidosRepo},
		PedidosController{Service: application.PedidosService{Repo: pedidosRepo}},
		CarritoController{Service: carritoService},
		ConfirmacionController{Service: confirmacionService},
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
	if w.Code != 200 || m["total"] != 24000.0 || m["reservaId"] == nil || m["venceEn"] == nil ||
		m["segundosRestantes"].(float64) < 590 || m["vencido"] != false {
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

func TestMisPedidosPorHTTP(t *testing.T) {
	h := servidor(t, nil)
	cocina := map[string]string{"X-Usuario-Id": "u-c", "X-Rol": "cocina", "X-Cocina-Id": "cerro"}

	// 1. El cliente ve sus pedidos; la cocina los de su cocina.
	for _, headers := range []map[string]string{clienteHeaders, cocina} {
		w, m := pedir(h, "GET", "/pedidos", "", headers)
		if w.Code != 200 || len(m["pedidos"].([]any)) != 1 || m["pagina"] != 1.0 || m["porPagina"] != 20.0 {
			t.Fatal(w.Code, w.Body.String())
		}
	}

	// 2. Los filtros de la URL llegan al servicio.
	w, m := pedir(h, "GET", "/pedidos?estado=ENTREGADO", "", clienteHeaders)
	if w.Code != 200 || len(m["pedidos"].([]any)) != 0 {
		t.Fatal("filtro por estado:", w.Code, w.Body.String())
	}
	w, m = pedir(h, "GET", "/pedidos?pagina=2", "", clienteHeaders)
	if w.Code != 200 || len(m["pedidos"].([]any)) != 0 || m["pagina"] != 2.0 {
		t.Fatal("segunda página:", w.Code, w.Body.String())
	}

	// 3. Errores.
	casos := []struct {
		nombre, ruta string
		headers      map[string]string
		estado       int
		code         string
	}{
		{"estado inventado", "/pedidos?estado=PERDIDO", clienteHeaders, 400, "DATOS_INVALIDOS"},
		{"página que no es número", "/pedidos?pagina=dos", clienteHeaders, 400, "DATOS_INVALIDOS"},
		{"página cero", "/pedidos?pagina=0", clienteHeaders, 400, "DATOS_INVALIDOS"},
		{"el admin no tiene pedidos", "/pedidos", map[string]string{"X-Usuario-Id": "u-a", "X-Rol": "admin"}, 403, "SIN_PERMISO"},
		{"sin identidad", "/pedidos", nil, 401, "NO_AUTENTICADO"},
	}
	for _, c := range casos {
		if w, m := pedir(h, "GET", c.ruta, "", c.headers); w.Code != c.estado || m["code"] != c.code {
			t.Errorf("%s: esperaba %d %s, obtuvo %d %s", c.nombre, c.estado, c.code, w.Code, w.Body.String())
		}
	}
}

func TestConfirmarRecepcionPorHTTP(t *testing.T) {
	h := servidor(t, nil)
	// El pedido del servidor de prueba está RECIBIDO: todavía no se puede confirmar la entrega.
	if w, m := pedir(h, "POST", "/pedidos/ped-1/entregado", "", clienteHeaders); w.Code != 409 || m["code"] != "TRANSICION_INVALIDA" {
		t.Fatal(w.Code, w.Body.String())
	}
	// Otro cliente no puede, y la cocina tampoco (no es su acción).
	if w, _ := pedir(h, "POST", "/pedidos/ped-1/entregado", "", map[string]string{"X-Usuario-Id": "u-2", "X-Rol": "cliente"}); w.Code != 404 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w, _ := pedir(h, "POST", "/pedidos/ped-1/entregado", "", map[string]string{"X-Usuario-Id": "u-c", "X-Rol": "cocina", "X-Cocina-Id": "cerro"}); w.Code != 403 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestPantallaDeLaCocinaPorHTTP(t *testing.T) {
	h := servidor(t, nil)
	cocina := map[string]string{"X-Usuario-Id": "u-c", "X-Rol": "cocina", "X-Cocina-Id": "cerro"}
	otraCocina := map[string]string{"X-Usuario-Id": "u-o", "X-Rol": "cocina", "X-Cocina-Id": "nueva-cordoba"}

	// 1. La lista: la cocina ve los suyos, filtrando por estado; un cliente no entra.
	if w, m := pedir(h, "GET", "/cocina/pedidos?estado=RECIBIDO", "", cocina); w.Code != 200 || len(m["pedidos"].([]any)) != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w, m := pedir(h, "GET", "/cocina/pedidos", "", clienteHeaders); w.Code != 403 || m["code"] != "SIN_PERMISO" {
		t.Fatal(w.Code, w.Body.String())
	}

	// 2. Las acciones (el pedido de prueba está RECIBIDO en cada llamada).
	casos := []struct {
		nombre, accion string
		headers        map[string]string
		estado         int
		resultado      string
	}{
		{"aceptar", "aceptar", cocina, 200, "ACEPTADO"},
		{"rechazar", "rechazar", cocina, 200, "CANCELADO"},
		{"avanzar sin aceptar", "avanzar", cocina, 409, "TRANSICION_INVALIDA"},
		{"otra cocina", "aceptar", otraCocina, 404, "PEDIDO_NO_ENCONTRADO"},
		{"un cliente", "aceptar", clienteHeaders, 403, "SIN_PERMISO"},
	}
	for _, c := range casos {
		w, m := pedir(h, "POST", "/cocina/pedidos/ped-1/"+c.accion, "", c.headers)
		obtenido := m["estado"]
		if c.estado != 200 {
			obtenido = m["code"]
		}
		if w.Code != c.estado || obtenido != c.resultado {
			t.Errorf("%s: esperaba %d %s, obtuvo %d %s", c.nombre, c.estado, c.resultado, w.Code, w.Body.String())
		}
	}
}

// ConfirmacionesMock guarda las confirmaciones en un mapa (versión mínima para las rutas).
type ConfirmacionesMock struct {
	lista map[string]domain.Confirmacion
}

func (m *ConfirmacionesMock) Iniciar(_ context.Context, c domain.Confirmacion) (domain.Confirmacion, bool, error) {
	m.lista[c.ID] = c
	return c, false, nil
}
func (m *ConfirmacionesMock) Obtener(_ context.Context, id string) (domain.Confirmacion, error) {
	c, ok := m.lista[id]
	if !ok {
		return domain.Confirmacion{}, domain.ErrNoEncontrado
	}
	return c, nil
}
func (m *ConfirmacionesMock) BuscarPorClave(_ context.Context, clienteID, clave string) (domain.Confirmacion, error) {
	for _, c := range m.lista {
		if c.ClienteID == clienteID && c.Clave == clave {
			return c, nil
		}
	}
	return domain.Confirmacion{}, domain.ErrNoEncontrado
}
func (m *ConfirmacionesMock) MarcarPaso(_ context.Context, id string, paso domain.PasoConfirmacion) error {
	c := m.lista[id]
	c.Paso = paso
	m.lista[id] = c
	return nil
}
func (m *ConfirmacionesMock) Completar(_ context.Context, c domain.Confirmacion, _ domain.Pedido) error {
	c.Estado = domain.ConfirmacionCompletada
	m.lista[c.ID] = c
	return nil
}
func (m *ConfirmacionesMock) Fallar(_ context.Context, c domain.Confirmacion, motivo string) error {
	c.Estado, c.Motivo = domain.ConfirmacionFallida, motivo
	m.lista[c.ID] = c
	return nil
}
func (m *ConfirmacionesMock) Pendientes(context.Context, time.Time) ([]domain.Confirmacion, error) {
	return nil, nil
}

// ConsumoMock responde siempre lo mismo.
type ConsumoMock struct{ err error }

func (m ConsumoMock) Consumir(context.Context, string, string, string) error { return m.err }

func TestConfirmarPorHTTP(t *testing.T) {
	cliente := map[string]string{"X-Usuario-Id": "u-1", "X-Usuario-Email": "c@ejemplo.com", "X-Rol": "cliente"}
	conClave := func(clave string) map[string]string {
		h := map[string]string{"Idempotency-Key": clave}
		for k, v := range cliente {
			h[k] = v
		}
		return h
	}

	// Sin clave: 400. Con el carrito vacío: 409.
	h := servidor(t, nil)
	if w, m := pedir(h, "POST", "/pedidos", "", cliente); w.Code != 400 || m["code"] != "IDEMPOTENCY_KEY_INVALIDA" {
		t.Fatal(w.Code, w.Body.String())
	}
	if w, m := pedir(h, "POST", "/pedidos", "", conClave("k-1")); w.Code != 409 || m["code"] != "CARRITO_VACIO" {
		t.Fatal(w.Code, w.Body.String())
	}

	// Con un plato en el carrito: 201 con el pedido en RECIBIDO y su Location.
	pedir(h, "POST", "/carrito/items", `{"platoId":"p17","cantidad":2}`, cliente)
	w, m := pedir(h, "POST", "/pedidos", "", conClave("k-2"))
	if w.Code != 201 || m["estado"] != "RECIBIDO" || m["total"] != 24000.0 || m["clienteEmail"] != "c@ejemplo.com" ||
		!strings.HasPrefix(w.Header().Get("Location"), "/pedidos/") {
		t.Fatal(w.Code, w.Body.String())
	}

	// Inventario no contesta: 202 "Estamos confirmando tu pedido", y se puede consultar.
	h = servidorConConsumo(t, nil, ConsumoMock{err: domain.ErrInventarioCaido})
	pedir(h, "POST", "/carrito/items", `{"platoId":"p17","cantidad":1}`, cliente)
	w, m = pedir(h, "POST", "/pedidos", "", conClave("k-3"))
	if w.Code != 202 || m["estado"] != "CONFIRMANDO" || m["mensaje"] != "Estamos confirmando tu pedido" {
		t.Fatal(w.Code, w.Body.String())
	}
	w, m = pedir(h, "GET", w.Header().Get("Location"), "", cliente)
	if w.Code != 200 || m["estado"] != "EN_CURSO" || m["pedidoId"] != nil {
		t.Fatal(w.Code, w.Body.String())
	}
}
