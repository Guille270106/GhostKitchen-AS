# Contrato del grupo proveedor

Lo que **consumimos** de otro grupo ([ADR-009](../../adr/ADR-009-consumo-del-proveedor.md)): la **API de comprobantes del Grupo 4** (agencia de viajes). Le mandamos el mail del cliente y las líneas del pedido; emite la factura en PDF y se la manda por correo.

| Qué | Valor |
| --- | --- |
| Archivo | [`comprobantes-v1.yaml`](comprobantes-v1.yaml): copia **sin cambios** de su OpenAPI |
| Versión | 1.1.0 |
| De dónde | Su repositorio, `docs/contracts/comprobantes/v1/openapi.yaml`, commit `d040004` del 2026-11-07. Comparado de nuevo el 2026-11-10 con el publicado: igual, salvo la URL del servidor |
| URL pública | `https://68-211-124-251.sslip.io` (publicada el 2026-11-10). Funciona: emitió facturas reales de nuestros pedidos |
| Quién lo usa | Pedidos, adaptador `services/pedidos/internal/adapters/proveedor` |

## Cómo lo usamos

- `POST /v1/comprobantes` con `X-API-Key` e `Idempotency-Key: pedido-{id}`: la misma clave nunca emite dos facturas.
- Cuerpo: `{"email": "<mail del cliente>", "items": [{"descripcion": "Burger x2", "precio": 24000}]}`. El precio de cada línea va ya multiplicado por la cantidad, en pesos.
- `201` o `200`: emitida (guardamos su `id` y su `numero`). `400`, `409` o `422`: rechazada, no se reintenta. `401`, `429`, `503` o sin respuesta: se reintenta después con la misma clave.

## Su mock

`docker compose` levanta `comprobantes-mock`: Prism servido desde este archivo. Responde con los ejemplos del contrato y **valida cada solicitud** contra el OpenAPI: si mandamos algo que no cumple su contrato, responde `422`. Pedidos lo usa si `PROVIDER_BASE_URL` del `.env` está vacía (por ejemplo, en una compu sin la clave), y el test de contrato corre siempre contra él.

## Cómo se conecta al servicio real

1. La API key se pide una sola vez: `POST https://68-211-124-251.sslip.io/v1/api-keys` con el nombre del grupo y un mail. La clave llega a ese mail (revisar spam) y empieza con `cmp_`. Si se pierde, se pide otra con el mismo grupo y mail: la anterior sigue andando.
2. Se completan `PROVIDER_BASE_URL=https://68-211-124-251.sslip.io` (sin `/v1`) y `PROVIDER_API_KEY` en el `.env` de cada compu, **nunca en el repo ni en un chat**, y se reinicia Pedidos (`docker compose up -d pedidos`).
3. Si publican una versión nueva del contrato, reemplazar `comprobantes-v1.yaml` y correr el test de contrato: avisa si algo dejó de ser compatible.
