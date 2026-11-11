// Package correlacion guarda el correlation id en el context, para que los adaptadores de
// salida lo pasen a Catálogo e Inventario y se pueda seguir la misma acción en sus logs.
package correlacion

import "context"

type clave struct{}

// Con guarda el correlation id en el context.
func Con(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, clave{}, id)
}

// Desde devuelve el correlation id guardado (vacío si no hay).
func Desde(ctx context.Context) string {
	id, _ := ctx.Value(clave{}).(string)
	return id
}
