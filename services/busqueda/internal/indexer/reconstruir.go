// Package indexer mantiene el índice de Búsqueda a partir de los datos de otros servicios.
//
// Entrega 2: reconstrucción completa desde la API de Catálogo (al arrancar y con
// POST /internal/reindexar). Cuando exista RabbitMQ, acá se suma el lector de los
// mensajes cocina.*, plato.* y disponibilidad.cambiada.
//
// Es la segunda entrada del servicio, al lado de los handlers HTTP (ADR-002): no atiende a
// usuarios, sino que copia en Solr datos que son de otros servicios. El índice es una copia
// que se puede tirar y volver a armar en cualquier momento (ADR-003, Clase 5).
package indexer

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"ghostkitchen/busqueda/internal/clients"
	"ghostkitchen/busqueda/internal/domain"
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

// Disponibilidad dice si un plato se puede pedir. Es dato de Inventario: mientras ese
// servicio no exista, se usa una tabla provisional. Un plato desconocido (por ejemplo,
// uno recién creado, todavía sin receta) figura como no disponible.
type Disponibilidad func(platoID string) bool

// Reconstructor arma el índice desde cero con los datos de Catálogo.
type Reconstructor struct {
	fuente FuenteCatalogo
	indice repositories.Indexador
	dispo  Disponibilidad
}

// NuevoReconstructor crea el reconstructor.
func NuevoReconstructor(f FuenteCatalogo, i repositories.Indexador, d Disponibilidad) *Reconstructor {
	return &Reconstructor{fuente: f, indice: i, dispo: d}
}

// Resultado cuenta lo que se indexó.
type Resultado struct {
	Sedes  int `json:"sedes"`
	Platos int `json:"platos"`
}

// Reconstruir lee todas las cocinas y platos de Catálogo y los guarda en el índice.
// Guarda por páginas: si Catálogo falla a mitad de camino, el índice conserva lo anterior
// para los platos que faltaron, y la próxima reconstrucción lo completa.
//
// Los datos se piden a la API interna de Catálogo, nunca a su base: cada servicio es el
// único que entra a la suya (una base por servicio, ADR-001; ARCHITECTURE 6.9). Así Catálogo
// puede cambiar cómo guarda sus datos sin romper a Búsqueda.
func (r *Reconstructor) Reconstruir(ctx context.Context) (res Resultado, err error) {
	// Un span que agrupa toda la reconstrucción: adentro quedan las llamadas a Catálogo
	// (y, por la propagación, lo que Catálogo hace en MongoDB) y las escrituras en Solr.
	ctx, span := trazas.Tracer().Start(ctx, "reconstruir índice")
	defer func() {
		span.SetAttributes(attribute.Int("indice.sedes", res.Sedes), attribute.Int("indice.platos", res.Platos))
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

	// Cada plato se guarda con el nombre de su restaurante y de su cocina copiados adentro
	// (desnormalizado): Solr no hace joins, y así una búsqueda no necesita consultar a
	// Catálogo (ADR-003). Este mapa permite armar esa copia sin pedir nada más.
	type datosRest struct {
		ref    domain.Referencia
		sede   domain.Referencia
		activo bool // el restaurante y su cocina están activos
	}
	restaurantes := map[string]datosRest{}
	sedes := make([]domain.Sede, 0, len(cocinas))
	for _, c := range cocinas {
		s := domain.Sede{
			ID: c.ID, Nombre: c.Nombre, Localidad: c.Localidad, Direccion: c.Direccion,
			Ubicacion: c.Ubicacion, TiempoEntregaMin: c.TiempoEntregaMin, TiempoEntregaMax: c.TiempoEntregaMax,
			CostoEnvio: c.CostoEnvio, Restaurantes: []domain.Referencia{}, Activa: c.Activa,
		}
		// Se recorren también los restaurantes inactivos: no se publican en la sede, pero
		// sus platos se tienen que indexar como inactivos para pisar la copia vieja.
		for _, rest := range c.Restaurantes {
			ref := domain.Referencia{ID: rest.ID, Nombre: rest.Nombre}
			if rest.Activo {
				s.Restaurantes = append(s.Restaurantes, ref)
			}
			restaurantes[rest.ID] = datosRest{ref: ref, sede: s.Ref(), activo: rest.Activo && c.Activa}
		}
		sedes = append(sedes, s)
	}
	if err := r.indice.GuardarSedes(ctx, sedes); err != nil {
		return Resultado{}, fmt.Errorf("guardar sedes: %w", err)
	}

	res = Resultado{Sedes: len(sedes)}
	// Cada página se guarda antes de pedir la siguiente: la memoria no crece con la cantidad
	// de platos, y si algo falla se devuelve res con lo que ya quedó guardado.
	for page := 1; ; page++ {
		pag, err := r.fuente.Platos(ctx, page, TamPagina)
		if err != nil {
			return res, err
		}
		platos := make([]domain.Plato, 0, len(pag.Items))
		for _, p := range pag.Items {
			rest, ok := restaurantes[p.RestauranteID]
			if !ok {
				continue // restaurante desconocido: no se indexa un plato sin dueño
			}
			etq := p.Etiquetas
			if etq == nil {
				etq = []string{}
			}
			moneda := p.Moneda
			if moneda == "" { // el contrato siempre publica una moneda; hoy todo es en pesos
				moneda = "ARS"
			}
			platos = append(platos, domain.Plato{
				ID: p.ID, Nombre: p.Nombre, Descripcion: p.Descripcion, Categoria: p.Categoria,
				Etiquetas: etq, Precio: p.Precio, Moneda: moneda, FotoURL: p.FotoURL,
				Disponible: r.dispo(p.ID),
				// Un plato solo se muestra si él, su restaurante y su cocina están activos (RN19).
				Activo:        p.Activo && rest.activo,
				Restaurante:   rest.ref,
				Sede:          rest.sede,
				ActualizadoEn: p.ActualizadoEn,
			})
		}
		if len(platos) > 0 {
			if err := r.indice.GuardarPlatos(ctx, platos); err != nil {
				return res, fmt.Errorf("guardar platos: %w", err)
			}
		}
		res.Platos += len(platos)
		if page >= pag.TotalPages {
			return res, nil
		}
	}
}
