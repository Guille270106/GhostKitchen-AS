package services

import (
	"context"
	"math"

	"ghostkitchen/busqueda/internal/domain"
	"ghostkitchen/busqueda/internal/geo"
)

// Estados de la cocina propia en la respuesta de proximidad.
const (
	EstadoEnRango    = "EN_RANGO"
	EstadoFueraRango = "FUERA_RANGO"
	EstadoSinSedes   = "SIN_SEDES"
)

// ConsultaProximidad es una consulta ya validada (RN26).
type ConsultaProximidad struct {
	Cliente            domain.Ubicacion
	Sede               domain.Ubicacion
	RadioKm            float64
	IncluirDarkKitchen bool
}

// SedeExterna es el resultado de comparar la sede del consumidor con su cliente.
type SedeExterna struct {
	DistanciaKm    float64 `json:"distanciaKm"`
	DentroDelRango bool    `json:"dentroDelRango"`
}

// DarkKitchen es nuestra cocina activa más cercana al cliente.
type DarkKitchen struct {
	Estado          string             `json:"estado"`
	Sede            *domain.Referencia `json:"sede"`
	DistanciaKm     *float64           `json:"distanciaKm"`
	RadioAplicadoKm float64            `json:"radioAplicadoKm"`
	DentroDelRango  bool               `json:"dentroDelRango"`
}

// ResultadoProximidad es la respuesta de POST /v1/proximidad.
type ResultadoProximidad struct {
	RadioKm     float64      `json:"radioKm"`
	SedeExterna SedeExterna  `json:"sedeExterna"`
	DarkKitchen *DarkKitchen `json:"darkKitchen"`
}

// Proximidad compara la sede externa con el cliente y, si se pide, busca nuestra cocina
// más cercana AL CLIENTE. No guarda ni registra las coordenadas recibidas.
func (b *Busqueda) Proximidad(ctx context.Context, q ConsultaProximidad) (ResultadoProximidad, error) {
	d := geo.DistanciaKm(q.Cliente, q.Sede)
	res := ResultadoProximidad{
		RadioKm: q.RadioKm,
		SedeExterna: SedeExterna{
			DistanciaKm:    geo.Redondear(d, 3),
			DentroDelRango: d <= q.RadioKm, // se compara sin redondear; el límite es inclusivo
		},
	}
	if !q.IncluirDarkKitchen {
		return res, nil // no depende de Solr
	}

	radio := math.Min(q.RadioKm, CoberturaKm)
	dk := &DarkKitchen{RadioAplicadoKm: radio}
	sede, ok, err := b.repo.SedeMasCercana(ctx, q.Cliente)
	if err != nil {
		return ResultadoProximidad{}, err // 503 para toda la consulta
	}
	if !ok {
		dk.Estado = EstadoSinSedes
	} else {
		dist := geo.DistanciaKm(q.Cliente, sede.Ubicacion)
		redondeada := geo.Redondear(dist, 3)
		ref := sede.Ref()
		dk.Sede, dk.DistanciaKm = &ref, &redondeada
		dk.DentroDelRango = dist <= radio
		dk.Estado = EstadoFueraRango
		if dk.DentroDelRango {
			dk.Estado = EstadoEnRango
		}
	}
	res.DarkKitchen = dk
	return res, nil
}
