package domain

type Disponibilidad struct {
	PlatoID     string `json:"plato_id"`
	CocinaID    string `json:"cocina_id"`
	Disponible  bool   `json:"disponible"`
	TieneReceta bool   `json:"tiene_receta"`
}
