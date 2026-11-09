package notificaciones

import (
	"context"

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
