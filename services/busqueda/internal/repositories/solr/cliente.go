// Package solr implementa el repositorio de Búsqueda sobre Apache Solr usando su API HTTP.
package solr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"ghostkitchen/busqueda/internal/domain"
	"ghostkitchen/busqueda/internal/trazas"
)

// Cores (índices) que usa Búsqueda. Se crean al levantar Solr (docker-compose).
const (
	CoreCocinas = "cocinas"
	CorePlatos  = "platos"
)

// Cliente habla con Solr por HTTP. No reintenta: quien llama decide (ADR-005).
type Cliente struct {
	base string // ej. http://solr:8983/solr
	http *http.Client
}

// NuevoCliente crea un cliente con un timeout por llamada.
func NuevoCliente(baseURL string, timeout time.Duration) *Cliente {
	return &Cliente{
		base: strings.TrimRight(baseURL, "/"),
		http: &http.Client{Timeout: timeout},
	}
}

// respuestaSelect es la parte de la respuesta de /select que usamos.
type respuestaSelect struct {
	Response struct {
		NumFound int               `json:"numFound"`
		Docs     []json.RawMessage `json:"docs"`
	} `json:"response"`
}

// errorSolr es el formato de error de Solr.
type errorSolr struct {
	Error struct {
		Msg string `json:"msg"`
	} `json:"error"`
}

// Select ejecuta /select en un core.
func (c *Cliente) Select(ctx context.Context, core string, params url.Values) (respuestaSelect, error) {
	var r respuestaSelect
	err := c.hacer(ctx, http.MethodGet, "/"+core+"/select?"+params.Encode(), nil, &r)
	return r, err
}

// Get usa el real-time get de Solr: devuelve el documento aunque todavía no se haya hecho commit.
func (c *Cliente) Get(ctx context.Context, core, id string) (json.RawMessage, error) {
	var r struct {
		Doc json.RawMessage `json:"doc"`
	}
	if err := c.hacer(ctx, http.MethodGet, "/"+core+"/get?id="+url.QueryEscape(id), nil, &r); err != nil {
		return nil, err
	}
	if len(r.Doc) == 0 || string(r.Doc) == "null" {
		return nil, nil
	}
	return r.Doc, nil
}

// Actualizar agrega o reemplaza documentos (por id) y hace commit.
func (c *Cliente) Actualizar(ctx context.Context, core string, docs any) error {
	return c.hacer(ctx, http.MethodPost, "/"+core+"/update?commit=true", docs, nil)
}

// Esquema envía comandos a la Schema API.
func (c *Cliente) Esquema(ctx context.Context, core string, comandos any) error {
	return c.hacer(ctx, http.MethodPost, "/"+core+"/schema", comandos, nil)
}

// NombresEsquema devuelve los nombres de campos o tipos de campo existentes.
// recurso: "fields", "fieldtypes" o "copyfields".
func (c *Cliente) NombresEsquema(ctx context.Context, core, recurso string) (map[string]bool, error) {
	var r map[string][]map[string]any
	if err := c.hacer(ctx, http.MethodGet, "/"+core+"/schema/"+recurso, nil, &r); err != nil {
		return nil, err
	}
	nombres := map[string]bool{}
	clave := map[string]string{"fields": "fields", "fieldtypes": "fieldTypes", "copyfields": "copyFields"}[recurso]
	for _, item := range r[clave] {
		if recurso == "copyfields" {
			nombres[fmt.Sprint(item["source"])+"->"+fmt.Sprint(item["dest"])] = true
			continue
		}
		nombres[fmt.Sprint(item["name"])] = true
	}
	return nombres, nil
}

// hacer ejecuta una llamada a Solr dentro de un span propio. El span lleva el core y la
// operación (ej. "solr platos/select"), pero no la URL completa: los parámetros pueden
// tener coordenadas o el texto que buscó el consumidor, y eso no se registra.
func (c *Cliente) hacer(ctx context.Context, metodo, ruta string, cuerpo any, destino any) (err error) {
	operacion := strings.TrimPrefix(strings.SplitN(ruta, "?", 2)[0], "/")
	ctx, span := trazas.Tracer().Start(ctx, "solr "+operacion, trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.system", "solr"),
			attribute.String("db.operation", operacion),
			attribute.String("http.request.method", metodo),
		))
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "falló la llamada a Solr")
		}
		span.End()
	}()

	var body io.Reader
	if cuerpo != nil {
		b, err := json.Marshal(cuerpo)
		if err != nil {
			return fmt.Errorf("solr: armar cuerpo: %w", err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, metodo, c.base+ruta, body)
	if err != nil {
		return fmt.Errorf("solr: armar solicitud: %w", err)
	}
	if cuerpo != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", domain.ErrNoDisponible, err)
	}
	defer resp.Body.Close()
	trace.SpanFromContext(ctx).SetAttributes(attribute.Int("http.response.status_code", resp.StatusCode))
	datos, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return fmt.Errorf("%w: leer respuesta: %v", domain.ErrNoDisponible, err)
	}
	if resp.StatusCode >= 300 {
		var e errorSolr
		_ = json.Unmarshal(datos, &e)
		return fmt.Errorf("%w: solr respondió %d: %s", domain.ErrNoDisponible, resp.StatusCode, e.Error.Msg)
	}
	if destino != nil {
		if err := json.Unmarshal(datos, destino); err != nil {
			return fmt.Errorf("%w: respuesta inválida: %v", domain.ErrNoDisponible, err)
		}
	}
	return nil
}
