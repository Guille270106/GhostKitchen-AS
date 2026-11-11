# POSTMORTEM — Jornada de fallas controladas

- **Fecha:** 2026-11-10
- **Responsable:** Lucía Rucci
- **Qué se probó:** apagar a propósito, de a una, las dependencias que pueden fallar en el flujo de un pedido —Inventario, RabbitMQ, el correo y el grupo proveedor— y comprobar que el sistema se comporta como prometen el [ADR-010](adr/ADR-010-resiliencia.md) (resiliencia), el [ADR-005](adr/ADR-005-comunicacion-entre-servicios.md) (mensajes y outbox) y el [ADR-009](adr/ADR-009-consumo-del-proveedor.md) (proveedor).
- **Resultado:** **el comportamiento fue el esperado en las cuatro fallas.** Ningún pedido se perdió, ningún stock se inventó ni se descontó dos veces, ninguna respuesta tardó más de 3 s y nunca se disparó la alerta de errores 5xx. Quedan tres mejoras pendientes, anotadas al final.
- **Evidencia:** la salida completa, con los tiempos de cada paso, está en [`docs/jornada-de-fallas/salida.txt`](jornada-de-fallas/salida.txt).

## Resumen

Las seis partes que pide el informe, en corto. El detalle de cada falla está más abajo y la salida completa, en [`salida.txt`](jornada-de-fallas/salida.txt).

### Impacto

| Falla | Qué no se pudo hacer | Qué siguió funcionando | Datos perdidos o duplicados |
| --- | --- | --- | --- |
| Inventario caído (dos cortes: 93 s y 10 s) | Agregar platos al carrito: `503` en 1,6 s | Confirmar (`202 "Estamos confirmando tu pedido"`), cancelar, navegar y ver los pedidos | Ninguno: el stock se descontó una sola vez (20 → 18) y la devolución se aplicó al volver |
| RabbitMQ caído (89 s) | Nada para el usuario: los mails llegaron unos 2 min tarde | Confirmar y aceptar pedidos | Ninguno: los 2 avisos esperaron en el outbox |
| Correo caído (un corte de 22 s y otro de 3 min) | Los mails se demoraron, y uno quedó en la DLQ sin enviar | Todo el flujo de pedidos | Ninguno se perdió: el aviso que no se pudo mandar quedó guardado en la DLQ |
| Proveedor caído (17 s) | La factura quedó pendiente | Confirmar pedidos sin esperar al proveedor | Ninguno: la factura salió una sola vez |

### Línea temporal

Minutos desde que empezó la jornada (10/11/2026; duró 8 min 39 s).

| Momento | Qué pasó |
| --- | --- |
| 00:11 | Todo prendido; una cocina con un plato (receta de 2 unidades, stock 20) y un cliente |
| 00:13 | **Se apaga Inventario** |
| 00:15 | Agregar al carrito responde `503` en 1,6 s |
| 00:17 | Confirmar responde `202 "Estamos confirmando tu pedido"` en 2,1 s |
| 01:44 | Se dispara la alerta `ServicioCaido(inventario)` |
| 01:46 | Vuelve Inventario |
| 01:59 | La confirmación se completa sola; el stock pasa de 20 a 18 |
| 02:01 | Se apaga Inventario otra vez; cancelar responde `200` en 27 ms y la devolución queda pendiente |
| 02:11 | Vuelve Inventario; a las 02:14 el stock vuelve a 20 |
| 02:17 | **Se apaga RabbitMQ**; confirmar y aceptar siguen andando; 2 avisos esperan en el outbox |
| 03:44 | Se dispara la alerta `ServicioCaido(rabbitmq)` |
| 03:46 | Vuelve RabbitMQ; a las 04:21 el outbox está vacío y a las 04:24 llegan los 2 mails |
| 04:26 | **Se apaga el correo** (corte corto); la cocina avanza el pedido igual |
| 04:48 | Vuelve el correo; a las 05:12 llega el mail pendiente por el reintento |
| 05:14 | **Se apaga el correo** (corte largo); la cocina marca el pedido `LISTO` |
| 08:12 | Se agotan los reintentos: el aviso pasa a la DLQ; la alerta `MensajesEnDLQ` queda `pending` |
| 08:13 | Vuelve el correo |
| 08:15 | **Se apaga el proveedor**; confirmar responde `201` en 0,1 s y la factura queda pendiente |
| 08:32 | Vuelve el proveedor; a las 08:39 sale la factura `B-0001-00000042` |
| 08:39 | Fin: todo responde y no se disparó la alerta de errores 5xx |

### Detección

| Falla | Cómo nos enteramos | Cuánto tardó |
| --- | --- | --- |
| Inventario, RabbitMQ | Alerta `ServicioCaido` (Prometheus) y el panel "Servicios arriba" de Grafana en rojo | 91 s y 82 s: la alerta espera 1 minuto caído, para no avisar por un reinicio, más lo que tarda Prometheus en volver a mirar |
| Correo | Panel "Mails por resultado" (aparecen `reintento` y después `dlq`), panel "Mensajes en las DLQ" y alerta `MensajesEnDLQ` | El panel, desde el primer reintento (unos 5 s). La alerta se dispara cuando el aviso lleva 5 minutos en la DLQ |
| Proveedor | **No hay alerta.** Solo se ve en el detalle del pedido, con la factura `PENDIENTE` | — (ver la mejora 5) |

### Respuesta

No hizo falta ninguna intervención manual: en los cuatro casos el sistema se recuperó solo al volver la pieza caída. Lo único manual fue prenderla de nuevo, que es parte de la prueba.

- **Inventario:** la confirmación quedó `EN_CURSO`. Pedidos la retomó cada 10 s con la misma `Idempotency-Key` hasta completarla, y mandó la devolución pendiente.
- **RabbitMQ:** Pedidos guardó los avisos en el outbox y los publicó en orden al reconectarse.
- **Correo:** Notificaciones reintentó con espera creciente (5 s, 30 s y 2 min) y, al agotar los intentos, mandó el aviso a la DLQ.
- **Proveedor:** la factura quedó pendiente y Pedidos la volvió a pedir cada 10 s, siempre con la misma clave, hasta emitirla.

### Causa

Las cuatro caídas fueron **provocadas** con `docker compose stop`, una por vez, para comprobar los mecanismos del [ADR-010](adr/ADR-010-resiliencia.md). Lo que interesa es por qué el sistema se comportó bien o mal en cada caso:

- **Por qué no se perdió ni se duplicó nada:** cada efecto (descontar, devolver, avisar, facturar) se anota en la misma transacción que el cambio que lo provoca, y se reintenta con la misma clave hasta confirmarse.
- **Por qué un aviso quedó sin enviar:** la DLQ guarda el mensaje, pero nada lo vuelve a mandar solo cuando el correo vuelve (mejora 1).
- **Por qué la detección tardó más de un minuto:** la alerta espera 1 minuto a propósito (mejora 2).
- **Por qué el proveedor caído no avisa:** Prometheus no mira al proveedor ni cuenta las facturas pendientes (mejora 5).

### Acciones de mejora

Están en [Qué aprendimos y qué mejorar](#qué-aprendimos-y-qué-mejorar), con el motivo de cada una. Quedan pendientes la 1 (reprocesar la DLQ), la 2 (bajar la espera de la alerta) y la 5 (alerta de facturas pendientes). La 3 ya está corregida.

## Cómo se hizo

Es una prueba **controlada**: se rompe una pieza a la vez, en un entorno local igual al de desarrollo, se mira qué ve el usuario y se mide cuánto tarda en recuperarse cuando la pieza vuelve. No es una prueba de carga (eso es el [ADR-013](adr/ADR-013-capacidad-y-costos.md)).

| | |
| --- | --- |
| Entorno | Todo el sistema en Docker Compose (`docker compose up -d --build`), desde `develop`. Pedidos usa el mock del Grupo 4 durante toda la jornada, aunque el `.env` apunte a su servicio real, para no generarle facturas de prueba |
| Cómo se rompe | `docker compose stop <servicio>` y, al terminar cada caso, `docker compose start <servicio>` |
| Quién hace de usuario | El script [`scripts/fallas/jornada.mjs`](../scripts/fallas/jornada.mjs): prepara una cocina con un plato (receta de 2 unidades y stock 20) y un cliente, y usa la API por el gateway, igual que el frontend |
| Qué se mira | El código y el tiempo de cada respuesta, el stock en Inventario, el outbox de Pedidos (MySQL), las colas de RabbitMQ, los mails en Mailpit y las alertas de Prometheus. En Grafana se ven en vivo los paneles "Servicios arriba", "Mensajes esperando en cada cola", "Mensajes en las DLQ" y "Alertas activas" |

**Para repetirla** (por ejemplo, en la defensa): con todo levantado, `node scripts/fallas/jornada.mjs`. Tarda unos 9 minutos, deja todo prendido al terminar y sale con código 1 si algo no se comportó como se espera.

## 1. Inventario caído

**Qué esperábamos (ADR-010):** no se reserva ni se confirma "a ciegas". Si no sabemos si Inventario descontó, no se dice que el pedido falló: se responde "Estamos confirmando tu pedido" y se termina solo cuando vuelve. Ninguna respuesta pasa de 3 s.

| Paso | Qué vio el usuario | Tiempo |
| --- | --- | --- |
| Agregar un plato al carrito | `503 SERVICIO_NO_DISPONIBLE`. No se inventa una reserva | 1,6 s |
| Confirmar el pedido (un carrito sin cambios pendientes) | `202 "Estamos confirmando tu pedido"`: la confirmación queda `EN_CURSO` | 2,1 s |
| Mientras Inventario sigue caído | La confirmación sigue `EN_CURSO`: nunca se dice que falló | — |
| Alerta `ServicioCaido(inventario)` | Disparada en Prometheus y visible en Grafana ("Alertas activas", severidad alta; "Servicios arriba" muestra Inventario en rojo) | 91 s después de la caída |
| **Inventario vuelve** | La confirmación se completa **sola** y el pedido queda `RECIBIDO`. El stock pasa de 20 a 18 (se descontó una sola vez) | 13 s |
| Cancelar con Inventario caído | `200 CANCELADO`. La devolución de ingredientes queda anotada como `PENDIENTE` | 0,03 s |
| **Inventario vuelve** | Los ingredientes vuelven al stock (20) | 3 s |

**Cambio posterior a esta jornada.** El 10/11 a la noche entró la corrección de consistencia entre el carrito y las reservas (Pedidos anota cada cambio del carrito como incierto antes de llamar a Inventario; ver el [README de Pedidos](../services/pedidos/README.md)). Desde entonces, si un "agregar" falla con Inventario caído, ese cambio queda sin resolver y **confirmar responde `409 CARRITO_MODIFICADO`** hasta que Inventario vuelve y Pedidos lo aclara solo (cada 10 s). Es a propósito: confirmar consumiría una reserva que quizás no coincide con el carrito. Por eso el script ahora confirma con un carrito sin cambios pendientes, y prueba el "agregar" fallido al final del caso, en la segunda caída: comprueba el `503`, el `409` y que el cambio se aclare solo sin dejar el plato en el carrito.

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

- **No se disparó `MuchosErrores5xx`.** Hubo `503` esperados (agregar al carrito con Inventario caído), pero son pocos y cortos para la alerta, que pide más de 5 % de errores durante 5 minutos. Ninguna respuesta fue un error inesperado: las fallas se tradujeron en respuestas claras (`202`, `503` con `Retry-After`).
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
| 3 | **Una corrida cortada a la mitad dejó servicios apagados**: la primera prueba se interrumpió de un día para el otro y la siguiente encontró el correo apagado | Una prueba de fallas tiene que dejar todo como lo encontró, aunque se corte | **Ya corregido:** el script prende todo y espera a que esté sano antes de empezar. Si se corta, `docker compose up -d` lo deja como antes (también vuelve a poner a Pedidos con el proveedor del `.env`) |
| 4 | **RabbitMQ tarda ~30 s en arrancar** y recién ahí salen los avisos acumulados | No afecta a los pedidos (para eso está el outbox), solo al momento en que llega el mail | Ninguna por ahora: es el tiempo normal de arranque del broker |
| 5 | **El proveedor caído no se ve en el monitoreo.** Ninguna alerta ni panel muestra que las facturas quedaron pendientes: solo se ve en el detalle de cada pedido | Si el Grupo 4 se cae un día entero, nadie se entera hasta que un cliente reclame la factura | Una métrica de Pedidos con las facturas pendientes, un panel en Grafana y una alerta si hay pendientes por más de 10 minutos |

## Qué no se probó en esta jornada

- **Usuarios caído:** por diseño (ADR-010) se rechaza el acceso (falla cerrada); no aporta otro comportamiento de recuperación.
- **Solr, Catálogo y MongoDB caídos:** ya se probaron en la Entrega 2 (Búsqueda responde `503` y nunca una lista vacía) y en las pruebas de carga de Catálogo con una copia caída ([ADR-012](adr/ADR-012-balanceo-de-carga.md)).
- **Dependencias lentas** (no caídas): Inventario lento está cubierto por los tests de Pedidos (responde "Estamos confirmando" antes de los 3 s con un Inventario que tarda 1,5 s).
