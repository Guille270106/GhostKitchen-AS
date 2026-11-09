// Package solr implementa el repositorio de Búsqueda sobre Apache Solr usando su API HTTP.
//
// Es la capa de repositorio (la implementación concreta de repositories.Lector e Indexador).
// Es el único paquete que sabe hablar con Solr: cliente.go hace las llamadas HTTP,
// esquema.go crea los campos y repositorio.go traduce entre el dominio y los documentos.
package solr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"ghostkitchen/busqueda/internal/domain"
	"ghostkitchen/busqueda/internal/trazas"
)

// llamadas mide cada llamada a Solr, por operación (ej. "platos/select") y resultado (ok o
// error). Es el panel "Dependencias" del ADR-011: saber si lo lento es Solr. La operación no
// lleva la URL con parámetros (ADR-011, privacidad).
var llamadas = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "busqueda_solr_duration_seconds",
	Help:    "Duración de las llamadas de Búsqueda a Solr, por operación y resultado.",
	Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.2, 0.3, 0.5, 1, 2, 5},
}, []string{"operacion", "resultado"})

// Cores (índices) que usa Búsqueda. Se crean al levantar Solr (docker-compose).
const (
	CoreCocinas = "cocinas"
	CorePlatos  = "platos"
)

// Cliente habla con Solr por HTTP. No reintenta: quien llama decide (ADR-005).
// Una consulta de usuario tiene un plazo de 3 s; con 2 s por llamada y sin reintentos,
// si Solr no contesta se responde 503 enseguida en lugar de dejar la solicitud colgada.
// Lo que sí reintenta es el arranque de Búsqueda, que es trabajo de fondo (ADR-010).
type Cliente struct {
	base  string       // ej. http://solr:8983/solr
	http  *http.Client // consultas: el límite corto de una solicitud de usuario
	fondo *http.Client // esquema y escrituras: trabajo de fondo, con más tiempo
}

// TimeoutFondo es el límite para cambiar el esquema y escribir documentos. Esas llamadas
// solo las hace el arranque y la reconstrucción del índice, nunca una solicitud de usuario,
// y pueden tardar: al cambiar el esquema Solr recarga el core (la primera vez tardó más de
// 2 s), y cada escritura hace commit (en la traza, casi 0,7 s).
const TimeoutFondo = 15 * time.Second

// NuevoCliente crea un cliente con un timeout por consulta (SOLR_TIMEOUT_MS, 2 s por defecto).
// El esquema y las escrituras usan TimeoutFondo, o el de consulta si es mayor.
func NuevoCliente(baseURL string, timeout time.Duration) *Cliente {
	fondo := TimeoutFondo
	if timeout > fondo {
		fondo = timeout
	}
	return &Cliente{
		base:  strings.TrimRight(baseURL, "/"),
		http:  &http.Client{Timeout: timeout},
		fondo: &http.Client{Timeout: fondo},
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
	err := c.hacer(ctx, c.http, http.MethodGet, "/"+core+"/select?"+params.Encode(), nil, &r)
	return r, err
}

// Get usa el real-time get de Solr: devuelve el documento aunque todavía no se haya hecho commit.
func (c *Cliente) Get(ctx context.Context, core, id string) (json.RawMessage, error) {
	var r struct {
		Doc json.RawMessage `json:"doc"`
	}
	if err := c.hacer(ctx, c.http, http.MethodGet, "/"+core+"/get?id="+url.QueryEscape(id), nil, &r); err != nil {
		return nil, err
	}
	if len(r.Doc) == 0 || string(r.Doc) == "null" {
		return nil, nil
	}
	return r.Doc, nil
}

// Confirmar hace commit en un core: lo escrito con Escribir aparece en /select ya, sin
// esperar el segundo de commitWithin. Se usa al terminar de armar el índice (para que /ready
// diga "listo" con todo visible) y antes de listar los platos de una cocina (G22).
func (c *Cliente) Confirmar(ctx context.Context, core string) error {
	return c.hacer(ctx, c.fondo, http.MethodPost, "/"+core+"/update?commit=true", []any{}, nil)
}

// ErrConflicto es la respuesta 409 de Solr cuando el documento cambió entre la lectura y la
// escritura (el _version_ que se mandó ya no es el guardado).
var ErrConflicto = errors.New("solr: el documento cambió mientras se escribía")

// Escribir agrega o reemplaza documentos (por id) sin esperar el commit: Solr los deja
// visibles en /select en menos de 1 s (commitWithin), y el real-time get ya los ve. Un commit
// por escritura tardaba casi 1 s cada uno (G15, G22). Como se reemplaza por id, guardar dos
// veces lo mismo no duplica nada.
//
// Si un documento trae _version_, Solr solo lo guarda si coincide con el guardado (o si es -1
// y no existe); si no, responde 409 y se devuelve ErrConflicto.
func (c *Cliente) Escribir(ctx context.Context, core string, docs any) error {
	return c.hacer(ctx, c.fondo, http.MethodPost, "/"+core+"/update?commitWithin=1000", docs, nil)
}

// Esquema envía comandos a la Schema API.
func (c *Cliente) Esquema(ctx context.Context, core string, comandos any) error {
	return c.hacer(ctx, c.fondo, http.MethodPost, "/"+core+"/schema", comandos, nil)
}

// NombresEsquema devuelve los nombres de campos o tipos de campo existentes.
// recurso: "fields", "fieldtypes" o "copyfields".
func (c *Cliente) NombresEsquema(ctx context.Context, core, recurso string) (map[string]bool, error) {
	defs, err := c.DefinicionesEsquema(ctx, core, recurso)
	if err != nil {
		return nil, err
	}
	nombres := make(map[string]bool, len(defs))
	for n := range defs {
		nombres[n] = true
	}
	return nombres, nil
}

// DefinicionesEsquema devuelve cada campo, tipo o copia del esquema con su definición
// completa, por nombre (las copias, como "origen->destino"). Sirve para saber si algo
// existe y también si quedó con una definición vieja.
func (c *Cliente) DefinicionesEsquema(ctx context.Context, core, recurso string) (map[string]map[string]any, error) {
	// Solr responde un objeto con "responseHeader" y la lista ("fields", "fieldTypes" o
	// "copyFields"). Solo se lee la lista; el resto de la respuesta se ignora.
	var r struct {
		Fields     []map[string]any `json:"fields"`
		FieldTypes []map[string]any `json:"fieldTypes"`
		CopyFields []map[string]any `json:"copyFields"`
	}
	if err := c.hacer(ctx, c.fondo, http.MethodGet, "/"+core+"/schema/"+recurso, nil, &r); err != nil {
		return nil, err
	}
	items := map[string][]map[string]any{"fields": r.Fields, "fieldtypes": r.FieldTypes, "copyfields": r.CopyFields}[recurso]
	defs := map[string]map[string]any{}
	for _, item := range items {
		if recurso == "copyfields" {
			defs[fmt.Sprint(item["source"])+"->"+fmt.Sprint(item["dest"])] = item
			continue
		}
		defs[fmt.Sprint(item["name"])] = item
	}
	return defs, nil
}

// hacer ejecuta una llamada a Solr dentro de un span propio. El span lleva el core y la
// operación (ej. "solr platos/select"), pero no la URL completa: los parámetros pueden
// tener coordenadas o el texto que buscó el consumidor, y eso no se registra (ADR-011).
//
// Toda falla (no conecta, timeout, Solr responde error o algo que no se entiende) sale
// envuelta en domain.ErrNoDisponible. Así el handler responde 503 sin tener que conocer
// los errores de Solr.
func (c *Cliente) hacer(ctx context.Context, cli *http.Client, metodo, ruta string, cuerpo any, destino any) (err error) {
	operacion := strings.TrimPrefix(strings.SplitN(ruta, "?", 2)[0], "/")
	ctx, span := trazas.Tracer().Start(ctx, "solr "+operacion, trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.system", "solr"),
			attribute.String("db.operation", operacion),
			attribute.String("http.request.method", metodo),
		))
	inicio := time.Now()
	defer func() {
		resultado := "ok"
		if err != nil && !errors.Is(err, ErrConflicto) { // un 409 es parte del protocolo, no una falla
			resultado = "error"
			span.RecordError(err)
			span.SetStatus(codes.Error, "falló la llamada a Solr")
		}
		llamadas.WithLabelValues(operacion, resultado).Observe(time.Since(inicio).Seconds())
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
		// No se incluye err: trae la URL completa, con sus parámetros.
		return fmt.Errorf("solr: armar solicitud para %s", operacion)
	}
	if cuerpo != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("%w: solr %s: %s", domain.ErrNoDisponible, operacion, sinURL(err))
	}
	defer resp.Body.Close()
	trace.SpanFromContext(ctx).SetAttributes(attribute.Int("http.response.status_code", resp.StatusCode))
	datos, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // tope de 10 MiB para no agotar la memoria
	if err != nil {
		return fmt.Errorf("%w: leer respuesta: %v", domain.ErrNoDisponible, err)
	}
	if resp.StatusCode == http.StatusConflict {
		return ErrConflicto
	}
	if resp.StatusCode >= 300 {
		var e errorSolr
		_ = json.Unmarshal(datos, &e)
		return fmt.Errorf("%w: solr %s respondió %d: %s", domain.ErrNoDisponible, operacion, resp.StatusCode, sinTextoCitado(e.Error.Msg))
	}
	if destino != nil {
		if err := json.Unmarshal(datos, destino); err != nil {
			return fmt.Errorf("%w: respuesta inválida: %v", domain.ErrNoDisponible, err)
		}
	}
	return nil
}

// sinURL devuelve el motivo de un error de red sin la URL: Go arma esos errores como
// `Get "http://...?pt=-31.42,-64.18&q=...": connection refused`, y la URL tiene los
// parámetros de la consulta (coordenadas, texto buscado) que no se registran (ADR-011).
func sinURL(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err.Error()
	}
	return err.Error()
}

// textoCitado encuentra lo que Solr pone entre comillas simples en sus mensajes de error.
// Ahí repite la consulta que no pudo entender (ej. "Cannot parse 'hamburguesa (': ...").
var textoCitado = regexp.MustCompile(`'[^']*'`)

// sinTextoCitado deja el motivo del error de Solr (sirve para diagnosticar) pero quita la
// consulta que repite entre comillas, y lo corta a 200 caracteres.
func sinTextoCitado(msg string) string {
	msg = textoCitado.ReplaceAllString(msg, "'…'")
	if r := []rune(msg); len(r) > 200 {
		msg = string(r[:200]) + "…"
	}
	return msg
}
