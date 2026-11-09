# Contexto de build: raíz del repositorio.
FROM golang:1.24-alpine AS migraciones
WORKDIR /src
COPY tools/mysql-startup/go.mod tools/mysql-startup/go.sum ./
RUN go mod download
COPY tools/mysql-startup/ ./
RUN CGO_ENABLED=0 go build -trimpath -o /mysql-startup .

FROM golang:1.24-alpine AS build
WORKDIR /src
COPY services/inventario/go.mod services/inventario/go.sum ./
RUN go mod download
COPY services/inventario/ ./
RUN CGO_ENABLED=0 go build -trimpath -o /inventario ./cmd/api

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=migraciones /mysql-startup /usr/local/bin/mysql-startup
COPY --from=build /inventario /app/inventario
COPY services/inventario/migrations /migrations
USER 65532:65532
EXPOSE 8084
ENTRYPOINT ["/usr/local/bin/mysql-startup"]
CMD ["/app/inventario"]
