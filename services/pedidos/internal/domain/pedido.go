// Package domain tiene el agregado Pedido y sus reglas. Es el centro del hexágono: no conoce
// HTTP, MySQL ni otros servicios.
//
// El pedido controla sus propios cambios de estado (DDD): nadie le cambia el estado desde
// afuera, se le pide una acción (Aceptar, Avanzar, Cancelar…) y él decide si es válida para
// quien la pide y para el estado en que está. Cada cambio sube la versión, que la base usa para
// que, si dos cambios llegan a la vez, solo se guarde el primero (ADR-004).
package domain

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// Estado del pedido. Es uno solo para todo el pedido: los restaurantes no tienen estados propios.
type Estado string

const (
	EstadoRecibido      Estado = "RECIBIDO"
	EstadoAceptado      Estado = "ACEPTADO"
	EstadoEnPreparacion Estado = "EN_PREPARACION"
	EstadoListo         Estado = "LISTO"
	EstadoEnCamino      Estado = "EN_CAMINO"
	EstadoEntregado     Estado = "ENTREGADO"
	EstadoCancelado     Estado = "CANCELADO"
)

// siguienteDeCocina es el recorrido que hace la cocina con "avanzar".
// Devuelve false si desde ese estado la cocina no puede avanzar.
func siguienteDeCocina(actual Estado) (Estado, bool) {
	switch actual {
	case EstadoAceptado:
		return EstadoEnPreparacion, true
	case EstadoEnPreparacion:
		return EstadoListo, true
	case EstadoListo:
		return EstadoEnCamino, true
	default:
		return "", false
	}
}

// Final indica si el pedido ya no puede cambiar.
func (e Estado) Final() bool { return e == EstadoEntregado || e == EstadoCancelado }

// Roles que manda el gateway en X-Rol.
const (
	RolCliente = "cliente"
	RolCocina  = "cocina"
	RolAdmin   = "admin"
)

var (
	ErrDatosInvalidos     = errors.New("datos del pedido inválidos")
	ErrSinIdentidad       = errors.New("falta identidad verificada")
	ErrSinPermiso         = errors.New("el rol no puede hacer esta acción")
	ErrNoEncontrado       = errors.New("pedido no encontrado")
	ErrTransicionInvalida = errors.New("el pedido no puede pasar a ese estado desde el estado actual")
	// ErrConflictoVersion: otro cambio se guardó primero. No se reintenta solo, porque ese
	// cambio puede hacer que este ya no tenga sentido (por ejemplo, cancelar uno ya aceptado).
	ErrConflictoVersion = errors.New("el pedido cambió mientras se procesaba la acción")
)

// Actor es quien pide la acción. Sale solo de los headers del gateway (X-Usuario-Id, X-Rol,
// X-Cocina-Id), nunca del cuerpo de la solicitud.
type Actor struct {
	UsuarioID string
	Email     string // X-Usuario-Email: se copia en el pedido para el mail de Notificaciones
	Rol       string
	CocinaID  string
}

// Validar exige la identidad mínima: sin usuario o sin rol, el pedido no viene del gateway.
func (a Actor) Validar() error {
	if a.UsuarioID == "" || a.Rol == "" {
		return ErrSinIdentidad
	}
	return nil
}

// Item es un plato del pedido. Nombre, restaurante y precio son copias del momento en que el
// cliente lo agregó al carrito: si después cambian en Catálogo, el pedido no cambia.
type Item struct {
	PlatoID           string `json:"platoId"`
	Nombre            string `json:"nombre"`
	RestauranteID     string `json:"restauranteId"`
	RestauranteNombre string `json:"restauranteNombre"`
	PrecioUnitario    int64  `json:"precioUnitario"`
	Cantidad          int64  `json:"cantidad"`
}

// Subtotal del item, en pesos.
func (i Item) Subtotal() int64 { return i.PrecioUnitario * i.Cantidad }

// CambioEstado es una entrada del historial: qué pasó, quién lo hizo y cuándo.
type CambioEstado struct {
	EstadoAnterior Estado    `json:"estadoAnterior,omitempty"`
	EstadoNuevo    Estado    `json:"estadoNuevo"`
	ActorID        string    `json:"actorId"`
	ActorRol       string    `json:"actorRol"`
	Version        int64     `json:"version"`
	OcurridoEn     time.Time `json:"ocurridoEn"`
}

// Pedido es el agregado. Sus campos se exportan para leerlos y guardarlos, pero el estado
// y la versión solo se cambian con sus métodos.
type Pedido struct {
	ID            string         `json:"id"`
	ClienteID     string         `json:"clienteId"`
	ClienteEmail  string         `json:"clienteEmail"`
	CocinaID      string         `json:"cocinaId"`
	Estado        Estado         `json:"estado"`
	Items         []Item         `json:"items"`
	Total         int64          `json:"total"`
	Moneda        string         `json:"moneda"`
	Version       int64          `json:"version"`
	CreadoEn      time.Time      `json:"creadoEn"`
	ActualizadoEn time.Time      `json:"actualizadoEn"`
	Historial     []CambioEstado `json:"historial,omitempty"`
}

// IdentificadorValido acepta ids seguros de hasta 64 caracteres (los de Catálogo, Usuarios y UUID).
func IdentificadorValido(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

// NuevoPedido crea un pedido en RECIBIDO, con su primera entrada en el historial. Lo crea
// el cliente al confirmar; los datos de los platos ya vienen validados por Catálogo.
func NuevoPedido(id string, cliente Actor, clienteEmail, cocinaID string, items []Item, ahora time.Time) (Pedido, error) {
	if err := cliente.Validar(); err != nil {
		return Pedido{}, err
	}
	if cliente.Rol != RolCliente {
		return Pedido{}, ErrSinPermiso
	}
	if !IdentificadorValido(id) || !IdentificadorValido(cocinaID) || !emailValido(clienteEmail) || len(items) == 0 {
		return Pedido{}, ErrDatosInvalidos
	}
	vistos := map[string]bool{}
	var total int64
	copia := make([]Item, 0, len(items))
	for _, it := range items {
		if !IdentificadorValido(it.PlatoID) || !IdentificadorValido(it.RestauranteID) || vistos[it.PlatoID] ||
			strings.TrimSpace(it.Nombre) == "" || it.Cantidad <= 0 || it.PrecioUnitario < 0 {
			return Pedido{}, ErrDatosInvalidos
		}
		vistos[it.PlatoID] = true
		total += it.Subtotal()
		copia = append(copia, it)
	}
	ahora = ahora.UTC()
	p := Pedido{
		ID: id, ClienteID: cliente.UsuarioID, ClienteEmail: clienteEmail, CocinaID: cocinaID,
		Estado: EstadoRecibido, Items: copia, Total: total, Moneda: "ARS", Version: 1,
		CreadoEn: ahora, ActualizadoEn: ahora,
	}
	p.Historial = []CambioEstado{{EstadoNuevo: EstadoRecibido, ActorID: cliente.UsuarioID, ActorRol: RolCliente, Version: 1, OcurridoEn: ahora}}
	return p, nil
}

// PuedeVer dice si el actor puede ver este pedido: el cliente los suyos y la cocina los de su cocina.
func (p Pedido) PuedeVer(a Actor) bool {
	switch a.Rol {
	case RolCliente:
		return a.UsuarioID == p.ClienteID
	case RolCocina:
		return a.CocinaID != "" && a.CocinaID == p.CocinaID
	default:
		return false
	}
}

// Aceptar: la cocina toma el pedido. Solo desde RECIBIDO.
func (p *Pedido) Aceptar(a Actor, ahora time.Time) (CambioEstado, error) {
	if err := p.autorizarCocina(a); err != nil {
		return CambioEstado{}, err
	}
	return p.cambiar(EstadoRecibido, EstadoAceptado, a, ahora)
}

// Rechazar: la cocina no puede hacerlo. Solo desde RECIBIDO y termina en CANCELADO
// (no existe un estado "rechazado").
func (p *Pedido) Rechazar(a Actor, ahora time.Time) (CambioEstado, error) {
	if err := p.autorizarCocina(a); err != nil {
		return CambioEstado{}, err
	}
	return p.cambiar(EstadoRecibido, EstadoCancelado, a, ahora)
}

// Avanzar: la cocina lo pasa al siguiente paso, ACEPTADO → EN_PREPARACION → LISTO → EN_CAMINO.
func (p *Pedido) Avanzar(a Actor, ahora time.Time) (CambioEstado, error) {
	if err := p.autorizarCocina(a); err != nil {
		return CambioEstado{}, err
	}
	siguiente, ok := siguienteDeCocina(p.Estado)
	if !ok {
		return CambioEstado{}, ErrTransicionInvalida
	}
	return p.cambiar(p.Estado, siguiente, a, ahora)
}

// Cancelar: el cliente se arrepiente. Solo desde RECIBIDO, antes de que la cocina lo acepte.
func (p *Pedido) Cancelar(a Actor, ahora time.Time) (CambioEstado, error) {
	if err := p.autorizarCliente(a); err != nil {
		return CambioEstado{}, err
	}
	return p.cambiar(EstadoRecibido, EstadoCancelado, a, ahora)
}

// ConfirmarEntrega: el cliente avisa que lo recibió. Solo desde EN_CAMINO.
func (p *Pedido) ConfirmarEntrega(a Actor, ahora time.Time) (CambioEstado, error) {
	if err := p.autorizarCliente(a); err != nil {
		return CambioEstado{}, err
	}
	return p.cambiar(EstadoEnCamino, EstadoEntregado, a, ahora)
}

// autorizarCocina: rol Cocina (si no, 403) y de la misma cocina (si no, 404, para no revelar
// que el pedido existe).
func (p Pedido) autorizarCocina(a Actor) error {
	if err := a.Validar(); err != nil {
		return err
	}
	if a.Rol != RolCocina {
		return ErrSinPermiso
	}
	if a.CocinaID == "" || a.CocinaID != p.CocinaID {
		return ErrNoEncontrado
	}
	return nil
}

func (p Pedido) autorizarCliente(a Actor) error {
	if err := a.Validar(); err != nil {
		return err
	}
	if a.Rol != RolCliente {
		return ErrSinPermiso
	}
	if a.UsuarioID != p.ClienteID {
		return ErrNoEncontrado
	}
	return nil
}

func (p *Pedido) cambiar(desde, hacia Estado, a Actor, ahora time.Time) (CambioEstado, error) {
	if p.Estado != desde {
		return CambioEstado{}, ErrTransicionInvalida
	}
	ahora = ahora.UTC()
	c := CambioEstado{EstadoAnterior: p.Estado, EstadoNuevo: hacia, ActorID: a.UsuarioID, ActorRol: a.Rol, Version: p.Version + 1, OcurridoEn: ahora}
	p.Estado, p.Version, p.ActualizadoEn = hacia, c.Version, ahora
	p.Historial = append(p.Historial, c)
	return c, nil
}

func emailValido(e string) bool {
	if utf8.RuneCountInString(e) > 254 {
		return false
	}
	at := strings.IndexByte(e, '@')
	return at > 0 && at < len(e)-1 && !strings.ContainsAny(e, " \t\r\n")
}
