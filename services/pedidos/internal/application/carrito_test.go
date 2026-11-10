package application

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/ports"
)

// CarritosMock guarda los carritos en un mapa, con el mismo control de versión que MySQL.
type CarritosMock struct {
	mu         sync.Mutex
	carritos   map[string]domain.Carrito
	operaciones map[string]bool // cliente + clave ya aplicadas
	// fallarGuardar simula que la base se cae justo al guardar (después de reservar).
	fallarGuardar bool
}

func (r *CarritosMock) Obtener(_ context.Context, clienteID string) (domain.Carrito, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.carritos[clienteID]
	if !ok {
		return domain.Carrito{ClienteID: clienteID}, nil
	}
	c.Items = append([]domain.ItemCarrito{}, c.Items...)
	return c, nil
}

func (r *CarritosMock) Guardar(_ context.Context, c domain.Carrito, versionAnterior int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fallarGuardar {
		return errors.New("se cayó la base")
	}
	if r.carritos[c.ClienteID].Version != versionAnterior {
		return domain.ErrCarritoModificado
	}
	if c.Operacion != "" {
		if r.operaciones == nil {
			r.operaciones = map[string]bool{}
		}
		if r.operaciones[c.ClienteID+"|"+c.Operacion] {
			return domain.ErrOperacionRepetida
		}
		r.operaciones[c.ClienteID+"|"+c.Operacion] = true
	}
	r.carritos[c.ClienteID] = c
	return nil
}

func (r *CarritosMock) OperacionAplicada(_ context.Context, clienteID, clave string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.operaciones[clienteID+"|"+clave], nil
}

// MarcarVencido vacía el carrito que tiene esa reserva, igual que MySQL: si ya no la tiene, no hace nada.
func (r *CarritosMock) MarcarVencido(_ context.Context, carritoID, reservaID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for cliente, c := range r.carritos {
		if c.ID == carritoID && c.ReservaID == reservaID {
			ahora := time.Now()
			c.Reiniciar("car-vencido")
			c.VencioEn = &ahora
			c.Version++
			r.carritos[cliente] = c
			return true, nil
		}
	}
	return false, nil
}

// vaciar deja vacío el carrito del cliente (lo usa el mock de confirmaciones).
func (r *CarritosMock) vaciar(clienteID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.carritos[clienteID]
	c.Reiniciar("car-nuevo")
	r.carritos[clienteID] = c
}

// CatalogoMock tiene dos platos de Cerro y uno de Nueva Córdoba.
type CatalogoMock struct{ caido bool }

func (m CatalogoMock) ObtenerPlato(_ context.Context, id string) (domain.PlatoCatalogo, error) {
	if m.caido {
		return domain.PlatoCatalogo{}, domain.ErrCatalogoCaido
	}
	platos := map[string]domain.PlatoCatalogo{
		"p17": {ID: "p17", CocinaID: "cerro", RestauranteID: "r5", Nombre: "Burger", Precio: 12000, Activo: true},
		"p21": {ID: "p21", CocinaID: "cerro", RestauranteID: "r6", Nombre: "Pizza", Precio: 9500, Activo: true},
		"p5":  {ID: "p5", CocinaID: "nueva-cordoba", RestauranteID: "r2", Nombre: "Sushi", Precio: 15500, Activo: true},
	}
	p, ok := platos[id]
	if !ok {
		return domain.PlatoCatalogo{}, domain.ErrPlatoNoEncontrado
	}
	return p, nil
}

// InventarioMock anota lo que le pidieron y puede simular falta de stock o vencimiento.
// Como el Inventario real, recuerda cada clave con sus datos (aunque la operación haya
// fallado): la misma clave con otros datos es un error.
type InventarioMock struct {
	stock    map[string]int64 // cuántas unidades se pueden reservar de cada plato
	reservas map[string][]ports.ItemReserva
	claves   []string
	usadas   map[string]string // clave -> datos con los que se usó
	vencida  bool
	caido    bool
}

func (m *InventarioMock) alcanza(clave string, items []ports.ItemReserva) error {
	if m.caido {
		return domain.ErrInventarioCaido
	}
	datos := fmt.Sprint(items)
	if anteriores, ok := m.usadas[clave]; ok && anteriores != datos {
		return domain.ErrCarritoModificado
	}
	m.usadas[clave] = datos
	if m.vencida {
		return domain.ErrCarritoVencido
	}
	for _, it := range items {
		if it.Cantidad > m.stock[it.PlatoID] {
			return domain.ErrSinStock
		}
	}
	return nil
}

func (m *InventarioMock) Reservar(_ context.Context, clave, carritoID, _ string, items []ports.ItemReserva) (string, time.Time, error) {
	if err := m.alcanza(clave, items); err != nil {
		return "", time.Time{}, err
	}
	m.claves = append(m.claves, clave)
	m.reservas["res-"+carritoID] = items
	return "res-" + carritoID, time.Now().Add(10 * time.Minute), nil
}

func (m *InventarioMock) Ajustar(_ context.Context, clave, reservaID string, items []ports.ItemReserva) error {
	if _, repetida := m.usadas[clave]; repetida && !m.caido {
		// Como Inventario real: la clave ya se usó, devuelve el resultado guardado y no reaplica.
		return m.alcanza(clave, items)
	}
	if err := m.alcanza(clave, items); err != nil {
		return err
	}
	m.claves = append(m.claves, clave)
	m.reservas[reservaID] = items
	return nil
}

func (m *InventarioMock) Liberar(_ context.Context, clave, reservaID string) error {
	if m.caido {
		return domain.ErrInventarioCaido
	}
	m.claves = append(m.claves, clave)
	delete(m.reservas, reservaID)
	return nil
}

var clienteCarrito = domain.Actor{UsuarioID: "u-cliente", Rol: domain.RolCliente}

func carritoEntorno() (CarritoService, *CarritosMock, *InventarioMock) {
	repo := &CarritosMock{carritos: map[string]domain.Carrito{}}
	inv := &InventarioMock{stock: map[string]int64{"p17": 5, "p21": 5, "p5": 5}, reservas: map[string][]ports.ItemReserva{}, usadas: map[string]string{}}
	return CarritoService{Repo: repo, Catalogo: CatalogoMock{}, Inventario: inv}, repo, inv
}

func TestAgregarPlatoReservaYGuardaElPrecio(t *testing.T) {
	svc, repo, inv := carritoEntorno()
	c, err := svc.AgregarPlato(context.Background(), clienteCarrito, "p17", 2)
	if err != nil {
		t.Fatal(err)
	}
	if c.ReservaID == "" || c.VenceEn == nil || c.Total != 24000 || c.Version != 1 {
		t.Fatalf("%+v", c)
	}
	if r := inv.reservas[c.ReservaID]; len(r) != 1 || r[0].Cantidad != 2 {
		t.Fatal("la reserva no quedó igual al carrito:", r)
	}

	// Segundo plato de otro restaurante de la misma cocina: se ajusta la misma reserva.
	c, err = svc.AgregarPlato(context.Background(), clienteCarrito, "p21", 1)
	if err != nil || len(inv.reservas[c.ReservaID]) != 2 || c.Version != 2 {
		t.Fatal(err, c)
	}
	if repo.carritos["u-cliente"].Total != 24000+9500 {
		t.Fatal("no se guardó el carrito")
	}
	// Cada cambio usa una clave distinta: carrito + número de cambio.
	if len(inv.claves) != 2 || inv.claves[0] == inv.claves[1] {
		t.Fatal("claves de idempotencia:", inv.claves)
	}
}

func TestSinStockNoCambiaElCarrito(t *testing.T) {
	svc, repo, _ := carritoEntorno()
	if _, err := svc.AgregarPlato(context.Background(), clienteCarrito, "p17", 9); !errors.Is(err, domain.ErrSinStock) {
		t.Fatal(err)
	}
	if _, ok := repo.carritos["u-cliente"]; ok {
		t.Fatal("si Inventario dice que no alcanza, el carrito no se guarda")
	}
}

// Lo encontró la prueba de punta a punta: después de un "sin stock", el siguiente cambio
// (con otros platos) no puede chocar con la clave del intento que falló.
func TestDespuesDeSinStockSePuedeSeguirUsandoElCarrito(t *testing.T) {
	svc, _, _ := carritoEntorno()
	if _, err := svc.AgregarPlato(context.Background(), clienteCarrito, "p17", 2); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AgregarPlato(context.Background(), clienteCarrito, "p17", 9); !errors.Is(err, domain.ErrSinStock) {
		t.Fatal(err)
	}
	c, err := svc.AgregarPlato(context.Background(), clienteCarrito, "p21", 1)
	if err != nil || len(c.Items) != 2 {
		t.Fatal("el carrito tiene que seguir funcionando:", err)
	}
	if _, err := svc.CambiarCantidad(context.Background(), clienteCarrito, "p17", 3); err != nil {
		t.Fatal(err)
	}
}

func TestReintentarElMismoCambioUsaLaMismaClave(t *testing.T) {
	items := []ports.ItemReserva{{PlatoID: "p17", Cantidad: 2}}
	if claveDeCambio("car-1", 3, "a", items) != claveDeCambio("car-1", 3, "a", items) {
		t.Fatal("el mismo intento del mismo cambio tiene que dar la misma clave")
	}
	otros := []ports.ItemReserva{{PlatoID: "p17", Cantidad: 3}}
	if claveDeCambio("car-1", 3, "a", items) == claveDeCambio("car-1", 3, "a", otros) {
		t.Fatal("otros platos tienen que dar otra clave")
	}
	if claveDeCambio("car-1", 3, "a", items) == claveDeCambio("car-1", 3, "b", items) {
		t.Fatal("otra solicitud tiene que dar otra clave")
	}
}

func TestOtraCocinaYPlatoInexistente(t *testing.T) {
	svc, _, _ := carritoEntorno()
	_, _ = svc.AgregarPlato(context.Background(), clienteCarrito, "p17", 1)
	if _, err := svc.AgregarPlato(context.Background(), clienteCarrito, "p5", 1); !errors.Is(err, domain.ErrOtraCocina) {
		t.Fatal(err)
	}
	if _, err := svc.AgregarPlato(context.Background(), clienteCarrito, "p999", 1); !errors.Is(err, domain.ErrPlatoNoEncontrado) {
		t.Fatal(err)
	}
}

func TestCambiarCantidadQuitarYVaciar(t *testing.T) {
	svc, _, inv := carritoEntorno()
	c, _ := svc.AgregarPlato(context.Background(), clienteCarrito, "p17", 1)
	_, _ = svc.AgregarPlato(context.Background(), clienteCarrito, "p21", 1)
	reserva := c.ReservaID

	c, err := svc.CambiarCantidad(context.Background(), clienteCarrito, "p17", 3)
	if err != nil || c.Total != 3*12000+9500 || inv.reservas[reserva][0].Cantidad != 3 {
		t.Fatal(err, c)
	}
	c, err = svc.QuitarPlato(context.Background(), clienteCarrito, "p21")
	if err != nil || len(c.Items) != 1 || len(inv.reservas[reserva]) != 1 {
		t.Fatal(err, c)
	}
	// Al quitar el último plato se libera la reserva y el carrito queda vacío con otro id.
	vacio, err := svc.QuitarPlato(context.Background(), clienteCarrito, "p17")
	if err != nil || !vacio.Vacio() || vacio.ReservaID != "" || vacio.ID == c.ID {
		t.Fatal(err, vacio)
	}
	if _, sigue := inv.reservas[reserva]; sigue {
		t.Fatal("la reserva no se liberó")
	}
}

func TestReservaVencidaVaciaElCarrito(t *testing.T) {
	svc, repo, inv := carritoEntorno()
	_, _ = svc.AgregarPlato(context.Background(), clienteCarrito, "p17", 1)
	inv.vencida = true
	if _, err := svc.AgregarPlato(context.Background(), clienteCarrito, "p21", 1); !errors.Is(err, domain.ErrCarritoVencido) {
		t.Fatal(err)
	}
	if c := repo.carritos["u-cliente"]; !c.Vacio() || c.ReservaID != "" {
		t.Fatal("el carrito vencido queda vacío:", c)
	}
}

func TestDependenciasCaidasDan503SinTocarElCarrito(t *testing.T) {
	svc, repo, inv := carritoEntorno()
	svc.Catalogo = CatalogoMock{caido: true}
	if _, err := svc.AgregarPlato(context.Background(), clienteCarrito, "p17", 1); !errors.Is(err, domain.ErrCatalogoCaido) {
		t.Fatal(err)
	}
	svc.Catalogo = CatalogoMock{}
	inv.caido = true
	if _, err := svc.AgregarPlato(context.Background(), clienteCarrito, "p17", 1); !errors.Is(err, domain.ErrInventarioCaido) {
		t.Fatal(err)
	}
	if _, ok := repo.carritos["u-cliente"]; ok {
		t.Fatal("con una dependencia caída el carrito no cambia")
	}
}

func TestSoloElClienteUsaElCarrito(t *testing.T) {
	svc, _, _ := carritoEntorno()
	cocina := domain.Actor{UsuarioID: "u-c", Rol: domain.RolCocina, CocinaID: "cerro"}
	if _, err := svc.AgregarPlato(context.Background(), cocina, "p17", 1); !errors.Is(err, domain.ErrSinPermiso) {
		t.Fatal(err)
	}
	if _, err := svc.Ver(context.Background(), domain.Actor{}); !errors.Is(err, domain.ErrSinIdentidad) {
		t.Fatal(err)
	}
}

func TestVerMuestraElTiempoQueQueda(t *testing.T) {
	svc, repo, _ := carritoEntorno()
	if _, err := svc.AgregarPlato(context.Background(), clienteCarrito, "p17", 1); err != nil {
		t.Fatal(err)
	}
	c, err := svc.Ver(context.Background(), clienteCarrito)
	if err != nil || c.SegundosRestantes < 590 || c.SegundosRestantes > 600 || c.Vencido {
		t.Fatal(err, c.SegundosRestantes, c.Vencido)
	}

	// Pasaron los 10 minutos: Ver lo avisa, pero no cambia el carrito.
	guardado := repo.carritos["u-cliente"]
	vencio := time.Now().Add(-time.Second)
	guardado.VenceEn = &vencio
	repo.carritos["u-cliente"] = guardado
	c, _ = svc.Ver(context.Background(), clienteCarrito)
	if !c.Vencido || c.SegundosRestantes != 0 || len(c.Items) != 1 {
		t.Fatal("tiene que avisar que venció sin borrar nada:", c.Vencido, c.SegundosRestantes, len(c.Items))
	}
}

// Inventario avisa que la reserva venció: el carrito queda vacío y marcado como vencido. El
// mismo aviso otra vez (o uno de una reserva vieja) no cambia nada.
func TestReservaVencidaVaciaElCarritoYLoMarca(t *testing.T) {
	svc, repo, _ := carritoEntorno()
	c, err := svc.AgregarPlato(context.Background(), clienteConfirma, "p17", 2)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Llega el aviso de esa reserva.
	if err := svc.ReservaVencida(context.Background(), c.ReservaID, c.ID); err != nil {
		t.Fatal(err)
	}
	visto, _ := svc.Ver(context.Background(), clienteConfirma)
	if len(visto.Items) != 0 || visto.ReservaID != "" || visto.VencioEn == nil || visto.ID == c.ID {
		t.Fatalf("tiene que quedar vacío, con otro id y marcado: %+v", visto)
	}

	// 2. El mismo aviso otra vez no cambia nada.
	version := repo.carritos[clienteConfirma.UsuarioID].Version
	if err := svc.ReservaVencida(context.Background(), c.ReservaID, c.ID); err != nil || repo.carritos[clienteConfirma.UsuarioID].Version != version {
		t.Fatal("un aviso repetido no tiene que tocar el carrito:", err)
	}

	// 3. Cuando el cliente vuelve a agregar, la marca de vencido desaparece.
	nuevo, err := svc.AgregarPlato(context.Background(), clienteConfirma, "p17", 1)
	if err != nil || nuevo.VencioEn != nil {
		t.Fatal("al cambiar el carrito ya no se avisa que el anterior venció:", err, nuevo.VencioEn)
	}
}

func TestReservaVencidaConDatosRotos(t *testing.T) {
	svc, _, _ := carritoEntorno()
	if err := svc.ReservaVencida(context.Background(), "", "car-1"); !errors.Is(err, domain.ErrDatosInvalidos) {
		t.Fatal(err)
	}
}

// Si la base se cae justo después de reservar el primer plato, el carrito no se guarda: la
// reserva no puede quedar apartando ingredientes que ningún carrito tiene. Se libera.
func TestSiFallaGuardarElPrimerPlatoSeLiberaLaReserva(t *testing.T) {
	svc, repo, inv := carritoEntorno()
	repo.fallarGuardar = true
	if _, err := svc.AgregarPlato(context.Background(), clienteCarrito, "p17", 2); err == nil {
		t.Fatal("tenía que fallar")
	}
	if len(inv.reservas) != 0 {
		t.Fatal("quedó una reserva sin carrito:", inv.reservas)
	}
}

// Si falla guardar un cambio de un carrito que ya existía, la reserva vuelve a quedar igual a
// lo guardado (no con el plato que no se pudo agregar).
func TestSiFallaGuardarUnCambioLaReservaVuelveALoGuardado(t *testing.T) {
	svc, repo, inv := carritoEntorno()
	c, err := svc.AgregarPlato(context.Background(), clienteCarrito, "p17", 2)
	if err != nil {
		t.Fatal(err)
	}
	// Guardar falla (pero leer anda): Inventario ya ajustó la reserva con p21.
	repo.fallarGuardar = true

	if _, err := svc.AgregarPlato(context.Background(), clienteCarrito, "p21", 1); err == nil {
		t.Fatal("tenía que fallar")
	}
	// La reserva tiene que coincidir con lo que quedó guardado: solo p17 x2.
	if r := inv.reservas[c.ReservaID]; len(r) != 1 || r[0].PlatoID != "p17" || r[0].Cantidad != 2 {
		t.Fatal("la reserva no volvió a lo guardado:", r)
	}
}

// Reenviar el mismo "agregar" (la misma Idempotency-Key) no suma dos veces; otro clic sí suma.
func TestReenviarElMismoAgregarNoSumaDosVeces(t *testing.T) {
	svc, repo, inv := carritoEntorno()
	if _, err := svc.AgregarPlatoConClave(context.Background(), clienteCarrito, "p17", 1, "clic-1"); err != nil {
		t.Fatal(err)
	}
	c, err := svc.AgregarPlatoConClave(context.Background(), clienteCarrito, "p17", 1, "clic-1")
	if err != nil || len(c.Items) != 1 || c.Items[0].Cantidad != 1 {
		t.Fatal("el reenvío volvió a sumar:", err, c)
	}
	if r := inv.reservas[c.ReservaID]; r[0].Cantidad != 1 {
		t.Fatal("el reenvío volvió a reservar:", r)
	}
	c, err = svc.AgregarPlatoConClave(context.Background(), clienteCarrito, "p17", 1, "clic-2")
	if err != nil || c.Items[0].Cantidad != 2 || repo.carritos["u-cliente"].Items[0].Cantidad != 2 {
		t.Fatal("otro clic tenía que sumar:", err, c)
	}
	if _, err := svc.AgregarPlatoConClave(context.Background(), clienteCarrito, "p17", 1, "con espacio"); !errors.Is(err, domain.ErrClaveInvalida) {
		t.Fatal("una clave inválida tenía que rechazarse:", err)
	}
}

// El caso de la revisión: reservar 1 → agregar otro (Inventario ajusta a 2) → falla guardar →
// la reserva se corrige a 1 → el cliente reintenta. El reintento tiene que volver a ajustar a 2
// (otra clave), no recibir la respuesta vieja de Inventario y dejar carrito 2 / reserva 1.
func TestReintentarDespuesDeCorregirLaReservaVuelveAAplicarla(t *testing.T) {
	svc, repo, inv := carritoEntorno()
	c, err := svc.AgregarPlato(context.Background(), clienteCarrito, "p17", 1)
	if err != nil {
		t.Fatal(err)
	}
	repo.fallarGuardar = true
	if _, err := svc.AgregarPlato(context.Background(), clienteCarrito, "p17", 1); err == nil {
		t.Fatal("tenía que fallar")
	}
	if r := inv.reservas[c.ReservaID]; r[0].Cantidad != 1 {
		t.Fatal("la reserva no volvió a 1:", r)
	}

	repo.fallarGuardar = false
	c, err = svc.AgregarPlato(context.Background(), clienteCarrito, "p17", 1)
	if err != nil {
		t.Fatal(err)
	}
	if c.Items[0].Cantidad != 2 || inv.reservas[c.ReservaID][0].Cantidad != 2 {
		t.Fatalf("carrito %d / reserva %d: tienen que coincidir en 2", c.Items[0].Cantidad, inv.reservas[c.ReservaID][0].Cantidad)
	}
}
