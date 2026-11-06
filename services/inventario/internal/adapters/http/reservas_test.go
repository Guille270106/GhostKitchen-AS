package http_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	adapter "ghostkitchen/inventario/internal/adapters/http"
	"ghostkitchen/inventario/internal/application"
	"ghostkitchen/inventario/internal/domain"
	"github.com/google/uuid"
)

type reservasFake struct {
	llamadas int
	err      error
}

func (f *reservasFake) CrearReserva(_ context.Context, _ string, d domain.SolicitudReserva) (domain.Reserva, error) {
	f.llamadas++
	return domain.Reserva{ID: "r1", CarritoID: d.CarritoID, CocinaID: d.CocinaID, Estado: "ACTIVA", Items: []domain.PlatoReservado{}}, f.err
}
func (f *reservasFake) AjustarReserva(_ context.Context, _, id string, _ []domain.PlatoSolicitado) (domain.Reserva, error) {
	f.llamadas++
	return domain.Reserva{ID: id, Items: []domain.PlatoReservado{}}, f.err
}
func (f *reservasFake) LiberarReserva(_ context.Context, _, id string) (domain.Reserva, error) {
	f.llamadas++
	return domain.Reserva{ID: id, Estado: "LIBERADA", Items: []domain.PlatoReservado{}}, f.err
}
func (f *reservasFake) ObtenerReserva(_ context.Context, id string) (domain.Reserva, error) {
	f.llamadas++
	return domain.Reserva{ID: id, Items: []domain.PlatoReservado{}}, f.err
}

func TestReservasHTTPContratoYValidacion(t *testing.T) {
	repo := &reservasFake{}
	r := adapter.Nuevo(application.Nuevo(&memoria{}), func(context.Context) error { return nil }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	adapter.RegistrarReservas(r, application.NuevasReservas(repo))
	hacer := func(method, path, clave, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Idempotency-Key", clave)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	valido := `{"carritoId":"cart","cocinaId":"cerro","items":[{"platoId":"p17","cantidad":1}]}`
	for _, tc := range []struct{ nombre, body, clave string }{
		{"sin clave", valido, ""}, {"clave larga", valido, strings.Repeat("a", 129)}, {"clave con espacios", valido, "a b"},
		{"sin carrito", `{"cocinaId":"cerro","items":[{"platoId":"p17","cantidad":1}]}`, "k"},
		{"sin cocina", `{"carritoId":"cart","items":[{"platoId":"p17","cantidad":1}]}`, "k"},
		{"lista vacía", `{"carritoId":"cart","cocinaId":"cerro","items":[]}`, "k"},
		{"duplicados", `{"carritoId":"cart","cocinaId":"cerro","items":[{"platoId":"p17","cantidad":1},{"platoId":"p17","cantidad":2}]}`, "k"},
		{"decimal", strings.Replace(valido, `"cantidad":1`, `"cantidad":1.5`, 1), "k"},
		{"cero", strings.Replace(valido, `"cantidad":1`, `"cantidad":0`, 1), "k"},
		{"negativo", strings.Replace(valido, `"cantidad":1`, `"cantidad":-1`, 1), "k"},
		{"nulo", strings.Replace(valido, `"cantidad":1`, `"cantidad":null`, 1), "k"},
		{"overflow", strings.Replace(valido, `"cantidad":1`, `"cantidad":9223372036854775808`, 1), "k"},
		{"campo extra", strings.Replace(valido, `"carritoId"`, `"extra":1,"carritoId"`, 1), "k"},
		{"varios objetos", valido + valido, "k"},
	} {
		t.Run(tc.nombre, func(t *testing.T) {
			rec := hacer("POST", "/reservas", tc.clave, tc.body)
			if rec.Code != 400 {
				t.Fatal(rec.Code, rec.Body.String())
			}
		})
	}
	for _, body := range []string{`{}`, `{"items":null}`} {
		if rec := hacer("PATCH", "/reservas/r1", "k", body); rec.Code != 400 {
			t.Fatal(rec.Code, rec.Body.String())
		}
	}
	if repo.llamadas != 0 {
		t.Fatal("datos inválidos llegaron al repositorio")
	}
	if rec := hacer("POST", "/reservas", "k", valido); rec.Code != 201 || rec.Header().Get("Location") != "/reservas/r1" {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if rec := hacer("PATCH", "/reservas/r1", "ajuste", `{"items":[]}`); rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	if rec := hacer("POST", "/reservas/r1/liberar", "liberar", ""); rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	if rec := hacer("GET", "/reservas/r1", "", ""); rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	for _, tc := range []struct {
		err    error
		code   string
		status int
	}{
		{domain.ErrSinStock, "SIN_STOCK", 409}, {domain.ErrIdempotencia, "IDEMPOTENCIA_CONFLICTO", 409},
		{domain.ErrReservaVencida, "RESERVA_VENCIDA", 409}, {domain.ErrReservaConflicto, "RESERVA_CONFLICTO", 409},
		{domain.ErrEnProceso, "OPERACION_EN_PROCESO", 409}, {domain.ErrNoEncontrado, "NO_ENCONTRADO", 404},
	} {
		repo.err = tc.err
		rec := hacer("POST", "/reservas", "k", valido)
		var problem struct {
			Code string `json:"code"`
		}
		json.Unmarshal(rec.Body.Bytes(), &problem)
		if rec.Code != tc.status || problem.Code != tc.code {
			t.Fatal(rec.Code, rec.Body.String())
		}
	}
}

func TestReservaAPIReal(t *testing.T) {
	if os.Getenv("TEST_MYSQL") != "1" {
		t.Skip("requiere el entorno descartable")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	key := "api-" + uuid.NewString()
	hacer := func(method, path, clave, body string) (int, []byte) {
		t.Helper()
		req, err := http.NewRequest(method, "http://inventario:8084"+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", clave)
		req.Header.Set("X-Usuario-Id", "prueba-reserva")
		req.Header.Set("X-Rol", "cocina")
		req.Header.Set("X-Cocina-Id", "nueva-cordoba")
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode, b
	}
	status, b := hacer("POST", "/cocina/ingredientes", "", `{"nombre":"Reserva API","unidad":"gramos","stockInicial":10}`)
	if status != 201 {
		t.Fatal(status, string(b))
	}
	var ingrediente domain.Ingrediente
	if err := json.Unmarshal(b, &ingrediente); err != nil {
		t.Fatal(err)
	}
	if status, b := hacer("PUT", "/cocina/platos/p3/receta", "", `{"items":[{"ingredienteId":"`+ingrediente.ID+`","cantidad":2}]}`); status != 200 {
		t.Fatal(status, string(b))
	}
	body := `{"carritoId":"` + key + `","cocinaId":"nueva-cordoba","items":[{"platoId":"p3","cantidad":2}]}`
	status, original := hacer("POST", "/reservas", key, body)
	if status != 201 {
		t.Fatal(status, string(original))
	}
	var reserva domain.Reserva
	if err := json.Unmarshal(original, &reserva); err != nil {
		t.Fatal(err)
	}
	status, replay := hacer("POST", "/reservas", key, body)
	if status != 201 || string(replay) != string(original) {
		t.Fatal("replay HTTP", status, string(replay))
	}
	if status, _ := hacer("POST", "/reservas", key, strings.Replace(body, `"cantidad":2`, `"cantidad":3`, 1)); status != 409 {
		t.Fatal("clave reutilizada", status)
	}
	status, b = hacer("PATCH", "/reservas/"+reserva.ID, key+"-ajuste", `{"items":[{"platoId":"p3","cantidad":3}]}`)
	if status != 200 {
		t.Fatal(status, string(b))
	}
	var ajustada domain.Reserva
	json.Unmarshal(b, &ajustada)
	if !ajustada.VenceEn.Equal(reserva.VenceEn) {
		t.Fatal("extendió vencimiento")
	}
	status, b = hacer("GET", "/cocina/ingredientes/"+ingrediente.ID, "", "")
	json.Unmarshal(b, &ingrediente)
	if status != 200 || ingrediente.Reservado != 6 {
		t.Fatal("stock", status, string(b))
	}
	status, b = hacer("GET", "/reservas/"+reserva.ID, "", "")
	if status != 200 {
		t.Fatal(status, string(b))
	}
	for j := 0; j < 2; j++ {
		status, b = hacer("POST", "/reservas/"+reserva.ID+"/liberar", key+"-liberar", "")
		if status != 200 {
			t.Fatal(status, string(b))
		}
	}
	status, b = hacer("GET", "/cocina/ingredientes/"+ingrediente.ID, "", "")
	json.Unmarshal(b, &ingrediente)
	if status != 200 || ingrediente.Reservado != 0 || ingrediente.Total != 10 {
		t.Fatal("liberación duplicada", status, string(b))
	}
}
