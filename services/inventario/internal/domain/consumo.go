package domain

import "time"

type Consumo struct {
	PedidoID    string     `json:"pedidoId"`
	ReservaID   string     `json:"reservaId"`
	Estado      string     `json:"estado"`
	ConsumidoEn time.Time  `json:"consumidoEn"`
	DevueltoEn  *time.Time `json:"devueltoEn,omitempty"`
}
