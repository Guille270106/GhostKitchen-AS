# ADR-009 — Lo que consumimos de otro grupo

- **Estado:** Aceptada (2026-11-09). Reemplaza la propuesta del 2026-10-16, escrita antes de que la cátedra asignara el proveedor
- **Fecha:** 2026-10-16, completada el 2026-11-09
- **Responsable:** Lucía Rucci
- **Decisión del enunciado:** D9 (cómo incorporamos la capacidad externa, cómo nos adaptamos a su contrato y qué hacemos ante errores, cambios o caídas)
- **Relacionado con:** [ADR-005](ADR-005-comunicacion-entre-servicios.md) · [ADR-010](ADR-010-resiliencia.md) · [ADR-008](ADR-008-contrato-propio.md) · [Contrato del proveedor](../contracts/proveedor/README.md)

## Contexto

El enunciado pide incorporar al flujo principal una capacidad que publica otro grupo, con un test de contrato capaz de detectar cambios incompatibles, tiempos de espera y errores definidos, una decisión sobre qué ve el usuario cuando el proveedor falla, y la demostración en la jornada de fallas.

La cátedra nos asignó la **API de comprobantes del Grupo 4** (agencia de viajes): recibe el mail de un cliente y una lista de líneas con descripción y precio, emite la factura en PDF y se la manda por correo. Su contrato es un OpenAPI 1.1.0 con:

- `POST /v1/comprobantes`, autenticado con `X-API-Key` y con `Idempotency-Key` obligatoria (8 a 64 caracteres): la misma clave con el mismo cuerpo devuelve la factura ya emitida (`200`) y **no** manda otro correo;
- respuestas `201` / `200` (emitida), `400` / `409` / `422` (rechazada), `401`, `429` y `503`;
- su recomendación: esperar hasta 5 s, reintentar 2 o 3 veces con la misma clave, y que una falla suya **no bloquee nuestra compra**.

Es un sistema que **no controlamos**: puede cambiar, ponerse lento o caerse sin avisarnos, y todavía no publicó su URL pública.

## Decisión

### 1. Dónde entra en el flujo: después de crear el pedido, en segundo plano

La factura **no es un paso esencial** del pedido: si el proveedor no está, el cliente igual tiene que poder comprar. Por eso no la pedimos dentro de la confirmación (que tiene 3 s y donde 5 s de espera no entran), sino después:

1. Al completar la confirmación, en la **misma transacción** que crea el pedido, se anota su factura como `PENDIENTE` (tabla `comprobante` de Pedidos). Si el pedido existe, su factura quedó anotada sí o sí.
2. Un proceso de Pedidos la pide cada 10 s, con los datos del pedido: el mail del cliente y una línea por plato (`"Burger x2"`, con el importe ya multiplicado y el precio que se copió al confirmar).
3. El cliente ve su pedido confirmado al instante y la factura le llega por mail unos segundos después. El detalle del pedido muestra cómo va: `PENDIENTE`, `EMITIDO` (con el número) o `RECHAZADO`.

Lo llama **Pedidos**, nunca el frontend ni nuestro gateway: así su API key no sale del backend ([ADR-005](ADR-005-comunicacion-entre-servicios.md)).

### 2. Una capa propia entre su contrato y nuestro dominio (anticorrupción)

Pedidos es hexagonal ([ADR-002](ADR-002-arquitectura-interna.md)). El núcleo define el puerto `Proveedor.Emitir(clave, SolicitudComprobante)` con nuestras palabras (mail y líneas con importe), y el adaptador `adapters/proveedor` traduce a su contrato (`email`, `items[].descripcion`, `items[].precio`) y sus respuestas a nuestros errores:

| Su respuesta | Nuestro error | Qué hacemos |
| --- | --- | --- |
| `201` o `200` | — | `EMITIDO`, guardamos su `id` y su `numero` |
| `400`, `409`, `422` | `ErrProveedorRechazo` | `RECHAZADO` con el motivo: reintentar no lo arregla |
| `401`, `429`, `503`, respuesta que no entendemos, o sin respuesta en 5 s | `ErrProveedorCaido` | Sigue `PENDIENTE` y se reintenta |

Un cambio en sus nombres o formatos se arregla en el adaptador, sin tocar las reglas de Pedidos. `401` se trata como "reintentar" porque casi siempre es la clave mal configurada en el `.env`: cuando se corrige, las facturas pendientes salen solas.

### 3. Contrato, mock y test de contrato

- Guardamos **su OpenAPI sin cambios** en [`docs/contracts/proveedor/comprobantes-v1.yaml`](../contracts/proveedor/comprobantes-v1.yaml), con la versión y el commit de su repositorio.
- **Su mock con Prism** (`comprobantes-mock` en `docker-compose.yml`) se sirve desde ese archivo. Mientras no publiquen su URL, Pedidos lo usa: `PROVIDER_BASE_URL` y `PROVIDER_API_KEY` vacías en el `.env` apuntan al mock.
- **Test de contrato** (`adapters/proveedor/contrato_test.go`, corre en `testdata/compose.yml`): le manda a su mock lo que arma nuestro adaptador, incluidos los casos límite (nombre largo, línea de importe 0, la clave más larga). Prism valida cada solicitud contra su OpenAPI y responde `422` si algo no cumple; además, el adaptador tiene que entender su respuesta de ejemplo. Un segundo test comprueba que el mock de verdad rechaza lo inválido (sin API key, un campo de más, sin líneas), así el primero no pasa "porque sí".
- Si publican una versión nueva, se reemplaza el archivo y el test avisa si algo dejó de ser compatible.

### 4. Tiempos, reintentos y fallas

| Situación | Qué hacemos |
| --- | --- |
| Tiempo de espera | 5 s por intento, lo que ellos recomiendan. No afecta al cliente: es en segundo plano |
| Reintentos | Cada 10 s, sin límite, siempre con la **misma** `Idempotency-Key` (`pedido-{id}`): repetir es seguro y nunca hay dos facturas ni dos mails |
| Circuit breaker | Uno propio (`proveedor-comprobantes`): si en 10 llamadas falla la mitad, no los llamamos durante 10 s y después probamos con 3 ([ADR-010](ADR-010-resiliencia.md)) |
| Rechazo (4xx de negocio) | No se reintenta; queda el motivo para revisarlo |
| Timeout | **No es un fracaso:** no sabemos si la emitieron. Queda pendiente y el reintento con la misma clave devuelve la que ya estaba |
| Cambio incompatible | Lo detecta el test de contrato; no se sube hasta adaptar el adaptador |

### 5. Qué ve el usuario

- **Nunca perdemos ni frenamos el pedido** por una falla del proveedor: se confirma igual.
- En el detalle del pedido, la factura figura `PENDIENTE` mientras no se emita, y `EMITIDO` con su número cuando sale.
- Si la rechazan, figura `RECHAZADO`; el pedido sigue siendo válido.

## Alternativas que pensamos

| Alternativa | A favor | En contra | Qué decidimos |
| --- | --- | --- | --- |
| **Pedir la factura dentro de la confirmación** | El cliente la tiene "al confirmar" | Sus 5 s no entran en nuestros 3 s, y su caída nos frenaría la compra (lo contrario de lo que piden) | La descartamos: segundo plano |
| **Llamarlo desde el frontend** | Menos código en el backend | Expone su API key; no controlamos errores ni reintentos | La descartamos |
| **Usar sus tipos en nuestro dominio** | Menos traducción | Cualquier cambio suyo nos rompe las reglas | La descartamos: adaptador |
| **Reintentar dentro de un mismo intento (2 o 3 veces seguidas)** | Lo que sugiere su guía | En segundo plano no hace falta: cada vuelta es un reintento, con la misma clave | Reintento por vueltas |
| **Probar solo a mano contra su servicio** | Rápido | No detecta cambios incompatibles a tiempo | La descartamos: test de contrato contra su mock |

## Consecuencias

- El cliente nunca espera ni pierde un pedido por el proveedor; la factura puede llegar unos segundos o minutos después.
- Hay que mantener la copia de su contrato al día y pedir la API key una sola vez cuando publiquen su URL.
- Una factura rechazada no se reintenta sola: queda registrada para revisarla.
- Mientras no publiquen su URL, todo el flujo se prueba contra su mock.

## Cómo lo validamos

- **Test de contrato** contra su mock, en cada corrida de `testdata/compose.yml` (y en GitHub Actions, en cada pull request).
- **Tests del servicio y del adaptador**: misma clave en cada reintento, rechazo sin reintento, timeout como pendiente, circuit breaker.
- **Jornada de fallas (hecha, 2026-11-10):** con el proveedor caído, el pedido se confirmó igual en 0,1 s, la factura quedó pendiente y salió sola 7 s después de que volvió. Detalle en el [POSTMORTEM](../../POSTMORTEM.md).

## Referencias

- Enunciado, sección 4: integración entre grupos.
- Grupo 4: `GUIA-INTEGRACION.md` y `openapi.yaml` 1.1.0 de su repositorio.
