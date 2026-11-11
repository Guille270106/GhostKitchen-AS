// Package mongo implementa el repositorio de Catálogo sobre MongoDB (base catalogo_db).
//
// Es la capa de repositorio: el único paquete que sabe hablar con MongoDB. Catálogo es el
// único servicio que entra a catalogo_db; los demás le piden los datos por su API (ADR-001).
package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"

	"ghostkitchen/catalogo/internal/domain"
)

// Colecciones de catalogo_db.
const (
	ColeccionCocinas = "cocinas"
	ColeccionPlatos  = "platos"
	ColeccionOutbox  = "outbox"
)

// RetencionOutbox es cuánto se guarda un evento ya publicado antes de que MongoDB lo borre
// solo (índice TTL). Sirve para revisar qué se mandó; después ya no hace falta.
const RetencionOutbox = 7 * 24 * time.Hour

// Repositorio implementa repositories.Repositorio.
type Repositorio struct {
	cliente *mongo.Client
	cocinas *mongo.Collection
	platos  *mongo.Collection
	outbox  *mongo.Collection
}

// Nuevo crea el repositorio sobre la base indicada.
func Nuevo(cliente *mongo.Client, base string) *Repositorio {
	db := cliente.Database(base)
	return &Repositorio{cliente: cliente, cocinas: db.Collection(ColeccionCocinas), platos: db.Collection(ColeccionPlatos), outbox: db.Collection(ColeccionOutbox)}
}

// CrearIndices crea los índices que usan las consultas. Es idempotente.
//
// El índice (cocinaId, activo, _id) cubre el menú: filtra por cocina y por activo y ya
// devuelve los platos ordenados por _id, sin ordenar en memoria. El de restauranteId es el
// de "platos por restaurante" que pide ADR-003.
func (r *Repositorio) CrearIndices(ctx context.Context) error {
	_, err := r.platos.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "cocinaId", Value: 1}, {Key: "activo", Value: 1}, {Key: "_id", Value: 1}}},
		{Keys: bson.D{{Key: "restauranteId", Value: 1}}},
	})
	if err != nil {
		return envolver(err)
	}
	// Outbox: los pendientes se buscan por publicadoEn vacío, del más viejo al más nuevo; y
	// los publicados se borran solos a los 7 días (TTL: solo actúa si publicadoEn es una fecha).
	_, err = r.outbox.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "publicadoEn", Value: 1}, {Key: "creadoEn", Value: 1}}},
		{Keys: bson.D{{Key: "publicadoEn", Value: 1}}, Options: options.Index().SetName("ttl_publicados").SetExpireAfterSeconds(int32(RetencionOutbox.Seconds()))},
	})
	return envolver(err)
}

// ListarCocinas devuelve las cocinas ordenadas por id, con sus restaurantes embebidos.
func (r *Repositorio) ListarCocinas(ctx context.Context, soloActivas bool) ([]domain.Cocina, error) {
	filtro := bson.D{}
	if soloActivas {
		filtro = bson.D{{Key: "activa", Value: true}}
	}
	cur, err := r.cocinas.Find(ctx, filtro, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, envolver(err)
	}
	cocinas := []domain.Cocina{} // vacío y no nil, para que el JSON sea [] y no null
	if err := cur.All(ctx, &cocinas); err != nil {
		return nil, envolver(err)
	}
	return cocinas, nil
}

// ObtenerCocina devuelve la cocina por id. Que no exista no es una falla de la base: se
// devuelve ok=false sin error, y el servicio decide que eso es un 404.
func (r *Repositorio) ObtenerCocina(ctx context.Context, id string) (domain.Cocina, bool, error) {
	var c domain.Cocina
	err := r.cocinas.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&c)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return domain.Cocina{}, false, nil
	}
	if err != nil {
		return domain.Cocina{}, false, envolver(err)
	}
	return c, true, nil
}

// ListarPlatosDeCocina devuelve los platos activos de una cocina, ordenados por id.
func (r *Repositorio) ListarPlatosDeCocina(ctx context.Context, cocinaID string) ([]domain.Plato, error) {
	filtro := bson.D{{Key: "cocinaId", Value: cocinaID}, {Key: "activo", Value: true}}
	cur, err := r.platos.Find(ctx, filtro, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, envolver(err)
	}
	platos := []domain.Plato{}
	if err := cur.All(ctx, &platos); err != nil {
		return nil, envolver(err)
	}
	return platos, nil
}

// ObtenerPlato devuelve el plato por id (ok=false si no existe), activo o no.
func (r *Repositorio) ObtenerPlato(ctx context.Context, id string) (domain.Plato, bool, error) {
	var p domain.Plato
	err := r.platos.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&p)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return domain.Plato{}, false, nil
	}
	if err != nil {
		return domain.Plato{}, false, envolver(err)
	}
	return p, true, nil
}

// PaginaPlatos devuelve una página de todos los platos y el total. Se ordena por _id para
// que las páginas no se pisen entre sí: sin un orden fijo, MongoDB podría devolver el mismo
// plato en dos páginas.
func (r *Repositorio) PaginaPlatos(ctx context.Context, page, pageSize int) ([]domain.Plato, int, error) {
	total, err := r.platos.CountDocuments(ctx, bson.D{})
	if err != nil {
		return nil, 0, envolver(err)
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "_id", Value: 1}}).
		SetSkip(int64((page - 1) * pageSize)).
		SetLimit(int64(pageSize))
	cur, err := r.platos.Find(ctx, bson.D{}, opts)
	if err != nil {
		return nil, 0, envolver(err)
	}
	platos := []domain.Plato{}
	if err := cur.All(ctx, &platos); err != nil {
		return nil, 0, envolver(err)
	}
	return platos, int(total), nil
}

// InsertarIniciales inserta sin pisar: si un documento ya existe (clave duplicada), lo deja como está.
func (r *Repositorio) InsertarIniciales(ctx context.Context, cocinas []domain.Cocina, platos []domain.Plato) error {
	// Sin orden (ordered=false), un duplicado no corta la inserción: MongoDB sigue con los
	// demás y al final informa qué documentos fallaron.
	sinOrden := options.InsertMany().SetOrdered(false)
	if len(cocinas) > 0 {
		docs := make([]any, len(cocinas))
		for i := range cocinas {
			docs[i] = cocinas[i]
		}
		if _, err := r.cocinas.InsertMany(ctx, docs, sinOrden); err != nil && !soloDuplicados(err) {
			return envolver(err)
		}
	}
	if len(platos) > 0 {
		docs := make([]any, len(platos))
		for i := range platos {
			docs[i] = platos[i]
		}
		if _, err := r.platos.InsertMany(ctx, docs, sinOrden); err != nil && !soloDuplicados(err) {
			return envolver(err)
		}
	}
	return nil
}

// Ping verifica que el servidor primario de MongoDB responde.
func (r *Repositorio) Ping(ctx context.Context) error {
	return envolver(r.cliente.Ping(ctx, readpref.Primary()))
}

// soloDuplicados es true si todos los errores de una inserción múltiple son de clave duplicada.
func soloDuplicados(err error) bool {
	var bwe mongo.BulkWriteException
	if !errors.As(err, &bwe) || bwe.WriteConcernError != nil {
		return false
	}
	for _, e := range bwe.WriteErrors {
		if e.Code != 11000 { // 11000 es el código de MongoDB para clave duplicada
			return false
		}
	}
	return true
}

// envolver convierte cualquier error de MongoDB en domain.ErrNoDisponible, para que las capas
// de arriba respondan 503 sin conocer los errores del driver.
func envolver(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %v", domain.ErrNoDisponible, err)
}

// CrearCocina inserta una cocina nueva y su evento en una sola transacción.
func (r *Repositorio) CrearCocina(ctx context.Context, c domain.Cocina, ev domain.Evento) error {
	return r.enTransaccion(ctx, ev, func(sc mongo.SessionContext) error {
		_, err := r.cocinas.InsertOne(sc, c)
		return err
	})
}

// ReemplazarCocina guarda la cocina entera solo si su versión sigue siendo versionAnterior,
// junto con su evento. El filtro por versión y el reemplazo son una sola operación atómica
// sobre el documento: si otra solicitud ya la cambió, no coincide ningún documento, la
// transacción se cancela y no se guarda ni la cocina ni el evento.
func (r *Repositorio) ReemplazarCocina(ctx context.Context, c domain.Cocina, versionAnterior int64, ev domain.Evento) error {
	return r.enTransaccion(ctx, ev, func(sc mongo.SessionContext) error {
		res, err := r.cocinas.ReplaceOne(sc, bson.D{{Key: "_id", Value: c.ID}, {Key: "version", Value: versionAnterior}}, c)
		if err == nil && res.MatchedCount == 0 {
			return domain.ErrConflicto
		}
		return err
	})
}

// ListarPlatosDeCocinaTodos devuelve todos los platos de una cocina, activos o no (para el
// panel de la cocina). Usa el mismo índice (cocinaId, activo, _id) por su primer campo.
func (r *Repositorio) ListarPlatosDeCocinaTodos(ctx context.Context, cocinaID string) ([]domain.Plato, error) {
	cur, err := r.platos.Find(ctx, bson.D{{Key: "cocinaId", Value: cocinaID}}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, envolver(err)
	}
	platos := []domain.Plato{}
	if err := cur.All(ctx, &platos); err != nil {
		return nil, envolver(err)
	}
	return platos, nil
}

// CrearPlato inserta un plato nuevo y su evento en una sola transacción.
func (r *Repositorio) CrearPlato(ctx context.Context, p domain.Plato, ev domain.Evento) error {
	return r.enTransaccion(ctx, ev, func(sc mongo.SessionContext) error {
		_, err := r.platos.InsertOne(sc, p)
		return err
	})
}

// ReemplazarPlato guarda el plato y su evento solo si su versión sigue siendo versionAnterior.
func (r *Repositorio) ReemplazarPlato(ctx context.Context, p domain.Plato, versionAnterior int64, ev domain.Evento) error {
	return r.enTransaccion(ctx, ev, func(sc mongo.SessionContext) error {
		res, err := r.platos.ReplaceOne(sc, bson.D{{Key: "_id", Value: p.ID}, {Key: "version", Value: versionAnterior}}, p)
		if err == nil && res.MatchedCount == 0 {
			return domain.ErrConflicto
		}
		return err
	})
}

// registroOutbox es como se guarda un evento en la colección outbox.
type registroOutbox struct {
	ID          string     `bson:"_id"`
	Tipo        string     `bson:"tipo"`
	AgregadoID  string     `bson:"agregadoId"`
	Sobre       string     `bson:"sobre"` // el JSON tal cual se va a publicar
	CreadoEn    time.Time  `bson:"creadoEn"`
	PublicadoEn *time.Time `bson:"publicadoEn"`
}

// enTransaccion corre el cambio y guarda el evento en la misma transacción de MongoDB
// (patrón outbox, ADR-005): o se guardan los dos, o ninguno. Por eso MongoDB corre como
// replica set: sin eso no hay transacciones.
//
// WithTransaction reintenta sola si la transacción choca con otra (errores transitorios);
// un error nuestro (ErrConflicto, clave duplicada) la cancela y se devuelve tal cual.
func (r *Repositorio) enTransaccion(ctx context.Context, ev domain.Evento, cambio func(mongo.SessionContext) error) error {
	sobre, err := ev.Sobre()
	if err != nil {
		return fmt.Errorf("armar el evento %s: %w", ev.Tipo, err)
	}
	sesion, err := r.cliente.StartSession()
	if err != nil {
		return envolver(err)
	}
	defer sesion.EndSession(ctx)
	_, err = sesion.WithTransaction(ctx, func(sc mongo.SessionContext) (any, error) {
		if err := cambio(sc); err != nil {
			return nil, err
		}
		_, err := r.outbox.InsertOne(sc, registroOutbox{
			ID: ev.EventoID, Tipo: ev.Tipo, AgregadoID: ev.AgregadoID, Sobre: string(sobre), CreadoEn: ev.OcurridoEn,
		})
		return nil, err
	})
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrConflicto):
		return domain.ErrConflicto
	case mongo.IsDuplicateKeyError(err):
		return domain.ErrYaExiste
	default:
		return envolver(err)
	}
}

// Pendientes devuelve los eventos sin publicar, del más viejo al más nuevo.
func (r *Repositorio) Pendientes(ctx context.Context, limite int) ([]domain.Pendiente, error) {
	cur, err := r.outbox.Find(ctx, bson.D{{Key: "publicadoEn", Value: nil}},
		options.Find().SetSort(bson.D{{Key: "creadoEn", Value: 1}, {Key: "_id", Value: 1}}).SetLimit(int64(limite)))
	if err != nil {
		return nil, envolver(err)
	}
	var regs []registroOutbox
	if err := cur.All(ctx, &regs); err != nil {
		return nil, envolver(err)
	}
	out := make([]domain.Pendiente, len(regs))
	for i, reg := range regs {
		out[i] = domain.Pendiente{EventoID: reg.ID, Tipo: reg.Tipo, Sobre: []byte(reg.Sobre)}
	}
	return out, nil
}

// MarcarPublicado anota cuándo RabbitMQ confirmó el evento.
func (r *Repositorio) MarcarPublicado(ctx context.Context, eventoID string, cuando time.Time) error {
	_, err := r.outbox.UpdateOne(ctx, bson.D{{Key: "_id", Value: eventoID}}, bson.D{{Key: "$set", Value: bson.D{{Key: "publicadoEn", Value: cuando.UTC()}}}})
	return envolver(err)
}
