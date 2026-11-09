package domain

import (
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

var ErrEventoInvalido = errors.New("evento de Catálogo inválido")

type Referencia struct {
	EventoID    string
	Correlacion string
	ID          string
	CocinaID    string
	Activa      bool
	Version     uint64
	EsCocina    bool
}

func LeerReferencia(cuerpo []byte, ruta string) (Referencia, error) {
	var e struct {
		ID          string `json:"evento_id"`
		Tipo        string `json:"tipo"`
		Version     int    `json:"version"`
		Correlacion string `json:"correlation_id"`
		Agregado    string `json:"agregado_id"`
		Secuencia   uint64 `json:"secuencia"`
		Datos       struct {
			ID       string `json:"id"`
			CocinaID string `json:"cocinaId"`
			Activa   *bool  `json:"activa"`
			Activo   *bool  `json:"activo"`
			Version  uint64 `json:"version"`
		} `json:"datos"`
	}
	if err := json.Unmarshal(cuerpo, &e); err != nil || e.Tipo != ruta || e.Version != 1 ||
		e.Secuencia == 0 || e.Secuencia != e.Datos.Version || e.Agregado != e.Datos.ID ||
		!IdentificadorValido(e.Agregado) || len(e.Correlacion) > 64 {
		return Referencia{}, ErrEventoInvalido
	}
	id, err := uuid.Parse(e.ID)
	if err != nil || len(e.ID) != 36 {
		return Referencia{}, ErrEventoInvalido
	}
	ref := Referencia{EventoID: id.String(), Correlacion: e.Correlacion, ID: e.Agregado, Version: e.Secuencia}
	switch e.Tipo {
	case "cocina.creada", "cocina.actualizada", "cocina.desactivada":
		if e.Datos.Activa == nil {
			return Referencia{}, ErrEventoInvalido
		}
		ref.EsCocina = true
		ref.CocinaID = ref.ID
		ref.Activa = *e.Datos.Activa
	case "plato.creado", "plato.actualizado", "plato.desactivado":
		if e.Datos.Activo == nil || !IdentificadorValido(e.Datos.CocinaID) {
			return Referencia{}, ErrEventoInvalido
		}
		ref.CocinaID = e.Datos.CocinaID
		ref.Activa = *e.Datos.Activo
	default:
		return Referencia{}, ErrEventoInvalido
	}
	return ref, nil
}
