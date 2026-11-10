# POSTMORTEM — Jornada de fallas controladas

- **Fecha:** 2026-11-10
- **Responsable:** Lucía Rucci (L20)
- **Qué se probó:** apagar a propósito, de a una, las dependencias que pueden fallar en el flujo de un pedido —Inventario, RabbitMQ, el correo y el grupo proveedor— y comprobar que el sistema se comporta como prometen el [ADR-010](docs/adr/ADR-010-resiliencia.md) (resiliencia), el [ADR-005](docs/adr/ADR-005-comunicacion-entre-servicios.md) (mensajes y outbox) y el [ADR-009](docs/adr/ADR-009-consumo-del-proveedor.md) (proveedor).
- **Resultado:** **el comportamiento fue el esperado en las cuatro fallas.** Ningún pedido se perdió, ningún stock se inventó ni se descontó dos veces, ninguna respuesta tardó más de 3 s y nunca se disparó la alerta de errores 5xx. Quedan tres mejoras anotadas al final.
- **Evidencia:** la salida completa, con los tiempos de cada paso, está en [`docs/jornada-de-fallas/salida.txt`](docs/jornada-de-fallas/salida.txt).

## Cómo se hizo

Es una prueba **controlada** (Clase 7): se rompe una pieza a la vez, en un entorno local igual al de desarrollo, se mira qué ve el usuario y se mide cuánto tarda en recuperarse cuando la pieza vuelve. No es una prueba de carga (eso es el [ADR-013](docs/adr/ADR-013-capacidad-y-costos.md)).

| | |
| --- | --- |
| Entorno | Todo el sistema en Docker Compose, con Notificaciones (`docker compose --profile notificaciones up -d --build`), desde `develop` |
| Cómo se rompe | `docker compose stop <servicio>` y, al terminar cada caso, `docker compose start <servicio>` |
| Quién hace de usuario | El script [`scripts/fallas/jornada.mjs`](scripts/fallas/jornada.mjs): prepara una cocina con un plato (receta de 2 unidades y stock 20) y un cliente, y usa la API por el gateway, igual que el frontend |
| Qué se mira | El código y el tiempo de cada respuesta, el stock en Inventario, el outbox de Pedidos (MySQL), las colas de RabbitMQ, los mails en Mailpit y las alertas de Prometheus. En Grafana se ven en vivo los paneles "Servicios arriba", "Mensajes esperando en cada cola", "Mensajes en las DLQ" y "Alertas activas" |

**Para repetirla** (por ejemplo, en la defensa): con todo levantado, `node scripts/fallas/jornada.mjs`. Tarda unos 9 minutos, deja todo prendido al terminar y sale con código 1 si algo no se comportó como se espera.

## 1. Inventario caído

**Qué esperábamos (ADR-010):** no se reserva ni se confirma "a ciegas". Si no sabemos si Inventario descontó, no se dice que el pedido falló: se responde "Estamos confirmando tu pedido" y se termina solo cuando vuelve. Ninguna respuesta pasa de 3 s.

| Paso | Qué vio el usuario | Tiempo |
| --- | --- | --- |
| Agregar un plato al carrito | `503 SERVICIO_NO_DISPONIBLE`. No se inventa una reserva | 1,6 s |
| Confirmar el pedido | `202 "Estamos confirmando tu pedido"`: la confirmación queda `EN_CURSO` | 2,1 s |
| Mientras Inventario sigue caído | La confirmación sigue `EN_CURSO`: nunca se dice que falló | — |
| Alerta `ServicioCaido(inventario)` | Disparada en Prometheus y visible en Grafana ("Alertas activas", severidad alta; "Servicios arriba" muestra Inventario en rojo) | 91 s después de la caída |
| **Inventario vuelve** | La confirmación se completa **sola** y el pedido queda `RECIBIDO`. El stock pasa de 20 a 18 (se descontó una sola vez) | 13 s |
| Cancelar con Inventario caído | `200 CANCELADO`. La devolución de ingredientes queda anotada como `PENDIENTE` | 0,03 s |
| **Inventario vuelve** | Los ingredientes vuelven al stock (20) | 3 s |

**Por qué funcionó:** la confirmación es una saga con cada paso guardado; los reintentos usan siempre la misma `Idempotency-Key` (Inventario no descuenta dos veces); un proceso de Pedidos retoma las confirmaciones pendientes cada 10 s; y la devolución de un pedido cancelado se anota en la misma transacción que el cambio de estado y se manda en segundo plano. Los tiempos de 1,6 s y 2,1 s salen de los límites por llamada (800 ms por intento) y del plazo de 2 s para descontar, dentro de los 3 s.

## 2. RabbitMQ caído

**Qué esperábamos (ADR-005):** los pedidos siguen funcionando. Los avisos de cambio de estado se guardan en el outbox junto con el cambio y salen cuando RabbitMQ vuelve; el mail llega más tarde, pero llega.

| Paso | Qué vio el usuario | Tiempo |
| --- | --- | --- |
| Confirmar un pedido | `201 RECIBIDO` | 0,1 s |
| La cocina lo acepta | `200 ACEPTADO` | 0,03 s |
| Los avisos | Quedan **2 sin publicar** en el outbox de Pedidos; todavía no llega ningún mail | — |
| Alerta `ServicioCaido(rabbitmq)` | Disparada | 82 s después |
| **RabbitMQ vuelve** | El outbox se vacía solo (Pedidos se reconecta y publica en orden) | 36 s (casi todo es lo que tarda RabbitMQ en arrancar) |
| Los mails | Llegan los 2 mails del pedido, `RECIBIDO` y `ACEPTADO` | 3 s después de publicarse |

**Por qué funcionó:** el outbox guarda el aviso en la misma transacción que el cambio, el publicador se reconecta solo y espera la confirmación del broker, y Notificaciones también se reconecta a su cola.

## 3. Correo caído (Mailpit)

**Qué esperábamos (ADR-005, ADR-010):** los pedidos no se enteran. Notificaciones reintenta el envío esperando cada vez más (5 s, 30 s y 2 min); si se agotan los intentos, el aviso va a la DLQ para revisarlo, sin perderse.

| Paso | Qué vio el usuario | Tiempo |
| --- | --- | --- |
| La cocina avanza el pedido con el correo caído | `200 EN_PREPARACION` | 0,07 s |
| **Corte corto (20 s)**: el correo vuelve | El mail de `EN_PREPARACION` llega solo, por el reintento | 24 s después de volver el correo |
| **Corte largo**: la cocina lo marca `LISTO` con el correo caído | `200 LISTO` | — |
| Se agotan los reintentos | El aviso pasa a `notificaciones.mailer.dlq`. No se pierde: queda para revisarlo | 178 s |
| Alerta `MensajesEnDLQ` | Empieza a aparecer (estado `pending`). Se dispara cuando el mensaje lleva 5 minutos en la DLQ | — |

**Por qué funcionó:** Notificaciones confirma el mensaje a RabbitMQ recién después de enviar el mail; para una falla pasajera lo reencola con espera creciente, y la cola tiene su DLQ.

## 4. Proveedor caído (Grupo 4, factura)

**Qué esperábamos (ADR-009):** la factura no es un paso esencial. El pedido se confirma igual, la factura queda pendiente y sale sola cuando el proveedor vuelve, sin facturas duplicadas.

| Paso | Qué vio el usuario | Tiempo |
| --- | --- | --- |
| Confirmar un pedido con el proveedor caído | `201 RECIBIDO`. El pedido no espera al proveedor | 0,1 s |
| El detalle del pedido | La factura figura `PENDIENTE` | — |
| **El proveedor vuelve** | La factura sale sola: `EMITIDO`, número `B-0001-00000042` | 7 s |

**Por qué funcionó:** la factura se anota pendiente en la misma transacción que crea el pedido y se pide en segundo plano cada 10 s, siempre con la misma clave `pedido-{id}`. Se probó contra el mock del Grupo 4, porque todavía no publicaron su URL.

## Al final de la jornada

- **No se disparó `MuchosErrores5xx`.** Las fallas se tradujeron en respuestas claras (`202`, `503` con `Retry-After`), no en errores inesperados.
- Todo siguió respondiendo y quedó prendido.

## Qué salió bien

- **Las fallas quedaron en su dependencia.** Con Inventario, RabbitMQ, el correo o el proveedor caídos, el resto del sistema siguió funcionando y respondiendo en menos de 3 s.
- **Honestidad ante la duda.** Cuando no se sabe si algo pasó (Inventario no contestó), el usuario ve "Estamos confirmando tu pedido", nunca un "falló" falso ni un stock inventado.
- **Recuperación sola.** En los cuatro casos el sistema se puso al día sin intervención: confirmaciones, devoluciones, avisos, mails y facturas.
- **Sin duplicados.** Las claves de idempotencia hicieron que los reintentos no descontaran dos veces ni emitieran dos facturas.
- **Se ve desde afuera.** Grafana mostró la pieza caída y la alerta, que es lo que necesitaría quien opera el sistema.

## Qué aprendimos y qué mejorar

| # | Hallazgo | Por qué importa | Mejora propuesta |
| --- | --- | --- | --- |
| 1 | **Un mail que cae en la DLQ no se vuelve a mandar solo.** Queda guardado, pero el cliente no recibe ese aviso hasta que alguien lo reprocese | Un corte de correo de más de ~3 min deja avisos sin enviar | Un comando para reprocesar la DLQ (mover los mensajes de `notificaciones.mailer.dlq` a la cola principal) y anotarlo como paso de la alerta `MensajesEnDLQ` |
| 2 | **La alerta `ServicioCaido` tarda ~1 min 30 s** en dispararse: espera 1 minuto caído (para no avisar por un reinicio) más lo que tarda Prometheus en volver a mirar | Es un buen equilibrio para un trabajo práctico, pero en producción convendría avisar antes de un minuto y medio | Evaluar bajar la espera a 30 s o mirar más seguido; medirlo contra los reinicios normales |
| 3 | **Una corrida cortada a la mitad dejó servicios apagados**: la primera prueba se interrumpió de un día para el otro y la siguiente encontró el correo apagado | Una prueba de fallas tiene que dejar todo como lo encontró, aunque se corte | **Ya corregido:** el script prende todo y espera a que esté sano antes de empezar. Si se corta, `docker compose --profile notificaciones up -d` lo deja como antes |
| 4 | **RabbitMQ tarda ~30 s en arrancar** y recién ahí salen los avisos acumulados | No afecta a los pedidos (para eso está el outbox), solo al momento en que llega el mail | Ninguna por ahora: es el tiempo normal de arranque del broker |

## Qué no se probó en esta jornada

- **Usuarios caído:** por diseño (ADR-010) se rechaza el acceso (falla cerrada); no aporta otro comportamiento de recuperación.
- **Solr, Catálogo y MongoDB caídos:** ya se probaron en la Entrega 2 (Búsqueda responde `503` y nunca una lista vacía) y en las pruebas de carga de Catálogo con una copia caída ([ADR-012](docs/adr/ADR-012-balanceo-de-carga.md)).
- **Dependencias lentas** (no caídas): Inventario lento está cubierto por los tests de Pedidos (responde "Estamos confirmando" antes de los 3 s con un Inventario que tarda 1,5 s).
