package indexer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"ghostkitchen/busqueda/internal/clients"
)

// ErrEventoInvalido: el mensaje no cumple el contrato (docs/contracts/eventos). Reintentarlo no
// lo arregla, así que se rechaza y RabbitMQ lo manda a la cola de mensajes fallidos (DLQ).
var ErrEventoInvalido = errors.New("evento inválido")

// Sobre es el sobre común de los eventos (docs/contracts/eventos/sobre.schema.json).
type Sobre struct {
	EventoID      string          `json:"evento_id"`
	Tipo          string          `json:"tipo"`
	Version       int             `json:"version"`
	OcurridoEn    time.Time       `json:"ocurrido_en"`
	CorrelationID string          `json:"correlation_id"`
	AgregadoID    string          `json:"agregado_id"`
	Secuencia     int64           `json:"secuencia"`
	Datos         json.RawMessage `json:"datos"`
}

// LeerSobre valida el sobre. ruta es la routing key con la que llegó: tiene que coincidir con
// el tipo, así un mensaje no se puede hacer pasar por otro.
func LeerSobre(cuerpo []byte, ruta string) (Sobre, error) {
	var s Sobre
	if err := json.Unmarshal(cuerpo, &s); err != nil {
		return Sobre{}, fmt.Errorf("%w: JSON inválido", ErrEventoInvalido)
	}
	switch {
	case s.EventoID == "", s.AgregadoID == "", len(s.Datos) == 0:
		return Sobre{}, fmt.Errorf("%w: faltan campos del sobre", ErrEventoInvalido)
	case s.Tipo != ruta:
		return Sobre{}, fmt.Errorf("%w: el tipo %q no coincide con la ruta %q", ErrEventoInvalido, s.Tipo, ruta)
	case s.Version != 1:
		return Sobre{}, fmt.Errorf("%w: versión %d no soportada", ErrEventoInvalido, s.Version)
	case s.Secuencia <= 0:
		return Sobre{}, fmt.Errorf("%w: secuencia %d", ErrEventoInvalido, s.Secuencia)
	}
	return s, nil
}

// Procesar aplica un evento en el índice. Devuelve ErrEventoInvalido si no cumple el contrato
// (no se reintenta) y cualquier otro error si falló algo pasajero, como Solr caído (se
// reintenta).
//
// Si el mismo evento llega dos veces, la segunda escribe lo mismo; si llega uno viejo, la
// versión guardada es mayor y se ignora (Aplicador).
func (a *Aplicador) Procesar(ctx context.Context, s Sobre) error {
	switch s.Tipo {
	case "cocina.creada", "cocina.actualizada", "cocina.desactivada":
		var c clients.CocinaCatalogo
		if err := leerDatos(s, &c); err != nil {
			return err
		}
		if c.ID != s.AgregadoID || c.Version != s.Secuencia {
			return fmt.Errorf("%w: la cocina no coincide con el sobre", ErrEventoInvalido)
		}
		return a.Cocina(ctx, c)
	case "plato.creado", "plato.actualizado", "plato.desactivado":
		var p clients.PlatoCatalogo
		if err := leerDatos(s, &p); err != nil {
			return err
		}
		if p.ID != s.AgregadoID || p.Version != s.Secuencia || p.CocinaID == "" {
			return fmt.Errorf("%w: el plato no coincide con el sobre", ErrEventoInvalido)
		}
		return a.Plato(ctx, p)
	case "disponibilidad.cambiada":
		var d clients.DisponibilidadInventario
		if err := leerDatos(s, &d); err != nil {
			return err
		}
		if d.PlatoID != s.AgregadoID || d.CocinaID == "" {
			return fmt.Errorf("%w: la disponibilidad no coincide con el sobre", ErrEventoInvalido)
		}
		d.Secuencia = s.Secuencia // el contador viaja en el sobre
		return a.Disponibilidad(ctx, d)
	default:
		return fmt.Errorf("%w: tipo %q desconocido", ErrEventoInvalido, s.Tipo)
	}
}

func leerDatos(s Sobre, destino any) error {
	if err := json.Unmarshal(s.Datos, destino); err != nil {
		return fmt.Errorf("%w: datos de %s inválidos", ErrEventoInvalido, s.Tipo)
	}
	return nil
}
