package domain

import "context"

type claveCorrelacion struct{}

func ConCorrelacion(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, claveCorrelacion{}, id)
}

func Correlacion(ctx context.Context) string {
	id, _ := ctx.Value(claveCorrelacion{}).(string)
	return id
}
