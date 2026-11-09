# Contrato del grupo proveedor

Lo que **consumimos** de otro grupo ([ADR-009](../../adr/ADR-009-consumo-del-proveedor.md)): la **API de comprobantes del Grupo 4** (agencia de viajes). Le mandamos el mail del cliente y las líneas del pedido; emite la factura en PDF y se la manda por correo.

| Qué | Valor |
| --- | --- |
| Archivo | [`comprobantes-v1.yaml`](comprobantes-v1.yaml): copia **sin cambios** de su OpenAPI |
| Versión | 1.1.0 |
| De dónde | Su repositorio, `docs/contracts/comprobantes/v1/openapi.yaml`, commit `d040004` del 2026-11-07 |
| URL pública | Todavía no la publicaron (`<COMPLETAR>` en su guía). Mientras tanto usamos su mock |
| Quién lo usa | Pedidos, adaptador `services/pedidos/internal/adapters/proveedor` |

## Cómo lo usamos

- `POST /v1/comprobantes` con `X-API-Key` e `Idempotency-Key: pedido-{id}`: la misma clave nunca emite dos facturas.
- Cuerpo: `{"email": "<mail del cliente>", "items": [{"descripcion": "Burger x2", "precio": 24000}]}`. El precio de cada línea va ya multiplicado por la cantidad, en pesos.
- `201` o `200`: emitida (guardamos su `id` y su `numero`). `400`, `409` o `422`: rechazada, no se reintenta. `401`, `429`, `503` o sin respuesta: se reintenta después con la misma clave.

## Su mock

`docker compose` levanta `comprobantes-mock`: Prism servido desde este archivo. Responde con los ejemplos del contrato y **valida cada solicitud** contra el OpenAPI: si mandamos algo que no cumple su contrato, responde `422`. Pedidos lo usa mientras `PROVIDER_BASE_URL` del `.env` esté vacía.

## Cuando publiquen su URL

1. Pedir la API key una sola vez con el mail del grupo: `POST {URL}/v1/api-keys` con `{"grupo": "grupo-3-ghostkitchen", "email": "<mail del grupo>"}`. La clave llega por correo.
2. Completar `PROVIDER_BASE_URL` y `PROVIDER_API_KEY` en el `.env` (nunca en el repo).
3. Si publican una versión nueva del contrato, reemplazar `comprobantes-v1.yaml` y correr el test de contrato: avisa si algo dejó de ser compatible.
