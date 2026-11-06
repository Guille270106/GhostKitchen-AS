# Mensajes entre servicios

Formato común de todos los mensajes que viajan por RabbitMQ. La decisión de cuándo usar mensajes y cómo no perderlos está en el [ADR-005](../../adr/ADR-005-comunicacion-entre-servicios.md); las colas y los bindings, en [`infra/rabbitmq`](../../../infra/rabbitmq/README.md).

## El sobre

Todos los mensajes tienen la misma forma. Lo que cambia según el tipo es `datos`. El esquema formal está en [`sobre.schema.json`](sobre.schema.json).

```json
{
  "evento_id": "0b8f2d4e-6c1a-4f7e-9d2b-3a5c7e9f1b20",
  "tipo": "pedido.estado_cambiado",
  "version": 1,
  "ocurrido_en": "2026-11-06T21:16:04Z",
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

| Campo | Qué es | Para qué sirve |
| --- | --- | --- |
| `evento_id` | UUID, único por mensaje | El consumidor lo anota como procesado y descarta los repetidos |
| `tipo` | Nombre del evento, igual a la clave de ruteo | Saber qué pasó y a qué colas va |
| `version` | Versión del formato de `datos` para ese tipo | Cambiarla solo si un cambio rompe a los consumidores |
| `ocurrido_en` | Fecha y hora UTC, ISO 8601 | Saber cuándo pasó, no cuándo llegó |
| `correlation_id` | El `X-Correlation-Id` de la solicitud que causó el cambio | Seguir la misma acción en los logs de todos los servicios |
| `agregado_id` | Id de la cosa que cambió (cocina, plato, carrito, pedido) | Ordenar los mensajes de una misma cosa |
| `secuencia` | Número que crece con cada cambio de esa cosa | Ignorar un mensaje viejo que llegó tarde |
| `datos` | Lo que cambió, según el tipo | El contenido del evento |

## Cómo se publica

- **Exchange:** `ghostkitchen.events`, de tipo `topic`.
- **Clave de ruteo:** el mismo valor que `tipo`, por ejemplo `plato.actualizado`.
- **Propiedades AMQP:** `content_type: application/json`, `delivery_mode: 2` (persistente) y `message_id` igual a `evento_id`.
- **Desde el outbox:** se guarda el cambio y el mensaje en la misma operación de la base; otro proceso lo publica y lo marca como enviado recién cuando RabbitMQ confirma (ADR-005).

## Cómo se consume

1. Cada servicio lee **solo su cola** (ver la tabla de [`infra/rabbitmq`](../../../infra/rabbitmq/README.md)).
2. Si el `evento_id` ya estaba procesado, confirma (ACK) y no hace nada.
3. Si `secuencia` es menor o igual a la última aplicada para ese `agregado_id`, confirma y lo ignora.
4. Aplica el cambio y anota el `evento_id` en la misma operación.
5. Recién ahí confirma (ACK).
6. Un error pasajero se reintenta esperando cada vez más (5 s, 30 s, 2 min). Un mensaje roto se rechaza sin reencolar y va a la DLQ. Además, la cola manda a la DLQ cualquier mensaje entregado 5 veces sin confirmar.
7. Los campos de `datos` que no conoce se ignoran.

## Los eventos

| Tipo | Lo publica | Lo escuchan (cola) | `agregado_id` | `secuencia` |
| --- | --- | --- | --- | --- |
| `cocina.creada`, `cocina.actualizada`, `cocina.desactivada` | Catálogo | `busqueda.indexador`, `inventario.referencias` | Id de la cocina | `version` de la cocina |
| `plato.creado`, `plato.actualizado`, `plato.desactivado` | Catálogo | `busqueda.indexador`, `inventario.referencias` | Id del plato | `version` del plato |
| `disponibilidad.cambiada` | Inventario | `busqueda.indexador` | Id del plato | Contador de Inventario por plato |
| `reserva.vencida` | Inventario | `pedidos.reservas` | Id del carrito | Contador de la reserva |
| `pedido.estado_cambiado` | Pedidos | `notificaciones.mailer` | Id del pedido | Número de cambio de estado del pedido |

### `datos` de cada evento

**`cocina.*`**: la cocina completa, con el mismo JSON que devuelve `GET /internal/cocinas` de Catálogo (`id`, `nombre`, `localidad`, `direccion`, `ubicacion`, tiempos, `costoEnvio`, `restaurantes`, `activa`, `version`, `actualizadaEn`). Se manda entera para que el consumidor no tenga que volver a pedirla.

**`plato.*`**: el plato completo, con el mismo JSON que `GET /internal/platos` (`id`, `cocinaId`, `restauranteId`, `nombre`, `categoria`, `etiquetas`, `precio`, `moneda`, `activo`, `version`, `actualizadoEn`).

**`disponibilidad.cambiada`**

```json
{ "plato_id": "p5", "cocina_id": "nueva-cordoba", "disponible": false, "tiene_receta": true }
```

**`reserva.vencida`**

```json
{ "reserva_id": "res-381", "carrito_id": "car-77", "cocina_id": "cerro" }
```

**`pedido.estado_cambiado`**

```json
{ "estado_anterior": "ACEPTADO", "estado_nuevo": "EN_PREPARACION", "cliente_email": "cliente@ejemplo.com" }
```

Los estados posibles son los del pedido: `RECIBIDO`, `ACEPTADO`, `EN_PREPARACION`, `LISTO`, `EN_CAMINO`, `ENTREGADO` y `CANCELADO`. El mail del cliente viaja en el mensaje para que Notificaciones no tenga que consultar a Usuarios.

## Cambiar un evento

- **Agregar un campo** a `datos` es compatible: no se cambia `version`.
- **Sacar o cambiar el significado de un campo** rompe a los consumidores: se sube `version` y, mientras haga falta, se publican las dos versiones.
- Un **evento nuevo** se agrega a esta tabla y, si alguien lo escucha, a [`definiciones.json`](../../../infra/rabbitmq/definiciones.json).
