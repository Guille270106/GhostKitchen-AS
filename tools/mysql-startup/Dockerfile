FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /mysql-startup .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=build /mysql-startup /usr/local/bin/mysql-startup
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/mysql-startup"]
