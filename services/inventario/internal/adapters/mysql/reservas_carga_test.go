package mysql_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"ghostkitchen/inventario/internal/domain"
)

// La prueba atraviesa HTTP -> aplicación -> MySQL. Cada participante tiene su propio
// carrito y clave: no es una prueba de deduplicación de la misma solicitud.
func TestV8UltimoPlato200Solicitudes(t *testing.T) {
	f := prepararReservas(t) // Omite sin TEST_MYSQL=1; requiere el entorno descartable.
	const participantes = 200
	ctx, cancelar := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancelar()

	// La receta necesita 2 unidades del primer ingrediente y 1 del segundo.
	// El segundo es el limitante. Los intentos perdedores pueden tocar el primero
	// antes de fallar: así se comprueba también el rollback de reservas parciales.
	for j, total := range []int64{1000, 1} {
		if _, err := f.db.ExecContext(ctx, "UPDATE ingrediente SET total=? WHERE id=?", total, f.ingredientes[j]); err != nil {
			t.Fatal(err)
		}
	}

	transporte := &http.Transport{MaxConnsPerHost: participantes, MaxIdleConns: participantes, MaxIdleConnsPerHost: participantes}
	defer transporte.CloseIdleConnections()
	cliente := &http.Client{Transport: transporte, Timeout: 30 * time.Second}
	type resultado struct {
		participante        int
		status              int
		id, carrito, codigo string
		err                 error
	}
	resultados := make(chan resultado, participantes)
	salida := make(chan struct{})
	var listos, terminados sync.WaitGroup
	listos.Add(participantes)
	terminados.Add(participantes)
	for n := 0; n < participantes; n++ {
		datos := f.solicitud(fmt.Sprintf("-c-%03d", n), domain.PlatoSolicitado{PlatoID: f.platos[0], Cantidad: 1})
		cuerpo, err := json.Marshal(datos)
		if err != nil {
			t.Fatal(err)
		}
		go func(n int, datos domain.SolicitudReserva, cuerpo []byte) {
			defer terminados.Done()
			listos.Done()
			<-salida
			out := resultado{participante: n}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://inventario:8084/reservas", bytes.NewReader(cuerpo))
			if err != nil {
				out.err = err
				resultados <- out
				return
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", fmt.Sprintf("%s-v8-%03d", f.prefijo, n))
			res, err := cliente.Do(req)
			if err != nil {
				out.err = err
				resultados <- out
				return
			}
			defer res.Body.Close()
			out.status = res.StatusCode
			var body struct {
				ID        string `json:"id"`
				CarritoID string `json:"carritoId"`
				Codigo    string `json:"code"`
			}
			out.err = json.NewDecoder(res.Body).Decode(&body)
			out.id, out.carrito, out.codigo = body.ID, body.CarritoID, body.Codigo
			if out.status == http.StatusCreated && (out.id == "" || out.carrito != datos.CarritoID) {
				out.err = fmt.Errorf("respuesta de creación incompleta o de otro carrito")
			}
			resultados <- out
		}(n, datos, cuerpo)
	}
	listos.Wait()
	inicio := time.Now()
	close(salida)
	terminados.Wait()
	duracion := time.Since(inicio)
	close(resultados)

	exitos, sinStock, inesperados := 0, 0, 0
	var ganador resultado
	for out := range resultados {
		switch {
		case out.err != nil:
			inesperados++
			t.Errorf("participante %d: %v", out.participante, out.err)
		case out.status == http.StatusCreated:
			exitos++
			ganador = out
		case out.status == http.StatusConflict && out.codigo == "SIN_STOCK":
			sinStock++
		default:
			inesperados++
			t.Errorf("participante %d: HTTP %d, código %q", out.participante, out.status, out.codigo)
		}
	}
	if exitos != 1 || sinStock != participantes-1 || inesperados != 0 {
		t.Errorf("respuestas: éxitos=%d, sin stock=%d, inesperadas=%d", exitos, sinStock, inesperados)
	}

	consultar := func(query string, args ...any) int64 {
		t.Helper()
		var n int64
		if err := f.db.QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := consultar("SELECT COUNT(*) FROM reserva WHERE cocina_id=?", f.cocina); n != 1 {
		t.Errorf("quedaron %d reservas; se esperaba una", n)
	}
	if n := consultar("SELECT COUNT(*) FROM reserva WHERE id=? AND carrito_id=? AND cocina_id=? AND estado='ACTIVA'", ganador.id, ganador.carrito, f.cocina); n != 1 {
		t.Error("la reserva persistida no coincide con el ganador HTTP")
	}
	if n := consultar("SELECT COUNT(*) FROM reserva_plato WHERE cocina_id=?", f.cocina); n != 1 {
		t.Errorf("hay %d platos reservados; se esperaba uno", n)
	}
	if n := consultar("SELECT COUNT(*) FROM reserva_plato WHERE reserva_id=? AND plato_id=? AND cantidad=1", ganador.id, f.platos[0]); n != 1 {
		t.Error("detalle del plato ganador incorrecto")
	}
	if n := consultar("SELECT COUNT(*) FROM reserva_item WHERE cocina_id=?", f.cocina); n != 2 {
		t.Errorf("hay %d detalles de ingredientes; se esperaban dos", n)
	}
	for j, esperado := range []int64{2, 1} {
		totalEsperado := []int64{1000, 1}[j]
		if n := consultar("SELECT total FROM ingrediente WHERE id=?", f.ingredientes[j]); n != totalEsperado {
			t.Errorf("ingrediente %d: total=%d, esperado=%d", j, n, totalEsperado)
		}
		if n := consultar("SELECT reservado FROM ingrediente WHERE id=?", f.ingredientes[j]); n != esperado {
			t.Errorf("ingrediente %d: reservado=%d, esperado=%d", j, n, esperado)
		}
		if n := consultar("SELECT cantidad FROM reserva_item WHERE reserva_id=? AND ingrediente_id=?", ganador.id, f.ingredientes[j]); n != esperado {
			t.Errorf("ingrediente %d: detalle=%d, esperado=%d", j, n, esperado)
		}
		if n := consultar("SELECT total-reservado FROM ingrediente WHERE id=?", f.ingredientes[j]); n != totalEsperado-esperado {
			t.Errorf("ingrediente %d: disponible=%d, esperado=%d", j, n, totalEsperado-esperado)
		}
	}
	if n := consultar("SELECT COUNT(*) FROM ingrediente WHERE cocina_id=? AND (reservado<0 OR reservado>total)", f.cocina); n != 0 {
		t.Errorf("%d ingredientes con stock inválido", n)
	}
	if n := consultar(`SELECT COUNT(*) FROM ingrediente i WHERE i.cocina_id=? AND
 i.reservado <> COALESCE((SELECT SUM(ri.cantidad) FROM reserva_item ri WHERE ri.ingrediente_id=i.id),0)`, f.cocina); n != 0 {
		t.Errorf("%d ingredientes no coinciden con el detalle reservado", n)
	}
	if n := consultar("SELECT COUNT(*) FROM idempotencia WHERE operacion='reservar' AND clave LIKE ? AND resultado IS NOT NULL", f.prefijo+"-v8-%"); n != participantes {
		t.Errorf("resultados durables=%d, esperados=%d", n, participantes)
	}
	if t.Failed() {
		return
	}
	t.Logf("%d solicitudes HTTP, 1 reserva exitosa, %d SIN_STOCK, 0 errores técnicos, 0 reservas parciales, 0 sobreventas; duración=%s", participantes, sinStock, duracion.Round(time.Millisecond))
	t.Log("MySQL: reservado=2 y 1; disponible=998 y 0; una reserva, un plato y dos detalles de ingredientes")
}
