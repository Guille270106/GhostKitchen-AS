# ADR-005 — Cómo se comunican los servicios

- **Estado:** Primera versión (se valida en la Entrega 2)
- **Fecha:** 2026-10-09
- **Responsable:** Lucía Rucci
- **Decisión del enunciado:** D5 (cuándo usar llamadas directas y cuándo mensajes, tiempos de espera, reintentos, eventos e idempotencia)
- **Relacionado con:** [ADR-001](ADR-001-limites-de-los-servicios.md) · [ADR-003](ADR-003-persistencia.md) · [ADR-008](ADR-008-contrato-propio.md) · [ARCHITECTURE](../ARCHITECTURE.md) (secciones 5, 6 y 10)

## Contexto

Con los servicios separados ([ADR-001](ADR-001-limites-de-los-servicios.md)), varias acciones pasan por más de un servicio:

- **Agregar al carrito:** Pedidos necesita los datos del plato (Catálogo) y reservar los ingredientes (Inventario) **antes** de contestarle al cliente.
- **Confirmar:** Pedidos le pide a Inventario que descuente el stock, y no puede descontar dos veces aunque se reintente.
- **Cancelar o rechazar:** el stock tiene que volver a Inventario.
- **Mails, búsqueda y copias de datos:** pueden hacerse un rato después, pero no se pueden perder.
- **Ninguna pantalla puede esperar más de 3 segundos**, y si se cae el mail o la búsqueda se tiene que poder seguir comprando.

## Decisión

### 1. Cuándo usamos cada forma

La regla sale de la Clase 4: síncrono cuando el resultado define la respuesta; asíncrono cuando el efecto puede terminar después.

| **Llamada directa (HTTP)** cuando… | **Mensaje (RabbitMQ)** cuando… |
| --- | --- |
| Necesitamos la respuesta para contestarle al usuario | La tarea puede hacerse un rato después |
| El usuario tiene que enterarse ya si algo falla (por ejemplo, "sin stock") | A varios servicios les interesa lo mismo |
| Ejemplos: ver el plato, reservar, descontar, devolver | Ejemplos: el mail, actualizar la búsqueda, avisar que venció una reserva |

### 2. Llamadas directas

- Usamos **HTTP con JSON**. Los errores tienen un código fijo (por ejemplo `SIN_STOCK` o `RESERVA_VENCIDA`) para que el que llama sepa qué pasó.
- **Tiempo total: 3 segundos.** Cada servicio usa el tiempo que **queda**, no empieza a contar de nuevo (`context.WithTimeout` en Go, Clase 7). Si no queda tiempo, no hace la llamada.

| Quién llama → a quién | Para qué | Espera máxima por intento | Reintentos máximos dentro del tiempo total |
| --- | --- | --- | --- |
| Gateway → Usuarios | Verificar el login | 300 ms | No |
| Usuarios → Catálogo | Ver la cocina al crear una cuenta Cocina | 800 ms | 1 |
| Pedidos → Catálogo | Ver el plato y la cocina | 500 ms | 1 |
| Pedidos → Inventario | Reservar, cambiar o liberar | 800 ms | 1, con la misma clave |
| Pedidos → Inventario | Descontar o devolver stock | 800 ms | 1, con la misma clave; después continúa la recuperación |
| Pedidos → Inventario | Preguntar si algo se hizo | Hasta 500 ms, si queda tiempo | Solo dentro del presupuesto restante; después consulta el proceso de recuperación |
| Pedidos → grupo proveedor | Depende de lo que nos asignen | Lo que quede del total de 3 s | Solo si es seguro repetir y queda tiempo |
| Búsqueda → Catálogo e Inventario | Reconstruir la búsqueda en segundo plano | 5 s | 3; no pertenece a una solicitud de usuario |

- **Esperar cada vez más entre reintentos** (100 ms, 200 ms, 400 ms…), con un poco de azar para que no reintenten todos juntos.
- Los máximos no se suman como tiempos garantizados: autenticación, llamadas, esperas entre intentos y trabajo local comparten el mismo plazo de 3 s. Si el siguiente intento no entra en ese plazo, no se inicia. Una confirmación con resultado incierto continúa con su paso persistido en segundo plano y el cliente ve "Estamos confirmando tu pedido".
- **Reintenta solo uno:** el servicio que hace la llamada. Ni el gateway ni el frontend reintentan, para no multiplicar los intentos (Clase 7).
- **No se reintenta** cuando el error es del negocio (sin stock, reserva vencida, datos mal cargados) ni cuando falta permiso: repetir no lo arregla.
- **Circuit breaker:** si la mitad de las últimas llamadas a un servicio fallaron, dejamos de llamarlo 10 segundos y respondemos rápido con un error. Después probamos de nuevo con pocas llamadas. Hay uno **por operación** (reservar, descontar, consultar, proveedor), porque cada una falla distinto (Clase 7).
- **Librerías:** la cátedra pidió usar librerías para el manejo de errores. Proponemos `context` de Go para los tiempos, `sony/gobreaker` para el circuit breaker y `cenkalti/backoff` para los reintentos con espera creciente. No las vimos en clase: se confirman en el ADR-010.
- **Si una llamada no contesta a tiempo, no sabemos si se hizo o no** (Clase 7: un timeout no es un fracaso). Por eso, antes de repetir o deshacer, Pedidos **pregunta si se hizo**. Al cliente le mostramos "Estamos confirmando tu pedido", nunca un rechazo que no es cierto.
- **No hay llamadas en círculo:** Catálogo e Inventario nunca llaman a Pedidos, e Inventario no llama a Catálogo.

### 3. Cómo evitamos hacer algo dos veces (idempotencia)

Todas las llamadas que **cambian algo** en Inventario mandan una clave en el header `Idempotency-Key`:

| Acción | Clave |
| --- | --- |
| Reservar o cambiar cantidades | carrito + plato + número de cambio |
| Descontar | ID de la confirmación |
| Liberar | ID de la reserva + "liberar" |
| Devolver | ID del pedido + "devolver" |

Inventario guarda la clave junto con el cambio. Si llega otra vez la misma clave:
- con los mismos datos, devuelve la misma respuesta **sin hacer nada de nuevo**;
- con otros datos, da error, porque alguien está reusando la clave mal;
- si la primera todavía se está procesando, avisa "en proceso".

El frontend también manda una clave al confirmar el pedido, así un doble clic no crea dos pedidos.

### 4. Mensajes por RabbitMQ

Cada servicio que escucha tiene **su propia cola**. Si un servicio tiene varias copias, se reparten los mensajes de su cola.

| Cola | Qué mensajes recibe | Para qué |
| --- | --- | --- |
| `busqueda.indexador` | `cocina.*`, `plato.*`, `disponibilidad.cambiada` | Mantener la búsqueda al día |
| `inventario.referencias` | `cocina.*`, `plato.*` | Saber la localidad de cada cocina y de qué cocina es cada plato |
| `pedidos.reservas` | `reserva.vencida` | Marcar el carrito como vencido |
| `notificaciones.mailer` | `pedido.estado_cambiado` | Mandar el mail |

Todos los mensajes tienen la misma forma:

```json
{
  "evento_id": "0b8f2d4e-…",
  "tipo": "pedido.estado_cambiado",
  "version": 1,
  "ocurrido_en": "2026-10-09T21:16:04Z",
  "correlation_id": "req-713a",
  "agregado_id": "ped-1042",
  "secuencia": 4,
  "datos": {
    "estado_anterior": "ACEPTADO",
    "estado_nuevo": "EN_PREPARACION",
    "cliente_email": "cliente@ejemplo.com"
  }
}
```

- `evento_id` sirve para no procesar dos veces el mismo mensaje.
- `secuencia` sirve para ignorar un mensaje viejo que llegó tarde.
- `correlation_id` sirve para seguir una misma acción en los logs de todos los servicios.

**Para no perder mensajes (outbox, Clases 2 y 4):** Catálogo, Inventario y Pedidos guardan el cambio y el mensaje **en la misma operación de su base**. Después, otro proceso lo manda a RabbitMQ y lo marca como enviado. Si RabbitMQ está caído, el mensaje queda guardado y sale cuando vuelve.

**Un mensaje puede llegar dos veces.** Por eso cada servicio que escucha:
1. anota el `evento_id` como procesado junto con el cambio que hace;
2. si ya estaba anotado, ignora el mensaje;
3. le confirma a RabbitMQ que lo procesó (ACK) **recién al final**.

En MySQL/MongoDB, el registro de deduplicación y el efecto se guardan en una transacción local. Búsqueda usa actualizaciones idempotentes y versiones persistidas en Solr, sin afirmar una transacción entre Solr y una tabla de mensajes. Mantiene una versión de Catálogo y otra de Inventario por documento: cada fuente modifica solamente sus propios campos. Los mensajes de distintos agregados no se comparan con una secuencia global.

El outbox publica mensajes persistentes en colas durables y espera la confirmación del broker antes de marcar un evento como enviado. Una caída entre esa confirmación y el marcado puede duplicar la publicación; el consumidor debe tolerarla. Recuperar eventos y deduplicación requiere almacenamiento persistente, no memoria del proceso.

**Si un mensaje falla:**
- si es un error pasajero, se reintenta esperando cada vez más (5 s, 30 s, 2 min);
- si sigue fallando, o el mensaje viene roto, va a la cola de errores (**DLQ**), por ejemplo `notificaciones.dlq`;
- si hay mensajes en la DLQ por más de 5 minutos, salta una alerta para revisarlos.

**El mail es un caso especial**, porque no se puede deshacer. Notificaciones anota el mensaje como PENDIENTE, manda el mail, lo marca ENVIADO y recién ahí hace el ACK. Si se cae justo después de mandarlo, el mail puede llegar repetido. Lo aceptamos porque es mejor que perder un mail.

Como un mail mandado por SMTP no se puede deshacer ni guardar en la misma transacción que la marca de ENVIADO, no se puede garantizar "exactamente una vez" (Clase 4). **El equipo acordó** priorizar que no se pierda ningún mail y aceptar ese caso puntual de repetido; quedó así en RN17 y HU13 del [SPEC](../../SPEC.md).

### 5. Comunicación con afuera

- **Frontend → gateway:** HTTP con el token del login.
- **Otro grupo → gateway:** por `/public/v1` con una API key ([ADR-008](ADR-008-contrato-propio.md)).
- **Pedidos → grupo proveedor:** directo desde Pedidos. Nunca desde el frontend ni por nuestro gateway (lo pide el enunciado). El detalle va en el ADR-009.

## Alternativas que pensamos

| Alternativa | A favor | En contra | Qué decidimos |
| --- | --- | --- | --- |
| **Todo con llamadas directas** | Más fácil de seguir | Si se cae el mail, no se puede comprar | La descartamos |
| **Reservar con mensajes** | Pedidos no depende de Inventario | El cliente necesita saber **en el momento** si hay stock | La descartamos |
| **Devolver el stock con un mensaje** | Menos dependencia | Los pasos quedan repartidos y es difícil ver en cuál falló | La descartamos: Pedidos coordina la devolución como un paso más |
| **Que Inventario le pregunte a Catálogo** | El dato siempre está al día | El servicio más importante dependería de otro | La descartamos: Inventario recibe una copia por mensajes |
| **gRPC** | Más rápido | Más herramientas nuevas; no lo necesitamos | La descartamos |
| **Kafka** | Guarda todos los mensajes | Más difícil de manejar; RabbitMQ es el que vimos en la materia | La descartamos |
| **Que cada servicio reaccione solo, sin coordinador** | No hay un servicio central | Es muy difícil seguir el flujo de la confirmación | La descartamos: **coordina Pedidos** (ADR-004) |

## Consecuencias

**Lo bueno**

- El cliente sabe en el momento si hay stock, y lo secundario (mails, búsqueda) no frena la compra.
- Ningún reintento puede duplicar una reserva, un descuento ni un pedido.
- Si mañana queremos que otro servicio reciba los cambios de estado, no hay que tocar Pedidos.

**Lo que aceptamos**

- Algunas cosas tardan unos segundos en verse: la búsqueda (máximo 5 s), el carrito vencido y las copias de Inventario.
- Hay más piezas para mantener: el outbox, las colas de reintento y la DLQ.
- El frontend tiene que mostrar estados intermedios, como "estamos confirmando tu pedido".
- Puede llegar algún mail repetido si un servicio se cae en el peor momento.

**Riesgos**

| Riesgo | Qué hacemos |
| --- | --- |
| Que los reintentos se multipliquen | Reintenta uno solo, con tiempo total limitado y circuit breaker |
| Que se llene la DLQ y nadie se entere | Alerta en Grafana |
| Que un mensaje viejo pise a uno nuevo | Usamos `secuencia` y descartamos los viejos |

## Cómo lo vamos a validar

- **Entrega 2:** seguir un "agregar al carrito" de punta a punta con el mismo `correlation_id` en el gateway, Pedidos, Catálogo e Inventario. Y un test que mande dos veces el mismo mensaje de cambio de estado y verifique que llega un solo mail.
- **Entrega final:** si tiramos Inventario, Pedidos tiene que responder rápido "no podemos tomar pedidos ahora". Si tiramos RabbitMQ, no se tiene que perder ningún mensaje.

## Referencias

- Clase 2: saga coordinada (orquestación) o por reacciones (coreografía), outbox, idempotencia.
- Clase 4: síncrono y asíncrono, RabbitMQ, una cola por servicio, ACK después del efecto, reintentos y DLQ.
- Clase 6: el gateway como única entrada.
- Clase 7: tiempo total de la operación, reintentos con espera creciente, claves de idempotencia, circuit breaker, "un timeout no es un fracaso".
