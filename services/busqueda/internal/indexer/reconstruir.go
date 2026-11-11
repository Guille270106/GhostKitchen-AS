// Package indexer mantiene el índice de Búsqueda a partir de los datos de otros servicios.
//
// Tiene dos caminos que usan las mismas reglas (aplicar.go):
//   - la reconstrucción completa desde Catálogo e Inventario (al arrancar y con
//     POST /internal/reindexar), en reconstruir.go;
//   - los mensajes cocina.*, plato.* y disponibilidad.cambiada, en eventos.go. El que
//     los recibe de RabbitMQ es el paquete mensajeria.
//
// Es la segunda entrada del servicio, al lado de los handlers HTTP (ADR-002): no atiende a
// usuarios, sino que copia en Solr datos que son de otros servicios. El índice es una copia
// que se puede tirar y volver a armar en cualquier momento (ADR-003).
package indexer

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"ghostkitchen/busqueda/internal/clients"
	"ghostkitchen/busqueda/internal/repositories"
	"ghostkitchen/busqueda/internal/trazas"
)

// TamPagina es la cantidad de platos que se pide por llamada a Catálogo. Se pide de a 100
// para no armar respuestas enormes ni acercarse a los 5 s de límite por intento.
const TamPagina = 100

// FuenteCatalogo es lo que se necesita de Catálogo (lo implementa clients.Catalogo).
// Es una interfaz para que los tests prueben la reconstrucción con un Catálogo falso,
// sin levantar el servicio real (ADR-002).
type FuenteCatalogo interface {
	Cocinas(ctx context.Context) ([]clients.CocinaCatalogo, error)
	Platos(ctx context.Context, page, pageSize int) (clients.PaginaPlatos, error)
}

// FuenteDisponibilidad es lo que se necesita de Inventario (lo implementa clients.Inventario).
type FuenteDisponibilidad interface {
	Disponibilidad(ctx context.Context) ([]clients.DisponibilidadInventario, error)
}

// Reconstructor arma el índice con los datos de Catálogo y de Inventario.
//
// No reemplaza los documentos a ciegas: aplica cada cocina, plato y disponibilidad
// con las mismas reglas que los mensajes (Aplicador). Así puede correr a la vez que el lector
// de mensajes sin volver atrás un cambio más nuevo, y no borra la disponibilidad que mandó
// Inventario cuando guarda los datos de Catálogo.
type Reconstructor struct {
	fuente      FuenteCatalogo
	inventario  FuenteDisponibilidad // nil: sin Inventario, se usa provisional
	provisional func(platoID string) bool
	aplicar     *Aplicador
}

// NuevoReconstructor crea el reconstructor. Si inv es nil (no hay INVENTARIO_URL), los platos
// nuevos toman la disponibilidad de provisional, la tabla de los datos de ejemplo.
func NuevoReconstructor(f FuenteCatalogo, inv FuenteDisponibilidad, idx repositories.Actualizador, provisional func(string) bool) *Reconstructor {
	return &Reconstructor{fuente: f, inventario: inv, provisional: provisional, aplicar: NuevoAplicador(idx)}
}

// Resultado cuenta lo que se indexó.
type Resultado struct {
	Sedes  int `json:"sedes"`
	Platos int `json:"platos"`
	// Disponibilidad es cuántos platos se actualizaron con la lista de Inventario. Si
	// Inventario no contestó, la reconstrucción sigue sin ella (SinInventario): los platos
	// conservan lo que tenían y los mensajes la ponen al día.
	Disponibilidad int  `json:"disponibilidad"`
	SinInventario  bool `json:"sinInventario,omitempty"`
}

// Reconstruir lee todas las cocinas y platos de Catálogo y los aplica en el índice; después,
// la disponibilidad de Inventario. Si Catálogo falla a mitad de camino, el índice conserva lo
// anterior para los platos que faltaron, y la próxima reconstrucción lo completa.
//
// Los datos se piden a las API internas, nunca a las bases de otros: cada servicio es el
// único que entra a la suya (una base por servicio, ADR-001; ARCHITECTURE 6.9).
func (r *Reconstructor) Reconstruir(ctx context.Context) (res Resultado, err error) {
	// Un span que agrupa toda la reconstrucción: adentro quedan las llamadas a Catálogo e
	// Inventario (y, por la propagación, lo que hacen en sus bases) y las escrituras en Solr.
	ctx, span := trazas.Tracer().Start(ctx, "reconstruir índice")
	defer func() {
		span.SetAttributes(attribute.Int("indice.sedes", res.Sedes), attribute.Int("indice.platos", res.Platos),
			attribute.Int("indice.disponibilidad", res.Disponibilidad))
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "reconstrucción incompleta")
		}
		span.End()
	}()

	cocinas, err := r.fuente.Cocinas(ctx)
	if err != nil {
		return Resultado{}, err
	}
	for _, c := range cocinas {
		if err := r.aplicar.Cocina(ctx, c); err != nil {
			return res, fmt.Errorf("guardar cocina %s: %w", c.ID, err)
		}
		res.Sedes++
	}

	// Sin Inventario, los platos nuevos usan la tabla provisional; con Inventario, entran no
	// disponibles y los corrige la lista de abajo.
	var inicial func(string) bool
	if r.inventario == nil {
		inicial = r.provisional
	}
	cocinaDe := map[string]bool{}
	for _, c := range cocinas {
		cocinaDe[c.ID] = true
	}
	for page := 1; ; page++ {
		pag, err := r.fuente.Platos(ctx, page, TamPagina)
		if err != nil {
			return res, err
		}
		for _, p := range pag.Items {
			if !cocinaDe[p.CocinaID] {
				continue // cocina desconocida: no se indexa un plato sin dueño
			}
			if err := r.aplicar.PlatoConDisponibilidad(ctx, p, inicial); err != nil {
				return res, fmt.Errorf("guardar plato %s: %w", p.ID, err)
			}
			res.Platos++
		}
		if page >= pag.TotalPages {
			break
		}
	}

	if r.inventario == nil {
		return res, nil
	}
	lista, err := r.inventario.Disponibilidad(ctx)
	if err != nil {
		// No se corta la reconstrucción: sin Inventario, Búsqueda igual puede mostrar el menú.
		// Los platos conservan la disponibilidad que tenían.
		res.SinInventario = true
		span.RecordError(err)
		return res, nil
	}
	for _, d := range lista {
		if err := r.aplicar.Disponibilidad(ctx, d); err != nil {
			return res, fmt.Errorf("guardar disponibilidad de %s: %w", d.PlatoID, err)
		}
		res.Disponibilidad++
	}
	return res, nil
}
