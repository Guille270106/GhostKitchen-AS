# Contexto de build: raíz del repositorio (igual que Inventario).
FROM golang:1.24-alpine AS migraciones
WORKDIR /src
COPY tools/mysql-startup/go.mod tools/mysql-startup/go.sum ./
RUN go mod download
COPY tools/mysql-startup/ ./
RUN CGO_ENABLED=0 go build -trimpath -o /mysql-startup .

FROM golang:1.24-alpine AS build
WORKDIR /src
COPY services/usuarios/go.mod services/usuarios/go.sum ./
RUN go mod download
COPY services/usuarios/ ./
RUN CGO_ENABLED=0 go build -trimpath -o /usuarios ./cmd/api

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=migraciones /mysql-startup /usr/local/bin/mysql-startup
COPY --from=build /usuarios /app/usuarios
COPY services/usuarios/migrations /migrations
USER 65532:65532
EXPOSE 8081
# Primero las migraciones, después el servicio. Si una migración falla, el servicio no arranca.
ENTRYPOINT ["/usr/local/bin/mysql-startup"]
CMD ["/app/usuarios"]
