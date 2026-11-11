# Contexto de build: raíz del repositorio (igual que Inventario y Usuarios).
FROM golang:1.24-alpine AS migraciones
WORKDIR /src
COPY tools/mysql-startup/go.mod tools/mysql-startup/go.sum ./
RUN go mod download
COPY tools/mysql-startup/ ./
RUN CGO_ENABLED=0 go build -trimpath -o /mysql-startup .

FROM golang:1.24-alpine AS build
WORKDIR /src
COPY services/pedidos/go.mod services/pedidos/go.sum ./
RUN go mod download
COPY services/pedidos/ ./
RUN CGO_ENABLED=0 go build -trimpath -o /pedidos ./cmd/api

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=migraciones /mysql-startup /usr/local/bin/mysql-startup
COPY --from=build /pedidos /app/pedidos
COPY services/pedidos/migrations /migrations
USER 65532:65532
EXPOSE 8085
# Primero aplica las migraciones de pedidos_db y después arranca el servicio.
ENTRYPOINT ["/usr/local/bin/mysql-startup"]
CMD ["/app/pedidos"]
