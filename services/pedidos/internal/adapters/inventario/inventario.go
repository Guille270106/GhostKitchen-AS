// Package inventario es el adaptador de salida hacia Inventario. Implementa ports.Inventario.
// Usa las rutas internas de reservas (V7): no pasan por el gateway.
package inventario

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"ghostkitchen/pedidos/internal/adapters/correlacion"
	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/ports"
	"github.com/sony/gobreaker"
)

// InventarioHTTP le habla a Inventario por HTTP (dentro de la red de Docker).
type InventarioHTTP struct {
	BaseURL string // por ejemplo http://inventario:8084
	Client  *http.Client
	// BreakerConsumo es el circuit breaker del descuento (ADR-010: uno por operación).
	BreakerConsumo *gobreaker.CircuitBreaker
}

// Nuevo arma el cliente con el tiempo máximo de cada intento (800 ms, ADR-005).
func Nuevo(baseURL string) InventarioHTTP {
	return InventarioHTTP{
		BaseURL:        baseURL,
		Client:         &http.Client{Timeout: 800 * time.Millisecond},
		BreakerConsumo: NuevoBreaker("inventario-consumir"),
	}
}

// NuevoBreaker arma un circuit breaker con los valores del ADR-010:
//   - si en una ventana de al menos 10 llamadas falla la mitad, se ABRE: durante 10 s responde
//     error al instante, sin llamar a Inventario (así no lo saturamos mientras se recupera);
//   - después deja pasar hasta 3 llamadas de prueba (semiabierto) y, si andan, se cierra.
//
// Solo cuentan como falla las veces que Inventario no contestó. Que la reserva haya vencido es
// una respuesta normal del negocio, no una falla de Inventario.
func NuevoBreaker(nombre string) *gobreaker.CircuitBreaker {
	return gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        nombre,
		MaxRequests: 3,                // llamadas de prueba en semiabierto
		Interval:    60 * time.Second, // cada cuánto se reinician los contadores si está cerrado
		Timeout:     10 * time.Second, // cuánto queda abierto
		ReadyToTrip: func(c gobreaker.Counts) bool {
			return c.Requests >= 10 && c.TotalFailures*2 >= c.Requests
		},
		IsSuccessful: func(err error) bool {
			return !errors.Is(err, domain.ErrInventarioCaido)
		},
	})
}

// Consumir descuenta la reserva para el pedido (V10 de Inventario). Pasa por el circuit
// breaker; los reintentos los hace la saga con backoff, por eso acá se intenta una sola vez.
func (i InventarioHTTP) Consumir(ctx context.Context, clave, reservaID, pedidoID string) error {
	_, err := i.BreakerConsumo.Execute(func() (any, error) {
		return nil, i.intentar(ctx, http.MethodPost, "/reservas/"+reservaID+"/consumir", clave, map[string]any{"pedidoId": pedidoID}, nil)
	})
	// Con el circuito abierto no se llamó a Inventario: para la saga es "no sabemos".
	if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
		return domain.ErrInventarioCaido
	}
	return err
}

// reservaJSON es la parte de la reserva que devuelve Inventario y usamos.
type reservaJSON struct {
	ID      string    `json:"id"`
	VenceEn time.Time `json:"venceEn"`
}

// errorJSON es el Problem Details que responde Inventario cuando algo sale mal.
type errorJSON struct {
	Code string `json:"code"`
}

// Reservar crea la reserva del carrito.
func (i InventarioHTTP) Reservar(ctx context.Context, clave, carritoID, cocinaID string, items []ports.ItemReserva) (string, time.Time, error) {
	cuerpo := map[string]any{"carritoId": carritoID, "cocinaId": cocinaID, "items": items}
	var reserva reservaJSON
	if err := i.enviar(ctx, http.MethodPost, "/reservas", clave, cuerpo, &reserva); err != nil {
		return "", time.Time{}, err
	}
	return reserva.ID, reserva.VenceEn, nil
}

// Ajustar deja en la reserva exactamente estos platos y cantidades.
func (i InventarioHTTP) Ajustar(ctx context.Context, clave, reservaID string, items []ports.ItemReserva) error {
	return i.enviar(ctx, http.MethodPatch, "/reservas/"+reservaID, clave, map[string]any{"items": items}, nil)
}

// Liberar devuelve todos los ingredientes de la reserva.
func (i InventarioHTTP) Liberar(ctx context.Context, clave, reservaID string) error {
	return i.enviar(ctx, http.MethodPost, "/reservas/"+reservaID+"/liberar", clave, map[string]any{}, nil)
}

// enviar hace la solicitud con su Idempotency-Key. Si no hay respuesta, reintenta UNA vez con
// la MISMA clave: si el primer intento sí llegó, Inventario lo reconoce y no reserva dos veces.
func (i InventarioHTTP) enviar(ctx context.Context, metodo, ruta, clave string, cuerpo any, destino any) error {
	err := i.intentar(ctx, metodo, ruta, clave, cuerpo, destino)
	if errors.Is(err, domain.ErrInventarioCaido) {
		err = i.intentar(ctx, metodo, ruta, clave, cuerpo, destino)
	}
	return err
}

// intentar hace un intento.
func (i InventarioHTTP) intentar(ctx context.Context, metodo, ruta, clave string, cuerpo any, destino any) error {
	// 1. Armamos la solicitud con la clave y el correlation id.
	datos, err := json.Marshal(cuerpo)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, metodo, i.BaseURL+ruta, bytes.NewReader(datos))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", clave)
	req.Header.Set("X-Correlation-Id", correlacion.Desde(ctx))

	// 2. La mandamos. Sin respuesta (timeout o conexión caída): Inventario no está disponible.
	resp, err := i.Client.Do(req)
	if err != nil {
		return domain.ErrInventarioCaido
	}
	defer resp.Body.Close()

	// 3. Salió bien: leemos la respuesta si nos interesa.
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		if destino != nil {
			if err := json.NewDecoder(resp.Body).Decode(destino); err != nil {
				return domain.ErrInventarioCaido
			}
		}
		return nil
	}

	// 4. Salió mal: traducimos el código de Inventario a un error de nuestro dominio.
	var problema errorJSON
	_ = json.NewDecoder(resp.Body).Decode(&problema)
	switch problema.Code {
	case "SIN_STOCK":
		return domain.ErrSinStock
	case "RESERVA_VENCIDA":
		return domain.ErrCarritoVencido
	case "NO_ENCONTRADO":
		// El plato no tiene receta cargada todavía: no se puede reservar.
		return domain.ErrPlatoNoDisponible
	case "IDEMPOTENCIA_CONFLICTO", "RESERVA_CONFLICTO":
		// Otro cambio del mismo carrito llegó a la vez.
		return domain.ErrCarritoModificado
	default:
		// 503, OPERACION_EN_PROCESO u otra falla de Inventario.
		return domain.ErrInventarioCaido
	}
}
