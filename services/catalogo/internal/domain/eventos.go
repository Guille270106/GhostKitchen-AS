package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Evento es un mensaje para RabbitMQ con el sobre común de docs/contracts/eventos: lo mismo
// para todos los servicios, y lo que cambia según el tipo va en Datos.
//
// Catálogo publica cocina.* y plato.* (los escuchan Búsqueda e Inventario). Agregado y
// Secuencia son el id y la versión de la cocina o el plato: el consumidor descarta un
// mensaje con una versión que ya aplicó, así un mensaje viejo que llega tarde no pisa uno nuevo.
type Evento struct {
	EventoID      string    `json:"evento_id"`
	Tipo          string    `json:"tipo"`
	Version       int       `json:"version"`
	OcurridoEn    time.Time `json:"ocurrido_en"`
	CorrelationID string    `json:"correlation_id,omitempty"`
	AgregadoID    string    `json:"agregado_id"`
	Secuencia     int64     `json:"secuencia"`
	Datos         any       `json:"datos"`
}

// Sobre es el JSON que se publica, tal cual. Se arma una sola vez, al guardar el evento en el
// outbox: si se publica dos veces (un reintento), las dos copias son idénticas.
func (e Evento) Sobre() ([]byte, error) { return json.Marshal(e) }

// Tipos de evento de Catálogo.
const (
	CocinaCreada      = "cocina.creada"
	CocinaActualizada = "cocina.actualizada"
	CocinaDesactivada = "cocina.desactivada"
	PlatoCreado       = "plato.creado"
	PlatoActualizado  = "plato.actualizado"
	PlatoDesactivado  = "plato.desactivado"
)

// EventoCocina arma el evento de un cambio de cocina. Lleva la cocina completa (como
// GET /internal/cocinas), para que el consumidor no tenga que volver a pedirla.
func EventoCocina(tipo string, c Cocina, correlationID string, ahora time.Time) Evento {
	return nuevo(tipo, c.ID, c.Version, c, correlationID, ahora)
}

// EventoPlato arma el evento de un cambio de plato, con el plato completo.
func EventoPlato(tipo string, p Plato, correlationID string, ahora time.Time) Evento {
	return nuevo(tipo, p.ID, p.Version, p, correlationID, ahora)
}

// TipoCambio decide si un cambio es una desactivación o una actualización.
func TipoCambio(estabaActivo, quedaActivo bool, actualizado, desactivado string) string {
	if estabaActivo && !quedaActivo {
		return desactivado
	}
	return actualizado
}

func nuevo(tipo, agregado string, version int64, datos any, correlationID string, ahora time.Time) Evento {
	return Evento{
		EventoID: uuid.NewString(), Tipo: tipo, Version: 1, OcurridoEn: ahora.UTC(),
		CorrelationID: correlationID, AgregadoID: agregado, Secuencia: version, Datos: datos,
	}
}

// Pendiente es un evento guardado en el outbox que todavía no se publicó.
type Pendiente struct {
	EventoID string
	Tipo     string
	Sobre    []byte
}
