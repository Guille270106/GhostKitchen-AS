package domain

import (
	"errors"
	"math"
	"sort"
	"time"
)

var (
	ErrReservaInvalida  = errors.New("se requieren identificadores válidos, platos distintos y cantidades enteras positivas")
	ErrClaveInvalida    = errors.New("Idempotency-Key es obligatoria y admite hasta 128 caracteres ASCII visibles")
	ErrIdempotencia     = errors.New("la clave ya se usó con otros datos")
	ErrReservaConflicto = errors.New("el carrito ya tiene reserva o la reserva no está activa")
	ErrReservaVencida   = errors.New("la reserva venció")
	ErrSinStock         = errors.New("no alcanzan todos los ingredientes")
	ErrEnProceso        = errors.New("la operación sigue en proceso; reintentá con la misma clave")
)

type PlatoSolicitado struct {
	PlatoID  string `json:"platoId"`
	Cantidad int64  `json:"cantidad"`
}
type SolicitudReserva struct {
	CarritoID string            `json:"carritoId"`
	CocinaID  string            `json:"cocinaId"`
	Items     []PlatoSolicitado `json:"items"`
}
type LoteReserva struct {
	Unidades      int64        `json:"unidades"`
	RecetaVersion uint64       `json:"recetaVersion"`
	Ingredientes  []RecetaItem `json:"ingredientes"`
}
type PlatoReservado struct {
	PlatoID  string        `json:"platoId"`
	Cantidad int64         `json:"cantidad"`
	Lotes    []LoteReserva `json:"-"`
}
type Reserva struct {
	ID        string           `json:"id"`
	CarritoID string           `json:"carritoId"`
	CocinaID  string           `json:"cocinaId"`
	Estado    string           `json:"estado"`
	VenceEn   time.Time        `json:"venceEn"`
	Items     []PlatoReservado `json:"items"`
}

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
func ValidarClave(clave string) error {
	if len(clave) == 0 || len(clave) > 128 {
		return ErrClaveInvalida
	}
	for _, c := range clave {
		if c < 33 || c > 126 {
			return ErrClaveInvalida
		}
	}
	return nil
}
func NormalizarItems(items []PlatoSolicitado, permitirVacia bool) ([]PlatoSolicitado, error) {
	if len(items) == 0 && !permitirVacia {
		return nil, ErrReservaInvalida
	}
	copia := append([]PlatoSolicitado{}, items...)
	sort.Slice(copia, func(i, j int) bool { return copia[i].PlatoID < copia[j].PlatoID })
	for i, p := range copia {
		if !IdentificadorValido(p.PlatoID) || p.Cantidad <= 0 || (i > 0 && copia[i-1].PlatoID == p.PlatoID) {
			return nil, ErrReservaInvalida
		}
	}
	return copia, nil
}

// Reducir retira primero las últimas unidades agregadas, con su receta original.
func Reducir(lotes []LoteReserva, unidades int64) []LoteReserva {
	copia := append([]LoteReserva(nil), lotes...)
	for unidades > 0 && len(copia) > 0 {
		n := len(copia) - 1
		quitar := min(unidades, copia[n].Unidades)
		copia[n].Unidades -= quitar
		unidades -= quitar
		if copia[n].Unidades == 0 {
			copia = copia[:n]
		}
	}
	return copia
}

func Totales(platos []PlatoReservado) (map[string]int64, error) {
	totales := map[string]int64{}
	for _, p := range platos {
		for _, l := range p.Lotes {
			for _, i := range l.Ingredientes {
				if l.Unidades <= 0 || i.Cantidad <= 0 || l.Unidades > math.MaxInt64/i.Cantidad {
					return nil, ErrReservaInvalida
				}
				n := l.Unidades * i.Cantidad
				if totales[i.IngredienteID] > math.MaxInt64-n {
					return nil, ErrReservaInvalida
				}
				totales[i.IngredienteID] += n
			}
		}
	}
	return totales, nil
}
