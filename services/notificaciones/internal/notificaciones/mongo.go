package notificaciones

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Registrar conserva el estado y los intentos de un registro ya existente.
// MongoDB garantiza la unicidad del evento con el índice de _id.
func Registrar(ctx context.Context, coleccion *mongo.Collection, pendiente Pendiente) error {
	_, err := coleccion.UpdateOne(ctx, bson.M{"_id": pendiente.EventoID},
		bson.M{"$setOnInsert": pendiente}, options.Update().SetUpsert(true))
	if mongo.IsDuplicateKeyError(err) {
		// Otro registro simultáneo pudo ganar el upsert.
		return coleccion.FindOne(ctx, bson.M{"_id": pendiente.EventoID}).Err()
	}
	return err
}

var ErrOcupado = errors.New("notificación en proceso")

func Reclamar(ctx context.Context, c *mongo.Collection, id, token string) (Pendiente, error) {
	ahora := time.Now().UTC()
	filtro := bson.M{"_id": id, "estado": "PENDIENTE", "$or": bson.A{
		bson.M{"reclamo_hasta": bson.M{"$exists": false}},
		bson.M{"reclamo_hasta": bson.M{"$lte": ahora}},
	}}
	var p Pendiente
	err := c.FindOneAndUpdate(ctx, filtro, bson.M{"$set": bson.M{
		"reclamo": token, "reclamo_hasta": ahora.Add(30 * time.Second),
	}, "$inc": bson.M{"intentos": 1}}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&p)
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return p, err
	}
	if err = c.FindOne(ctx, bson.M{"_id": id}).Decode(&p); err != nil {
		return p, err
	}
	if p.Estado == "ENVIADO" {
		return p, nil
	}
	return p, ErrOcupado
}

func CerrarIntento(ctx context.Context, c *mongo.Collection, id, token string, enviado bool) error {
	actualizacion := bson.M{"$unset": bson.M{"reclamo": "", "reclamo_hasta": ""}}
	if enviado {
		actualizacion["$set"] = bson.M{"estado": "ENVIADO", "enviado_en": time.Now().UTC()}
	}
	res, err := c.UpdateOne(ctx, bson.M{"_id": id, "reclamo": token, "estado": "PENDIENTE"}, actualizacion)
	if err != nil {
		return err
	}
	if res.MatchedCount != 1 {
		return ErrOcupado
	}
	return nil
}
