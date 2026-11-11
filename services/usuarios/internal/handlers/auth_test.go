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
func (m *repoMemoria) PorEmail(_ context.Context, email string) (domain.Usuario, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.datos {
		if u.Email == email {
			return u, nil
		}
	}
	return domain.Usuario{}, domain.ErrNoEncontrado
}
func (m *repoMemoria) PorID(context.Context, string) (domain.Usuario, error) {
	return domain.Usuario{}, domain.ErrNoEncontrado
}
func (m *repoMemoria) Listo(context.Context) error { return nil }

func pedirRegistro(t *testing.T, repo *repoMemoria, cuerpo string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	return pedirA(t, repo, http.MethodPost, "/auth/registro", cuerpo, "")
}

// pedirA hace una solicitud al router completo y devuelve la respuesta y los logs.
func pedirA(t *testing.T, repo *repoMemoria, metodo, ruta, cuerpo, autorizacion string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	var logs bytes.Buffer
	svc := services.Nuevo(repo, tokensDePrueba(t)).ConCostoBcrypt(bcrypt.MinCost)
	r := Nuevo(svc, slog.New(slog.NewJSONHandler(&logs, nil)))
	req := httptest.NewRequest(metodo, ruta, strings.NewReader(cuerpo))
	req.Header.Set("Content-Type", "application/json")
	if autorizacion != "" {
		req.Header.Set("Authorization", autorizacion)
	}
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

func TestLoginYVerify(t *testing.T) {
	repo := &repoMemoria{}
	pedirRegistro(t, repo, registroValido)

	rec, logs := pedirA(t, repo, http.MethodPost, "/auth/login", `{"email":"ana@mail.com","password":"clave-secreta-1"}`, "")
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("esperaba 200 no-store: %d %s", rec.Code, rec.Body)
	}
	var s struct {
		Token, Tipo string
		Usuario     map[string]any
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &s)
	if s.Token == "" || s.Tipo != "Bearer" || s.Usuario["rol"] != "cliente" {
		t.Fatalf("cuerpo inesperado: %s", rec.Body)
	}
	if strings.Contains(rec.Body.String(), "clave-secreta-1") || strings.Contains(logs, s.Token) || strings.Contains(logs, "clave-secreta-1") {
		t.Fatal("ni la contraseña ni el token van a la respuesta o al log que no corresponde")
	}

	rec, logs = pedirA(t, repo, http.MethodGet, "/auth/verify", "", "Bearer "+s.Token)
	if rec.Code != http.StatusNoContent || rec.Header().Get("X-Rol") != "cliente" || rec.Header().Get("X-Usuario-Id") != s.Usuario["id"] || rec.Header().Get("X-Usuario-Email") != "ana@mail.com" {
		t.Fatalf("verify: %d %v", rec.Code, rec.Header())
	}
	if rec.Header().Get("X-Cocina-Id") != "" || rec.Header().Get("X-Localidad") != "" {
		t.Fatal("un cliente no lleva cocina ni localidad")
	}
	if strings.Contains(logs, s.Token) {
		t.Fatal("el token no se registra en el log")
	}
}

func TestLoginMalDaElMismo401(t *testing.T) {
	repo := &repoMemoria{}
	pedirRegistro(t, repo, registroValido)
	mal, _ := pedirA(t, repo, http.MethodPost, "/auth/login", `{"email":"ana@mail.com","password":"otra-clave-99"}`, "")
	nadie, _ := pedirA(t, repo, http.MethodPost, "/auth/login", `{"email":"nadie@mail.com","password":"clave-secreta-1"}`, "")
	for _, rec := range []*httptest.ResponseRecorder{mal, nadie} {
		if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "CREDENCIALES_INVALIDAS") {
			t.Fatalf("esperaba 401 CREDENCIALES_INVALIDAS: %d %s", rec.Code, rec.Body)
		}
	}
	quitarId := func(s string) string { return s[:strings.Index(s, `"correlationId"`)] }
	if quitarId(mal.Body.String()) != quitarId(nadie.Body.String()) {
		t.Fatalf("las dos respuestas tienen que ser iguales:\n%s\n%s", mal.Body, nadie.Body)
	}
}

func TestVerifySinTokenOConTokenMalo(t *testing.T) {
	for _, auth := range []string{"", "Bearer ", "Basic YWJjOmRlZg==", "Bearer no.es.token"} {
		rec, _ := pedirA(t, &repoMemoria{}, http.MethodGet, "/auth/verify", "", auth)
		if rec.Code != http.StatusUnauthorized || rec.Header().Get("WWW-Authenticate") == "" || rec.Header().Get("X-Usuario-Id") != "" {
			t.Errorf("%q: esperaba 401 con WWW-Authenticate y sin identidad: %d %v", auth, rec.Code, rec.Header())
		}
	}
}
