// Package repositories define cómo Catálogo lee y escribe sus datos.
// La implementación real usa MongoDB (subpaquete mongo); los tests usan una en memoria.
//
// Es el patrón Repository (ADR-002): services depende de esta interfaz y no de MongoDB, y
// main le pasa la implementación (inyección de dependencias). Así las reglas se prueban sin
// base, y la caché de Memcached (ADR-007) se podrá agregar como un decorator que implementa
// la misma interfaz, sin que el servicio se entere.
package repositories

import (
	"context"
	"time"

	"ghostkitchen/catalogo/internal/domain"
)

// Repositorio es el acceso a cocinas y platos. Toda falla de la base se devuelve
// envuelta en domain.ErrNoDisponible.
type Repositorio interface {
	// ListarCocinas devuelve las cocinas ordenadas por id. Con soloActivas=false incluye las desactivadas.
	ListarCocinas(ctx context.Context, soloActivas bool) ([]domain.Cocina, error)
	// ObtenerCocina devuelve la cocina por id (ok=false si no existe).
	ObtenerCocina(ctx context.Context, id string) (domain.Cocina, bool, error)
	// ListarPlatosDeCocina devuelve los platos activos de una cocina, ordenados por id.
	ListarPlatosDeCocina(ctx context.Context, cocinaID string) ([]domain.Plato, error)
	// ObtenerPlato devuelve el plato por id (ok=false si no existe).
	ObtenerPlato(ctx context.Context, id string) (domain.Plato, bool, error)
	// PaginaPlatos devuelve todos los platos (activos y no) de a páginas, ordenados por id.
	PaginaPlatos(ctx context.Context, page, pageSize int) ([]domain.Plato, int, error)
	// InsertarIniciales guarda cocinas y platos nuevos. No pisa documentos existentes.
	InsertarIniciales(ctx context.Context, cocinas []domain.Cocina, platos []domain.Plato) error
	// Ping verifica que la base responde.
	Ping(ctx context.Context) error
}

// Escritor son las escrituras del admin y de la cocina sobre Catálogo.
//
// Las cocinas se cambian con concurrencia optimista: el servicio lee la cocina, aplica las
// reglas del dominio (4 restaurantes, sin repetir) y guarda la cocina entera solo si su
// versión sigue siendo la que leyó. Si otra solicitud la cambió en el medio, la base no
// guarda nada (domain.ErrConflicto) y el servicio vuelve a empezar con la versión nueva.
// Así dos admins que agregan el quinto restaurante a la vez no pueden pasar los dos.
type Escritor interface {
	// CrearCocina guarda una cocina nueva y su evento en el outbox, juntos. Si el id ya
	// existe, domain.ErrYaExiste y no se guarda ninguno de los dos.
	CrearCocina(ctx context.Context, c domain.Cocina, ev domain.Evento) error
	// ReemplazarCocina guarda c y su evento solo si la versión guardada es versionAnterior.
	// Si no coincide (o la cocina no existe), domain.ErrConflicto y no se guarda nada.
	ReemplazarCocina(ctx context.Context, c domain.Cocina, versionAnterior int64, ev domain.Evento) error
}

// EscritorPlatos son las escrituras de la cuenta Cocina sobre sus platos. Mismo
// criterio que las cocinas: concurrencia optimista con la versión del plato.
type EscritorPlatos interface {
	// ListarPlatosDeCocinaTodos devuelve todos los platos de una cocina, activos o no.
	ListarPlatosDeCocinaTodos(ctx context.Context, cocinaID string) ([]domain.Plato, error)
	// CrearPlato guarda un plato nuevo y su evento, juntos. Si el id ya existe, domain.ErrYaExiste.
	CrearPlato(ctx context.Context, p domain.Plato, ev domain.Evento) error
	// ReemplazarPlato guarda p y su evento solo si la versión guardada es versionAnterior;
	// si no, domain.ErrConflicto.
	ReemplazarPlato(ctx context.Context, p domain.Plato, versionAnterior int64, ev domain.Evento) error
}

// Outbox es la tabla de mensajes pendientes (patrón outbox, ADR-005). Cada escritura guarda su
// evento acá en la misma transacción que el cambio: si el cambio se guardó, el mensaje
// también, aunque RabbitMQ esté caído. El publicador los manda después.
type Outbox interface {
	// Pendientes devuelve hasta limite eventos sin publicar, del más viejo al más nuevo.
	Pendientes(ctx context.Context, limite int) ([]domain.Pendiente, error)
	// MarcarPublicado anota que RabbitMQ confirmó el evento.
	MarcarPublicado(ctx context.Context, eventoID string, cuando time.Time) error
}
