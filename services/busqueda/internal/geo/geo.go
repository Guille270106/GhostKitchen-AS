// Package geo calcula distancias en línea recta sobre la Tierra.
package geo

import (
	"math"

	"ghostkitchen/busqueda/internal/domain"
)

// RadioTierraKm es el radio medio terrestre. Es el mismo que usa Solr en geodist().
const RadioTierraKm = 6371.0087714

// DistanciaKm calcula la distancia Haversine entre dos puntos, en km, sin redondear.
func DistanciaKm(a, b domain.Ubicacion) float64 {
	lat1, lat2 := rad(a.Lat), rad(b.Lat)
	dLat := lat2 - lat1
	dLng := rad(b.Lng - a.Lng)
	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * RadioTierraKm * math.Asin(math.Min(1, math.Sqrt(h)))
}

// Redondear redondea a la cantidad de decimales indicada (solo para mostrar).
func Redondear(v float64, decimales int) float64 {
	f := math.Pow(10, float64(decimales))
	return math.Round(v*f) / f
}

func rad(g float64) float64 { return g * math.Pi / 180 }
