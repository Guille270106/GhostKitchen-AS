# Etapa 1: compilar el binario de Go.
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/busqueda ./cmd/api

# Etapa 2: imagen chica solo con el binario. Corre con un usuario sin privilegios.
FROM alpine:3.20
RUN adduser -D -u 10001 app
USER app
COPY --from=build /out/busqueda /usr/local/bin/busqueda
EXPOSE 8083
ENTRYPOINT ["busqueda"]
