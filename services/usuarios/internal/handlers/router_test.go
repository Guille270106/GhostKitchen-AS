package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"ghostkitchen/usuarios/internal/domain"
	"ghostkitchen/usuarios/internal/services"
)

func init() { gin.SetMode(gin.TestMode) }

// repoFalso es un repositorio en memoria: G5 solo necesita saber si la base está lista.
type repoFalso struct{ caido bool }

func (r repoFalso) Crear(context.Context, domain.Usuario) error { return nil }
func (r repoFalso) PorEmail(context.Context, string) (domain.Usuario, error) {
	return domain.Usuario{}, domain.ErrNoEncontrado
}
func (r repoFalso) PorID(context.Context, string) (domain.Usuario, error) {
	return domain.Usuario{}, domain.ErrNoEncontrado
}
func (r repoFalso) Listo(context.Context) error {
	if r.caido {
		return errors.New("dial tcp mysql:3306: connection refused")
	}
	return nil
}

func pedir(t *testing.T, repo repoFalso, ruta string, headers map[string]string) (*httptest.ResponseRecorder, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	r := Nuevo(services.Nuevo(repo, tokensDePrueba(t)), slog.New(slog.NewJSONHandler(&logs, nil)))
	req := httptest.NewRequest(http.MethodGet, ruta, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec, &logs
}

func TestHealthNoDependeDeMySQL(t *testing.T) {
	rec, _ := pedir(t, repoFalso{caido: true}, "/health", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("/health tenía que dar 200 aunque MySQL esté caído: %d", rec.Code)
	}
}

func TestReady(t *testing.T) {
	if rec, _ := pedir(t, repoFalso{}, "/ready", nil); rec.Code != http.StatusOK {
		t.Fatalf("con MySQL arriba esperaba 200: %d", rec.Code)
	}
	rec, _ := pedir(t, repoFalso{caido: true}, "/ready", map[string]string{"X-Correlation-Id": "prueba-1"})
	if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("con MySQL caído esperaba 503 Problem Details: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	var p map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &p)
	if p["code"] != "SERVICIO_NO_DISPONIBLE" || p["correlationId"] != "prueba-1" {
		t.Fatalf("cuerpo inesperado: %s", rec.Body)
	}
	if strings.Contains(rec.Body.String(), "mysql:3306") {
		t.Fatalf("el error de la base no se muestra: %s", rec.Body)
	}
}

func TestRutaInexistenteDa404ProblemDetails(t *testing.T) {
	rec, _ := pedir(t, repoFalso{}, "/no-existe", nil)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"NO_ENCONTRADO"`) {
		t.Fatalf("esperaba 404 NO_ENCONTRADO: %d %s", rec.Code, rec.Body)
	}
}

func TestCorrelationIdYLog(t *testing.T) {
	rec, logs := pedir(t, repoFalso{}, "/health", map[string]string{"X-Correlation-Id": "demo-7"})
	if rec.Header().Get("X-Correlation-Id") != "demo-7" {
		t.Fatalf("tenía que devolver el mismo correlation id")
	}
	var linea map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &linea); err != nil {
		t.Fatalf("el log tiene que ser una línea JSON: %v (%s)", err, logs)
	}
	if linea["correlation_id"] != "demo-7" || linea["servicio"] != "usuarios" || linea["ruta"] != "/health" {
		t.Fatalf("log inesperado: %v", linea)
	}

	// Un id con caracteres raros se reemplaza por uno nuevo.
	rec, _ = pedir(t, repoFalso{}, "/health", map[string]string{"X-Correlation-Id": "<script>"})
	if got := rec.Header().Get("X-Correlation-Id"); got == "<script>" || len(got) != 36 {
		t.Fatalf("esperaba un id nuevo, obtuve %q", got)
	}
}

func tokensDePrueba(t *testing.T) *services.Tokens {
	t.Helper()
	tk, err := services.NuevosTokens("un-secreto-de-prueba-de-32-bytes!!", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return tk
}
