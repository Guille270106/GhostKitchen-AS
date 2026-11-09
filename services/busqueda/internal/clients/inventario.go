package clients

import "context"

// DisponibilidadInventario es si un plato se puede pedir, según Inventario
// (GET /platos/disponibilidad, y los mensajes disponibilidad.cambiada).
//
// Secuencia es el contador de Inventario para ese plato. La lista de Inventario todavía no
// la trae (queda en 0); si la agrega, la reconstrucción la usa sin cambiar nada más.
type DisponibilidadInventario struct {
	PlatoID     string `json:"plato_id"`
	CocinaID    string `json:"cocina_id"`
	Disponible  bool   `json:"disponible"`
	TieneReceta bool   `json:"tiene_receta"`
	Secuencia   int64  `json:"secuencia,omitempty"`
}

// Inventario llama a la API interna de Inventario, con los mismos límites que Catálogo: 5 s
// por intento y 3 reintentos (ADR-005). Solo se usa para reconstruir el índice.
type Inventario struct {
	lector
}

// NuevoInventario crea el cliente. base es, por ejemplo, http://inventario:8084.
func NuevoInventario(base string) *Inventario {
	return &Inventario{nuevoLector("inventario", base)}
}

// Disponibilidad devuelve la disponibilidad de todos los platos que conoce Inventario.
func (c *Inventario) Disponibilidad(ctx context.Context) ([]DisponibilidadInventario, error) {
	var r struct {
		Items []DisponibilidadInventario `json:"items"`
	}
	err := c.obtener(ctx, "/platos/disponibilidad", &r)
	return r.Items, err
}
