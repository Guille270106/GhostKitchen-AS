package handlers

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"ghostkitchen/usuarios/internal/clients"
	"ghostkitchen/usuarios/internal/domain"
	"ghostkitchen/usuarios/internal/services"
)

type catalogoDePrueba struct{ caido bool }

func (c catalogoDePrueba) Cocina(_ context.Context, id, _ string) (clients.Cocina, error) {
	switch {
	case c.caido:
		return clients.Cocina{}, clients.ErrCatalogoNoDisponible
	case id == "nueva-cordoba":
		return clients.Cocina{ID: id, Localidad: "Nueva Córdoba", Activa: true}, nil
	case id == "cerro":
		return clients.Cocina{ID: id, Localidad: "Cerro de las Rosas", Activa: true}, nil
	}
	return clients.Cocina{}, clients.ErrCocinaNoExiste
}

func crearCuenta(t *testing.T, cat catalogoDePrueba, rol domain.Rol, cuerpo string) *httptest.ResponseRecorder {
	t.Helper()
	tk := tokensDePrueba(t)
	svc := services.Nuevo(&repoMemoria{}, tk).ConCostoBcrypt(bcrypt.MinCost).ConCatalogo(cat)
	r := Nuevo(svc, slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
	req := httptest.NewRequest(http.MethodPost, "/admin/cuentas-cocina", strings.NewReader(cuerpo))
	if rol != "" {
		u := domain.Usuario{ID: "u1", Email: "x@gk.dev", Rol: rol}
		switch rol {
		case domain.RolAdmin:
			u.Localidad = "Nueva Córdoba"
		case domain.RolCocina:
			u.CocinaAsignadaID = "cerro"
		}
		token, _, _ := tk.Emitir(u)
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

const cuentaValida = `{"nombre":"Cocina NC","email":"cocina.nc@gk.dev","password":"clave-cocina-1","cocinaId":"nueva-cordoba"}`

func TestCrearCuentaCocinaPorHTTP(t *testing.T) {
	rec := crearCuenta(t, catalogoDePrueba{}, domain.RolAdmin, cuentaValida)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"rol":"cocina"`) || !strings.Contains(rec.Body.String(), `"cocinaAsignadaId":"nueva-cordoba"`) {
		t.Fatalf("esperaba 201 con la cuenta Cocina: %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "clave-cocina-1") || strings.Contains(rec.Body.String(), "$2a$") {
		t.Fatal("la respuesta no lleva la contraseña ni el hash")
	}

	casos := []struct {
		nombre string
		cat    catalogoDePrueba
		rol    domain.Rol
		cuerpo string
		estado int
		codigo string
	}{
		{"sin token", catalogoDePrueba{}, "", cuentaValida, 401, "NO_AUTENTICADO"},
		{"un cliente", catalogoDePrueba{}, domain.RolCliente, cuentaValida, 403, "SIN_PERMISO"},
		{"una cuenta Cocina", catalogoDePrueba{}, domain.RolCocina, cuentaValida, 403, "SIN_PERMISO"},
		{"otra localidad", catalogoDePrueba{}, domain.RolAdmin, strings.Replace(cuentaValida, "nueva-cordoba", "cerro", 1), 403, "OTRA_LOCALIDAD"},
		{"cocina inexistente", catalogoDePrueba{}, domain.RolAdmin, strings.Replace(cuentaValida, "nueva-cordoba", "rio-cuarto", 1), 422, "COCINA_INVALIDA"},
		{"Catálogo caído", catalogoDePrueba{caido: true}, domain.RolAdmin, cuentaValida, 503, "CATALOGO_NO_DISPONIBLE"},
		{"campo de más", catalogoDePrueba{}, domain.RolAdmin, strings.Replace(cuentaValida, "}", `,"rol":"admin"}`, 1), 400, "DATOS_INVALIDOS"},
	}
	for _, c := range casos {
		rec := crearCuenta(t, c.cat, c.rol, c.cuerpo)
		if rec.Code != c.estado || !strings.Contains(rec.Body.String(), c.codigo) {
			t.Errorf("%s: esperaba %d %s, obtuve %d %s", c.nombre, c.estado, c.codigo, rec.Code, rec.Body)
		}
	}
	if rec := crearCuenta(t, catalogoDePrueba{caido: true}, domain.RolAdmin, cuentaValida); rec.Header().Get("Retry-After") == "" {
		t.Fatal("con Catálogo caído el 503 lleva Retry-After")
	}
}
