package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/ports"
)

// ConfirmacionesMock imita a MySQL: la misma clave o el mismo carrito no crean otra
// confirmación, y Completar crea el pedido y vacía el carrito juntos.
type ConfirmacionesMock struct {
	mu               sync.Mutex
	lista            map[string]domain.Confirmacion
	pedidos          *PedidosMock
	carritos         *CarritosMock
	fallaAlCompletar bool
}

// direccionPrueba es la dirección de entrega que mandan los tests al confirmar.
const direccionPrueba = "Obispo Trejo 242"

func (m *ConfirmacionesMock) Iniciar(_ context.Context, c domain.Confirmacion) (domain.Confirmacion, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.lista {
		if (e.ClienteID == c.ClienteID && e.Clave == c.Clave) || e.CarritoID == c.CarritoID {
			return e, true, nil
		}
	}
	m.lista[c.ID] = c
	return c, false, nil
}

func (m *ConfirmacionesMock) Obtener(_ context.Context, id string) (domain.Confirmacion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.lista[id]
	if !ok {
		return domain.Confirmacion{}, domain.ErrNoEncontrado
	}
	return c, nil
}

func (m *ConfirmacionesMock) BuscarPorClave(_ context.Context, clienteID, clave string) (domain.Confirmacion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.lista {
		if c.ClienteID == clienteID && c.Clave == clave {
			return c, nil
		}
	}
	return domain.Confirmacion{}, domain.ErrNoEncontrado
}

func (m *ConfirmacionesMock) MarcarPaso(_ context.Context, id string, paso domain.PasoConfirmacion) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.lista[id]
	c.Paso = paso
	m.lista[id] = c
	return nil
}

func (m *ConfirmacionesMock) Completar(ctx context.Context, c domain.Confirmacion, p domain.Pedido) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fallaAlCompletar {
		return errors.New("mysql caído")
	}
	if m.lista[c.ID].Estado != domain.ConfirmacionEnCurso {
		return nil
	}
	c.Estado = domain.ConfirmacionCompletada
	m.lista[c.ID] = c
	_ = m.pedidos.Crear(ctx, p)
	m.carritos.vaciar(c.ClienteID)
	return nil
}

func (m *ConfirmacionesMock) Fallar(_ context.Context, c domain.Confirmacion, motivo string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c.Estado, c.Motivo = domain.ConfirmacionFallida, motivo
	m.lista[c.ID] = c
	m.carritos.vaciar(c.ClienteID)
	return nil
}

func (m *ConfirmacionesMock) Pendientes(_ context.Context, _ time.Time) ([]domain.Confirmacion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	lista := []domain.Confirmacion{}
	for _, c := range m.lista {
		if c.Estado == domain.ConfirmacionEnCurso {
			lista = append(lista, c)
		}
	}
	return lista, nil
}

// ConsumoMock es el descuento de Inventario: cuenta las llamadas y puede fallar.
type ConsumoMock struct {
	mu       sync.Mutex
	llamadas int
	claves   map[string]bool
	err      error // lo que responde (nil = descontó)
}

func (m *ConsumoMock) Consumir(_ context.Context, clave, _, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.llamadas++
	m.claves[clave] = true
	return m.err
}

var clienteConfirma = domain.Actor{UsuarioID: "u-cliente", Email: "cliente@ejemplo.com", Rol: domain.RolCliente}

// entornoConfirmacion arma todo con un carrito que ya tiene 2 burgers reservadas.
func entornoConfirmacion(t *testing.T) (ConfirmacionService, *ConfirmacionesMock, *ConsumoMock, *CarritosMock, *PedidosMock) {
	t.Helper()
	carritoSvc, carritos, _ := carritoEntorno()
	if _, err := carritoSvc.AgregarPlato(context.Background(), clienteConfirma, "p17", 2, ""); err != nil {
		t.Fatal(err)
	}
	pedidos := &PedidosMock{pedidos: map[string]domain.Pedido{}}
	confirmaciones := &ConfirmacionesMock{lista: map[string]domain.Confirmacion{}, pedidos: pedidos, carritos: carritos}
	consumo := &ConsumoMock{claves: map[string]bool{}}
	svc := ConfirmacionService{Repo: confirmaciones, Carritos: carritos, Pedidos: pedidos, Consumo: consumo}
	return svc, confirmaciones, consumo, carritos, pedidos
}

func TestConfirmarCreaElPedidoYVaciaElCarrito(t *testing.T) {
	svc, _, consumo, carritos, pedidos := entornoConfirmacion(t)
	r, err := svc.Confirmar(context.Background(), clienteConfirma, "clave-1", direccionPrueba)
	if err != nil || r.Pedido == nil {
		t.Fatal(err, r)
	}
	if r.Pedido.Estado != domain.EstadoRecibido || r.Pedido.Total != 24000 || r.Pedido.ClienteEmail != "cliente@ejemplo.com" {
		t.Fatalf("%+v", r.Pedido)
	}
	if len(pedidos.pedidos) != 1 || consumo.llamadas != 1 {
		t.Fatal("un pedido y un descuento:", len(pedidos.pedidos), consumo.llamadas)
	}
	if c := carritos.carritos["u-cliente"]; !c.Vacio() {
		t.Fatal("el carrito queda vacío después de confirmar")
	}
}

func TestDobleClicConLaMismaClaveEsUnSoloPedido(t *testing.T) {
	svc, _, consumo, _, pedidos := entornoConfirmacion(t)
	r1, err := svc.Confirmar(context.Background(), clienteConfirma, "clave-1", direccionPrueba)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := svc.Confirmar(context.Background(), clienteConfirma, "clave-1", direccionPrueba)
	if err != nil || r2.Pedido == nil || r2.Pedido.ID != r1.Pedido.ID {
		t.Fatal("la misma clave devuelve el mismo pedido:", err, r2)
	}
	if len(pedidos.pedidos) != 1 || consumo.llamadas != 1 {
		t.Fatal("no se crea otro pedido ni se descuenta dos veces:", len(pedidos.pedidos), consumo.llamadas)
	}
}

func TestDosClavesALaVezConfirmanUnaSolaVez(t *testing.T) {
	svc, _, consumo, _, pedidos := entornoConfirmacion(t)
	var wg sync.WaitGroup
	for _, clave := range []string{"clave-a", "clave-b"} {
		wg.Add(1)
		go func(clave string) {
			defer wg.Done()
			_, _ = svc.Confirmar(context.Background(), clienteConfirma, clave, direccionPrueba)
		}(clave)
	}
	wg.Wait()
	if len(pedidos.pedidos) != 1 || len(consumo.claves) != 1 {
		t.Fatal("el mismo carrito se confirma una sola vez:", len(pedidos.pedidos), consumo.claves)
	}
}

func TestInventarioCaidoRespondeEstamosConfirmandoYDespuesSeRecupera(t *testing.T) {
	svc, confirmaciones, consumo, carritos, pedidos := entornoConfirmacion(t)
	consumo.err = domain.ErrInventarioCaido

	// 1. Inventario no contesta: no decimos que falló, queda EN_CURSO (la API responde 202).
	r, err := svc.Confirmar(context.Background(), clienteConfirma, "clave-1", direccionPrueba)
	if err != nil || r.Pedido != nil || r.Confirmacion.Estado != domain.ConfirmacionEnCurso {
		t.Fatal(err, r)
	}
	if consumo.llamadas < 2 {
		t.Fatal("tiene que reintentar con backoff:", consumo.llamadas)
	}
	if len(consumo.claves) != 1 {
		t.Fatal("todos los reintentos usan la misma clave:", consumo.claves)
	}
	if len(pedidos.pedidos) != 0 || len(carritos.carritos["u-cliente"].Items) == 0 {
		t.Fatal("todavía no hay pedido y el carrito sigue igual")
	}

	// 2. Inventario vuelve: el proceso de recuperación termina la confirmación.
	consumo.err = nil
	if _, err := svc.RecuperarPendientes(context.Background()); err != nil {
		t.Fatal(err)
	}
	conf, _ := confirmaciones.Obtener(context.Background(), r.Confirmacion.ID)
	if conf.Estado != domain.ConfirmacionCompletada || len(pedidos.pedidos) != 1 {
		t.Fatal("la recuperación tiene que completar la confirmación:", conf.Estado, len(pedidos.pedidos))
	}

	// 3. Si el cliente vuelve a apretar con la misma clave, recibe el pedido.
	r, err = svc.Confirmar(context.Background(), clienteConfirma, "clave-1", direccionPrueba)
	if err != nil || r.Pedido == nil {
		t.Fatal(err, r)
	}
}

func TestReservaVencidaFallaYVaciaElCarrito(t *testing.T) {
	svc, _, consumo, carritos, pedidos := entornoConfirmacion(t)
	consumo.err = domain.ErrCarritoVencido
	if _, err := svc.Confirmar(context.Background(), clienteConfirma, "clave-1", direccionPrueba); !errors.Is(err, domain.ErrCarritoVencido) {
		t.Fatal(err)
	}
	if consumo.llamadas != 1 {
		t.Fatal("una respuesta definitiva no se reintenta:", consumo.llamadas)
	}
	if len(pedidos.pedidos) != 0 || len(carritos.carritos["u-cliente"].Items) != 0 {
		t.Fatal("sin pedido y con el carrito vacío")
	}
	// La misma clave sigue diciendo lo mismo.
	if _, err := svc.Confirmar(context.Background(), clienteConfirma, "clave-1", direccionPrueba); !errors.Is(err, domain.ErrCarritoVencido) {
		t.Fatal(err)
	}
}

func TestSiFallaGuardarElPedidoNoSeDevuelveNadaYSeReintenta(t *testing.T) {
	svc, confirmaciones, consumo, _, pedidos := entornoConfirmacion(t)
	confirmaciones.fallaAlCompletar = true
	r, err := svc.Confirmar(context.Background(), clienteConfirma, "clave-1", direccionPrueba)
	if err != nil || r.Pedido != nil {
		t.Fatal(err, r)
	}
	conf, _ := confirmaciones.Obtener(context.Background(), r.Confirmacion.ID)
	if conf.Paso != domain.PasoCrearPedido {
		t.Fatal("el descuento ya está hecho: queda anotado el paso siguiente:", conf.Paso)
	}

	confirmaciones.fallaAlCompletar = false
	_, _ = svc.RecuperarPendientes(context.Background())
	if len(pedidos.pedidos) != 1 || consumo.llamadas != 1 {
		t.Fatal("se crea el pedido sin volver a descontar:", len(pedidos.pedidos), consumo.llamadas)
	}
}

func TestNoSeConfirmaSinClaveOConElCarritoVacio(t *testing.T) {
	svc, _, _, _, _ := entornoConfirmacion(t)
	if _, err := svc.Confirmar(context.Background(), clienteConfirma, "", direccionPrueba); !errors.Is(err, domain.ErrClaveInvalida) {
		t.Fatal(err)
	}
	otro := domain.Actor{UsuarioID: "u-otro", Email: "otro@ejemplo.com", Rol: domain.RolCliente}
	if _, err := svc.Confirmar(context.Background(), otro, "clave-1", direccionPrueba); !errors.Is(err, domain.ErrCarritoVacio) {
		t.Fatal(err)
	}
}

// EnviosMock es la tarifa de envío de la cocina en Catálogo; se puede cambiar o "caer".
type EnviosMock struct {
	tarifa int64
	caido  bool
}

func (e *EnviosMock) DatosDeCocina(context.Context, string) (ports.DatosCocina, error) {
	if e.caido {
		return ports.DatosCocina{}, domain.ErrCatalogoCaido
	}
	return ports.DatosCocina{CostoEnvio: e.tarifa, Restaurantes: map[string]string{"r5": "Johnny B. Good", "r6": "Il Gatto"}}, nil
}

// El pedido guarda el envío que se cobró al confirmar. Si después la cocina cambia su tarifa,
// el importe del pedido no cambia.
func TestElPedidoGuardaElEnvioDelMomentoDeConfirmar(t *testing.T) {
	svc, _, _, _, pedidos := entornoConfirmacion(t)
	envios := &EnviosMock{tarifa: 1300}
	svc.Envios = envios
	r, err := svc.Confirmar(context.Background(), clienteConfirma, "clave-1", direccionPrueba)
	if err != nil || r.Pedido == nil {
		t.Fatal(err, r)
	}
	if r.Pedido.CostoEnvio == nil || *r.Pedido.CostoEnvio != 1300 || *r.Pedido.TotalConEnvio != 24000+1300 || r.Pedido.Total != 24000 {
		t.Fatalf("envío o total mal guardados: %+v", r.Pedido)
	}
	for _, it := range r.Pedido.Items {
		if it.RestauranteNombre == "" {
			t.Fatalf("el pedido tiene que guardar el nombre del restaurante: %+v", it)
		}
	}

	envios.tarifa = 2300 // el admin cambia la tarifa
	guardado := pedidos.pedidos[r.Pedido.ID]
	if *guardado.CostoEnvio != 1300 || *guardado.TotalConEnvio != 25300 {
		t.Fatal("el pedido ya confirmado cambió de importe:", *guardado.CostoEnvio, *guardado.TotalConEnvio)
	}
}

// Si Catálogo no contesta al confirmar, todavía no se hizo nada: no se descuenta stock ni se
// crea el pedido, y el carrito sigue igual.
func TestSinTarifaDeEnvioNoSeConfirmaNada(t *testing.T) {
	svc, _, consumo, carritos, pedidos := entornoConfirmacion(t)
	svc.Envios = &EnviosMock{caido: true}
	if _, err := svc.Confirmar(context.Background(), clienteConfirma, "clave-1", direccionPrueba); !errors.Is(err, domain.ErrCatalogoCaido) {
		t.Fatal("tenía que avisar que Catálogo no está:", err)
	}
	if len(pedidos.pedidos) != 0 || consumo.llamadas != 0 || len(carritos.carritos["u-cliente"].Items) == 0 {
		t.Fatal("no se tenía que tocar nada")
	}
}
