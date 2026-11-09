package application

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/ports"
)

// CarritosMock guarda los carritos en un mapa, con el mismo control de versión que MySQL.
type CarritosMock struct {
	carritos map[string]domain.Carrito
}

func (r *CarritosMock) Obtener(_ context.Context, clienteID string) (domain.Carrito, error) {
	c, ok := r.carritos[clienteID]
	if !ok {
		return domain.Carrito{ClienteID: clienteID}, nil
	}
	c.Items = append([]domain.ItemCarrito{}, c.Items...)
	return c, nil
}

func (r *CarritosMock) Guardar(_ context.Context, c domain.Carrito, versionAnterior int64) error {
	if r.carritos[c.ClienteID].Version != versionAnterior {
		return domain.ErrCarritoModificado
	}
	r.carritos[c.ClienteID] = c
	return nil
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
	if claveDeCambio("car-1", 3, items) != claveDeCambio("car-1", 3, items) {
		t.Fatal("el mismo cambio tiene que dar la misma clave")
	}
	otros := []ports.ItemReserva{{PlatoID: "p17", Cantidad: 3}}
	if claveDeCambio("car-1", 3, items) == claveDeCambio("car-1", 3, otros) {
		t.Fatal("otros platos tienen que dar otra clave")
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
