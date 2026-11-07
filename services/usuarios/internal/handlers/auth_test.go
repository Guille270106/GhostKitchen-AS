package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"ghostkitchen/usuarios/internal/domain"
	"ghostkitchen/usuarios/internal/services"
)

// repoMemoria guarda en memoria y respeta el correo único, como MySQL.
type repoMemoria struct {
	mu    sync.Mutex
	datos []domain.Usuario
}

func (m *repoMemoria) Crear(_ context.Context, u domain.Usuario) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, o := range m.datos {
		if o.Email == u.Email {
			return domain.ErrEmailRepetido
		}
	}
	m.datos = append(m.datos, u)
	return nil
}
func (m *repoMemoria) PorEmail(context.Context, string) (domain.Usuario, error) {
	return domain.Usuario{}, domain.ErrNoEncontrado
}
func (m *repoMemoria) PorID(context.Context, string) (domain.Usuario, error) {
	return domain.Usuario{}, domain.ErrNoEncontrado
}
func (m *repoMemoria) Listo(context.Context) error { return nil }

func pedirRegistro(t *testing.T, repo *repoMemoria, cuerpo string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	var logs bytes.Buffer
	svc := services.Nuevo(repo).ConCostoBcrypt(bcrypt.MinCost)
	r := Nuevo(svc, slog.New(slog.NewJSONHandler(&logs, nil)))
	req := httptest.NewRequest(http.MethodPost, "/auth/registro", strings.NewReader(cuerpo))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec, logs.String()
}

const registroValido = `{"nombre":"Ana","email":"Ana@Mail.com","password":"clave-secreta-1","direccion":"Obispo Trejo 323"}`

func TestRegistroDevuelve201SinContrasena(t *testing.T) {
	repo := &repoMemoria{}
	rec, logs := pedirRegistro(t, repo, registroValido)
	if rec.Code != http.StatusCreated {
		t.Fatalf("esperaba 201: %d %s", rec.Code, rec.Body)
	}
	var u map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &u)
	if u["rol"] != "cliente" || u["email"] != "ana@mail.com" || u["id"] == "" {
		t.Fatalf("cuerpo inesperado: %s", rec.Body)
	}
	if rec.Header().Get("Location") != "/usuarios/"+u["id"].(string) {
		t.Fatalf("Location inesperado: %s", rec.Header().Get("Location"))
	}
	for _, prohibido := range []string{"clave-secreta-1", "password", "$2a$"} {
		if strings.Contains(rec.Body.String(), prohibido) {
			t.Fatalf("la respuesta no puede tener %q: %s", prohibido, rec.Body)
		}
		if strings.Contains(logs, prohibido) || strings.Contains(logs, "ana@mail.com") {
			t.Fatalf("el log no puede tener la contraseña ni el correo: %s", logs)
		}
	}
}

func TestRegistroCorreoRepetidoDa409(t *testing.T) {
	repo := &repoMemoria{}
	pedirRegistro(t, repo, registroValido)
	rec, _ := pedirRegistro(t, repo, strings.Replace(registroValido, `"Ana"`, `"Otra"`, 1))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "EMAIL_REPETIDO") {
		t.Fatalf("esperaba 409 EMAIL_REPETIDO: %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "Ana") || strings.Contains(rec.Body.String(), "Obispo") {
		t.Fatalf("no se muestran datos de la otra cuenta: %s", rec.Body)
	}
}

func TestRegistroDatosInvalidosDa400(t *testing.T) {
	casos := map[string]string{
		"falta la dirección": `{"nombre":"Ana","email":"a@b.com","password":"clave-secreta-1"}`,
		"contraseña corta":   `{"nombre":"Ana","email":"a@b.com","password":"123","direccion":"X 1"}`,
		"correo inválido":    `{"nombre":"Ana","email":"no-es-correo","password":"clave-secreta-1","direccion":"X 1"}`,
		"pide ser admin":     `{"nombre":"Ana","email":"a@b.com","password":"clave-secreta-1","direccion":"X 1","rol":"admin"}`,
		"manda localidad":    `{"nombre":"Ana","email":"a@b.com","password":"clave-secreta-1","direccion":"X 1","localidad":"Córdoba"}`,
		"no es JSON":         `nombre=Ana`,
		"dos objetos":        registroValido + registroValido,
		"cuerpo enorme":      `{"nombre":"` + strings.Repeat("a", 20000) + `"}`,
	}
	for nombre, cuerpo := range casos {
		repo := &repoMemoria{}
		rec, _ := pedirRegistro(t, repo, cuerpo)
		if rec.Code != http.StatusBadRequest || rec.Header().Get("Content-Type") != "application/problem+json" {
			t.Errorf("%s: esperaba 400 Problem Details, obtuve %d %s", nombre, rec.Code, rec.Body)
		}
		if len(repo.datos) != 0 {
			t.Errorf("%s: no tenía que guardar nada", nombre)
		}
	}
	rec, _ := pedirRegistro(t, &repoMemoria{}, casos["falta la dirección"])
	if !strings.Contains(rec.Body.String(), "dirección") {
		t.Fatalf("el 400 tiene que decir qué dato falta: %s", rec.Body)
	}
}
