# Pedidos

**Responsable:** Lucía Rucci.

> Qué hace hoy: el **carrito** (con la reserva de Inventario al día y el tiempo que le queda), **confirmar el pedido** como saga (plazo de 3 s, circuit breaker con `sony/gobreaker` y reintentos con `cenkalti/backoff`), los **estados del pedido** con su historial, la **pantalla de la cocina**, **cancelar** devolviendo los ingredientes, los **avisos** por RabbitMQ y la **factura** del grupo proveedor.

Maneja **el pedido de principio a fin**. Puerto interno **8085**. Es uno de los dos servicios donde no se puede equivocar nada.

## Cómo está organizado (hexagonal)

| Pieza | Rol en hexagonal | Archivo |
| --- | --- | --- |
| `PedidosController`, `CarritoController`, `ConfirmacionController` | Adaptador de entrada (Gin) | `internal/adapters/http/controllers.go` |
| `PedidosService`, `CarritoService`, `ConfirmacionService` | Casos de uso (núcleo) | `internal/application/` |
| `Pedido`, `Carrito`, `Confirmacion` y sus reglas | Dominio (núcleo) | `internal/domain/` |
| `PedidosRepo`, `CarritosRepo`, `ConfirmacionesRepo`, `Catalogo`, `Inventario`, `Consumo` | Puertos de salida (interfaces) | `internal/ports/` |
| `PedidosMySQL`, `CarritosMySQL`, `ConfirmacionesMySQL` | Adaptadores de salida a MySQL | `internal/adapters/mysql/` |
| `CatalogoHTTP`, `InventarioHTTP`, `ConsumoSimulado` | Adaptadores de salida a los otros servicios | `internal/adapters/catalogo/`, `internal/adapters/inventario/` |

Las piezas se arman a mano en `cmd/api/main.go` (`PedidosService{Repo: pedidosRepo}`), igual que en las demos de clase. En los tests, los puertos se reemplazan por mocks en memoria (`PedidosMock`, `CarritosMock`, `ConfirmacionesMock`, `CatalogoMock`, `InventarioMock`, `ConsumoMock`).

## El carrito

| Ruta en el gateway | Para qué |
| --- | --- |
| `GET /api/v1/carrito` | Ver el carrito (vacío si no tiene platos) |
| `POST /api/v1/carrito/items` | Agregar un plato: `{"platoId": "p1", "cantidad": 2}` |
| `PATCH /api/v1/carrito/items/{platoId}` | Dejar una cantidad final: `{"cantidad": 3}` |
| `DELETE /api/v1/carrito/items/{platoId}` | Sacar un plato (si era el último, se libera la reserva) |
| `DELETE /api/v1/carrito` | Vaciar y liberar la reserva |

Cada cambio sigue los mismos pasos: (1) Catálogo dice si el plato existe, su precio y su cocina (500 ms, un reintento); (2) el dominio aplica las reglas (una sola cocina, de 1 a 50 unidades, el precio se copia al agregar); (3) Inventario deja la reserva igual al carrito (800 ms, un reintento con la **misma** `Idempotency-Key`); (4) se guarda el carrito con control de versión.

La `Idempotency-Key` es carrito + número de cambio + una huella de los platos: reintentar el mismo cambio no reserva dos veces, y después de un intento fallido (por ejemplo, sin stock) el cambio siguiente usa otra clave.

| Respuesta | Cuándo |
| --- | --- |
| `409 SIN_STOCK` | No alcanzan los ingredientes; el carrito queda como estaba |
| `409 OTRA_COCINA` | El carrito ya tiene platos de otra cocina |
| `409 PLATO_NO_DISPONIBLE` | El plato no tiene receta cargada en Inventario |
| `409 CARRITO_VENCIDO` | Pasaron los 10 minutos: Inventario liberó todo y el carrito queda vacío |
| `409 CARRITO_MODIFICADO` | Otro cambio del mismo carrito llegó a la vez |
| `404 PLATO_NO_ENCONTRADO` | El plato no existe o está desactivado en Catálogo |
| `400 CANTIDAD_INVALIDA` | La cantidad no está entre 1 y 50 |
| `403 SIN_PERMISO` | No es un cliente (por ejemplo, una cuenta Cocina) |
| `503 SERVICIO_NO_DISPONIBLE` | Catálogo o Inventario no responden: el carrito no cambia, nunca se inventa un resultado |

## Cuánto le queda a la reserva

Todas las respuestas del carrito traen:

```json
{ "venceEn": "2026-11-09T01:29:14.803Z", "segundosRestantes": 595, "vencido": false }
```

- El plazo es de **10 minutos desde el primer plato** y agregar más no lo renueva. Lo fija Inventario con su reloj y Pedidos guarda `venceEn`; `segundosRestantes` se calcula en cada respuesta, redondeando hacia arriba.
- Sin platos no hay reserva: `segundosRestantes` es 0 y `vencido` es `false`.
- Cuando se cumple el plazo, `GET /carrito` responde `vencido: true` y `segundosRestantes: 0`, pero **no cambia nada**: mirar el carrito no modifica datos. Inventario libera los ingredientes en su barrido de cada 15 s; el próximo cambio del cliente recibe `409 CARRITO_VENCIDO` y el carrito queda vacío.
- El frontend puede mostrar la cuenta regresiva a partir de `venceEn`, sin pedir el carrito cada segundo.

## Confirmar el pedido

| Ruta en el gateway | Para qué |
| --- | --- |
| `POST /api/v1/pedidos` | Confirmar el carrito. Pide el header `Idempotency-Key` |
| `GET /api/v1/confirmaciones/{id}` | Ver cómo va una confirmación que respondió 202 |
| `GET /api/v1/pedidos/{id}` | Ver el pedido |

Confirmar toca Pedidos e Inventario, así que no entra en una sola transacción: es una **saga coordinada por Pedidos**, y cada paso queda guardado en la tabla `confirmacion` (ADR-004).

1. **Anotar** la confirmación, con una copia del carrito. La misma `Idempotency-Key` del mismo cliente es la misma confirmación (doble clic: un solo pedido), y un carrito se confirma una sola vez aunque lleguen dos claves a la vez.
2. **Descontar** en Inventario (`POST /reservas/{id}/consumir`). Si no contesta, se reintenta con **backoff y jitter** (100 ms, 200 ms, 400 ms… con ±50 % de azar), siempre con la **misma clave**, hasta 2 s. El **circuit breaker** se abre si en 10 llamadas falla la mitad: durante 10 s responde al instante sin llamar a Inventario.
3. **Crear el pedido** en RECIBIDO, marcar la confirmación COMPLETADA y vaciar el carrito, en **una sola transacción**.

| Respuesta | Cuándo |
| --- | --- |
| `201` + el pedido | Se completó. Repetir con la misma clave devuelve el mismo pedido |
| `202` + `"Estamos confirmando tu pedido"` | No sabemos todavía si Inventario descontó. **No se dice que falló.** Un proceso en segundo plano sigue cada 10 s desde el paso guardado; se consulta en `GET /confirmaciones/{id}` |
| `409 CARRITO_VENCIDO` | La reserva venció: no se descontó nada, la confirmación queda FALLIDA y el carrito vacío |
| `409 CARRITO_VACIO` | No hay platos para confirmar |
| `400 IDEMPOTENCY_KEY_INVALIDA` | Falta el header o no es válido |

**Descuento real en Inventario:** `docker-compose.yml` usa `CONSUMO_INVENTARIO: real`, así que confirmar descuenta el stock de verdad (`POST /reservas/{id}/consumir`). Con `simulado` Pedidos no llama a Inventario: siempre sale bien y no descuenta nada (sirve para probar Pedidos solo; los tests de `testdata/` lo usan así).

## Mis pedidos y confirmar que lo recibí

| Ruta (por el gateway) | Qué hace |
| --- | --- |
| `GET /api/v1/pedidos` | Mis pedidos (cliente) o los de mi cocina, de a 20 y del más nuevo al más viejo, con sus platos |
| `GET /api/v1/pedidos?estado=EN_CAMINO&pagina=2` | Los mismos, filtrados por estado y por página (los dos son opcionales) |
| `POST /api/v1/pedidos/{id}/entregado` | El cliente avisa que lo recibió: `EN_CAMINO` → `ENTREGADO` |

- Qué pedidos se ven lo decide la identidad del gateway (`X-Usuario-Id`, `X-Rol`, `X-Cocina-Id`), nunca un id de la URL.
- El gateway reenvía los parámetros `?...` a Pedidos, que los valida. Su log guarda la ruta sin ellos.

| Respuesta | Cuándo |
| --- | --- |
| `200` + `{"pedidos": [...], "pagina": 1, "porPagina": 20}` | La lista (vacía si esa página no tiene pedidos) |
| `200` + el pedido | Se confirmó la entrega |
| `400 DATOS_INVALIDOS` | Estado que no existe o página que no es un número desde 1 |
| `409 TRANSICION_INVALIDA` | El pedido todavía no salió (no está `EN_CAMINO`) |
| `409 PEDIDO_MODIFICADO` | Otro cambio se guardó primero |
| `403 SIN_PERMISO` | El admin no tiene pedidos; la cocina no confirma entregas |
| `404 PEDIDO_NO_ENCONTRADO` | El pedido no existe o es de otro cliente |

## Pantalla de pedidos de la cocina

| Ruta (por el gateway) | Qué hace |
| --- | --- |
| `GET /api/v1/cocina/pedidos` | Los pedidos de mi cocina, de a 20 y del más nuevo al más viejo (`?estado=RECIBIDO`, `?pagina=2`) |
| `POST /api/v1/cocina/pedidos/{id}/aceptar` | Tomar el pedido: `RECIBIDO` → `ACEPTADO` |
| `POST /api/v1/cocina/pedidos/{id}/rechazar` | No se puede hacer: `RECIBIDO` → `CANCELADO` |
| `POST /api/v1/cocina/pedidos/{id}/avanzar` | Paso siguiente: `ACEPTADO` → `EN_PREPARACION` → `LISTO` → `EN_CAMINO` |

- Solo cuentas Cocina (un cliente recibe `403`), y solo los pedidos de **su** cocina: el pedido de otra cocina responde `404`, como si no existiera.
- Cada acción la decide el pedido (dominio) y se guarda con control de versión: si el cliente cancela mientras la cocina acepta, gana el primero y el otro recibe `409 PEDIDO_MODIFICADO`.
- Desde `EN_CAMINO` la cocina ya no avanza: lo cierra el cliente al confirmar que lo recibió.

## Cancelar y devolver los ingredientes

| Ruta (por el gateway) | Qué hace |
| --- | --- |
| `POST /api/v1/pedidos/{id}/cancelar` | El cliente cancela su pedido: solo desde `RECIBIDO`, antes de que la cocina lo acepte |

Cuando un pedido pasa a `CANCELADO` (lo cancela el cliente o lo **rechaza la cocina**), hay que devolverle a Inventario lo que se descontó al confirmar:

1. En la **misma transacción** que el cambio de estado se anota una fila en `devolucion` (`PENDIENTE`), si el pedido vino de una confirmación (ahí se descontó el stock). O queda todo o nada: no puede quedar un pedido cancelado sin su devolución anotada.
2. Un proceso en segundo plano (cada 5 s) la manda a Inventario: `POST /consumos/{pedidoId}/devolver` con `Idempotency-Key: {pedidoId}-devolver`, **siempre la misma clave**, así reintentar no devuelve dos veces (Inventario devuelve exactamente lo que descontó, aunque después cambie la receta).
3. Si Inventario no contesta, queda `PENDIENTE`, suma un intento y se vuelve a probar en la vuelta siguiente. Si Inventario no tiene un descuento de ese pedido (`404`), no hay nada que devolver y se marca `HECHA`.

| Respuesta | Cuándo |
| --- | --- |
| `200` + el pedido en `CANCELADO` | Se canceló. Los ingredientes se devuelven en segundo plano |
| `409 TRANSICION_INVALIDA` | La cocina ya lo aceptó (o ya estaba cancelado) |
| `409 PEDIDO_MODIFICADO` | La cocina lo aceptó al mismo tiempo y ganó |
| `403 SIN_PERMISO` | No es un cliente (la cocina no cancela: rechaza) |
| `404 PEDIDO_NO_ENCONTRADO` | El pedido no existe o es de otro cliente |

Con `CONSUMO_INVENTARIO: real` (lo que usa `docker-compose.yml`) se le pide a Inventario que devuelva los ingredientes. Con `simulado` la devolución también es simulada: como el descuento no fue real, no hay nada que devolver.

## Historial y aviso de cada cambio de estado

- **Historial:** cada cambio queda en `pedido_historial` (estado anterior, nuevo, quién y cuándo) y se ve en `GET /api/v1/pedidos/{id}`, en `historial`.
- **Aviso:** cada cambio (también la creación en `RECIBIDO`) publica `pedido.estado_cambiado` en RabbitMQ, con el sobre común de [`docs/contracts/eventos`](../../docs/contracts/eventos/README.md). Lo escucha Notificaciones (cola `notificaciones.mailer`) para mandarle el mail al cliente.

Cómo no se pierde un aviso (**outbox**, ADR-005):

1. El mensaje se guarda en la tabla `outbox` en la **misma transacción** que el cambio: si el cambio no se guarda, tampoco el aviso, y al revés.
2. Un proceso en segundo plano (cada 1 s) los publica **en orden** y marca cada uno recién cuando RabbitMQ **confirma** que lo guardó (publisher confirms). Si uno falla, para ahí y reintenta en la vuelta siguiente, así no se adelanta un estado posterior.
3. Si RabbitMQ está caído, Pedidos sigue atendiendo: los mensajes esperan en MySQL y salen cuando vuelve.
4. Si Pedidos se cae justo después de publicar y antes de marcar, el mensaje sale dos veces. Por eso lleva `evento_id` (también como `message_id`) y `secuencia` (la versión del pedido): el consumidor descarta repetidos y viejos.

| Variable | Para qué |
| --- | --- |
| `RABBITMQ_URL` | `amqp://usuario:clave@rabbitmq:5672/`, armada en `docker-compose.yml` con `RABBITMQ_USER` y `RABBITMQ_PASSWORD` del `.env`. Sin ella, los mensajes quedan en el outbox |

## Carrito vencido por aviso de Inventario

Cuando una reserva cumple sus 10 minutos, Inventario libera los ingredientes y publica `reserva.vencida` (`{"reserva_id", "carrito_id", "cocina_id"}`). Pedidos escucha la cola `pedidos.reservas` y:

1. Vacía el carrito que **todavía tiene esa reserva**, le da otro id y anota `vencio_en`. `GET /api/v1/carrito` lo muestra en `vencioEn`, para que el frontend diga "Tu carrito venció" en vez de mostrar un carrito vacío sin explicación.
2. Si el carrito ya no tiene esa reserva (el cliente lo vació, lo confirmó o el aviso llega repetido), **no cambia nada**. Por eso el mismo mensaje dos veces no hace daño.
3. El próximo cambio del cliente borra `vencioEn`.

| Qué pasa con el mensaje | Respuesta a RabbitMQ |
| --- | --- |
| Se procesó (o no había nada que hacer) | `ACK`: sale de la cola |
| Está roto o es de otro tipo | Rechazo sin reencolar: va a `pedidos.reservas.dlq` |
| Falló algo pasajero (MySQL no contesta) | Espera 5 s y vuelve a la cola; a las 5 entregas RabbitMQ lo manda a la DLQ |

El consumidor lee de a un mensaje (prefetch 1) y, si se corta la conexión con RabbitMQ, vuelve a conectarse a los 5 s. Mientras tanto, el carrito sigue funcionando: si el cliente lo toca con la reserva vencida, recibe `409 CARRITO_VENCIDO`.

## Factura con el grupo proveedor

Lo que consumimos de otro grupo es la **API de comprobantes del Grupo 4**: con el mail del cliente y las líneas del pedido, emite la factura en PDF y se la manda por correo. Su contrato y cómo lo usamos están en [`docs/contracts/proveedor`](../../docs/contracts/proveedor/README.md), y la decisión, en el [ADR-009](../../docs/adr/ADR-009-consumo-del-proveedor.md).

**La factura no es un paso esencial: el pedido nunca la espera.**

1. Al completar la confirmación, en la **misma transacción** que crea el pedido, se anota su factura `PENDIENTE` (tabla `comprobante`).
2. Un proceso en segundo plano (cada 10 s) la pide: `POST /v1/comprobantes` con `X-API-Key`, `Idempotency-Key: pedido-{id}` (**siempre la misma**, así no hay dos facturas) y las líneas del pedido con el precio que se copió al confirmar.
3. Según la respuesta:

| Responde | Qué hacemos |
| --- | --- |
| `201` o `200` | `EMITIDO`, con su número |
| `400`, `409` o `422` | `RECHAZADO`: reintentar no lo arregla (queda el motivo) |
| `401`, `429`, `503` o no contesta en 5 s | Sigue `PENDIENTE` y se reintenta en la vuelta siguiente |

- Tiene su propio **circuit breaker** (ADR-010): si en 10 llamadas falla la mitad, no los llamamos durante 10 s.
- `GET /api/v1/pedidos/{id}` muestra cómo va: `"comprobante": {"estado": "EMITIDO", "numero": "B-0001-00000042"}`.
- Configuración: `PROVIDER_BASE_URL` y `PROVIDER_API_KEY` del `.env`. Mientras estén vacías (el Grupo 4 todavía no publicó su URL), `docker-compose.yml` usa su mock, `comprobantes-mock`, que valida cada solicitud contra su contrato.

## Métricas del negocio para el admin

`GET /api/v1/admin/metricas[?cocinaId=]` devuelve, para los **últimos 7 días** (hoy y los 6 anteriores, cortando los días en hora de Córdoba):

- cuántos pedidos tuvo cada cocina de la localidad del admin (`cocinas`);
- el detalle de una cocina (`detalle`, la elegida o la primera): pedidos y facturación sin contar los cancelados, cancelados, ticket promedio, pedidos por día, los 5 platos más vendidos, en qué porcentaje de los pedidos aparece cada restaurante y los minutos promedio de recibido a entregado.

Solo el **admin**, y solo de **su localidad**: la lista de cocinas de la localidad la da Catálogo (`/internal/cocinas`), nunca lo que mande el cliente. Una cocina de otra localidad responde `403`. Son consultas de solo lectura sobre `pedido`, `pedido_item` y `pedido_historial`, que usan el índice `idx_pedido_cocina`. Las prueban `application/metricas_test.go` (permisos, cocina de otra localidad, sin cocinas) y `adapters/mysql/metricas_integration_test.go` contra MySQL real (cancelados, día en hora de Córdoba, ranking y tiempo de entrega).

## Probar

Sin Docker, las reglas del pedido y las rutas (con un repositorio en memoria):

```powershell
cd services/pedidos
go test ./...
```

Contra MySQL real, en una base descartable que no usa el `.env` ni los datos de desarrollo:

```powershell
docker compose -p gk-pedidos-check -f services/pedidos/testdata/compose.yml up -d --build --wait pedidos comprobantes-mock
docker compose -p gk-pedidos-check -f services/pedidos/testdata/compose.yml run --rm --build pruebas
docker compose -p gk-pedidos-check -f services/pedidos/testdata/compose.yml down -v
```

El test `TestMySQLCambiosSimultaneosGanaUnoSolo` manda 20 veces "cancelar" (cliente) y "aceptar" (cocina) al mismo pedido a la vez: en cada vuelta gana uno solo, la versión queda en 2 y el historial tiene dos entradas.

El mismo comando corre el **test de contrato con el grupo proveedor** (`adapters/proveedor/contrato_test.go`): le manda al mock del Grupo 4, servido desde la copia de su OpenAPI, lo que arma nuestro adaptador. Prism valida cada solicitud contra su contrato y responde `422` si algo no cumple. Si el Grupo 4 publica una versión nueva, se reemplaza `docs/contracts/proveedor/comprobantes-v1.yaml` y este test avisa si algo dejó de ser compatible ([ADR-009](../../docs/adr/ADR-009-consumo-del-proveedor.md)).

**De punta a punta**, con todo el sistema levantado y entrando por el gateway como el frontend:

```powershell
docker compose up -d --build
node services/pedidos/testdata/e2e.mjs
```

Necesita Node 18 o más nuevo y el `.env` completo (usa `ADMIN_PASSWORD` y el usuario de RabbitMQ, sin imprimirlos). Si el gateway ya estaba levantado, `docker compose restart gateway` para que tome las rutas nuevas. Cada línea sale `OK` o `FALLA`, y al final `TODO OK`:

| Parte | Qué comprueba |
| --- | --- |
| Preparación | El admin crea una cuenta Cocina; la cocina crea un plato, un ingrediente con stock 10 y la receta, y activa el plato |
| Confirmar y cancelar | Carrito → Inventario reserva; confirmar → 201 y el stock **baja** (doble clic con la misma clave: el mismo pedido); mis pedidos y la pantalla de la cocina con `?estado=`; cancelar → el stock **vuelve** en segundo plano |
| Cocina y entrega | La cocina acepta y avanza hasta `EN_CAMINO`; antes de eso no se puede confirmar la entrega y después no se puede cancelar; el cliente confirma que lo recibió y el historial tiene los 6 estados |
| Mensajes | Los 8 `pedido.estado_cambiado` llegan a la cola `notificaciones.mailer` |

Deja datos de prueba (cuentas `cocina.e2e…` y `cliente.e2e…`, un plato y un ingrediente "e2e", pedidos y mensajes en la cola). Un plato recién activado tarda uno o dos segundos en poder pedirse, hasta que Inventario recibe el aviso de Catálogo por RabbitMQ (consistencia eventual): el script lo espera.

## Qué hace

- **Carrito:** guarda los platos y cantidades que eligió el cliente, todos de **una sola cocina**. Guarda una copia del nombre, el precio y el restaurante de cada plato, y el número de la reserva que hizo Inventario.
- **Confirmación:** cuando el cliente confirma, coordina los pasos con Inventario (y con el grupo proveedor, en el paso que corresponda). Solo crea el pedido si todo salió bien.
- **Estados:** controla que el pedido pase por los estados en el orden correcto y guarda el historial de cambios.
- **Cancelar y rechazar:** el pedido pasa a CANCELADO y le pide a Inventario que devuelva el stock.
- Es el **único servicio que habla con el grupo proveedor**.

## Estados del pedido

```
RECIBIDO → ACEPTADO → EN_PREPARACION → LISTO → EN_CAMINO → ENTREGADO
    └──→ CANCELADO   (solo desde RECIBIDO: cancela el cliente o rechaza la cocina)
```

La cocina lo avanza hasta EN_CAMINO y el cliente confirma que lo recibió (ENTREGADO). No existe un estado "rechazado". El pedido es **uno solo**: los platos se muestran agrupados por restaurante, pero los restaurantes no tienen estados propios.

## Qué datos guarda (MySQL)

| Tabla | Qué guarda |
| --- | --- |
| `carrito` / `carrito_item` | El carrito: cocina, reserva, cuándo vence, si venció (`vencio_en`) y sus platos |
| `pedido` | Cliente, copia de su mail, cocina, estado, total y un número de **versión** |
| `pedido_item` | Cada plato con su nombre, precio, restaurante y cantidad |
| `pedido_historial` | Cada cambio de estado, cuándo y quién lo hizo |
| `confirmacion` | En qué paso va cada confirmación (la saga), con una copia del carrito |
| `devolucion` | Los ingredientes a devolver de cada pedido cancelado, y si ya se devolvieron |
| `outbox` | Los avisos `pedido.estado_cambiado` que todavía no confirmó RabbitMQ (y cuándo se publicó cada uno) |
| `comprobante` | La factura de cada pedido con el grupo proveedor: pendiente, emitida (con su número) o rechazada |

## Cómo se organiza por dentro: hexagonal + DDD

Igual que Inventario, las reglas quedan **en el centro**, separadas de la base, de HTTP y de los otros servicios. Además usamos una idea de DDD: el **pedido controla sus propias reglas**. No se le puede cambiar el estado desde cualquier lado; hay que pedírselo al pedido, y él decide si el cambio es válido (por ejemplo, no se puede cancelar si ya está ACEPTADO).

El número de **versión** sirve para cuando dos cosas pasan a la vez. Si el cliente cancela justo cuando la cocina acepta, se guarda solo el primer cambio y el otro se rechaza.

```
cmd/api/                      arranca el servicio
internal/domain/              el pedido y sus reglas, el carrito, el monto
internal/application/         las acciones: AgregarAlCarrito, Confirmar, Cancelar, Rechazar, Avanzar, ConfirmarEntrega
internal/ports/               lo que el centro ofrece y lo que necesita (Catálogo, Inventario, proveedor, base, mensajes)
internal/adapters/http/       rutas HTTP
internal/adapters/mysql/      acceso a MySQL
internal/adapters/rabbitmq/   envío y lectura de mensajes
internal/adapters/catalogo/   para pedirle el plato a Catálogo
internal/adapters/inventario/ para reservar, descontar y devolver en Inventario
internal/adapters/proveedor/  para hablar con el grupo proveedor
migrations/                   tablas
```

Si el grupo proveedor cambia su API, solo cambiamos su adaptador: las reglas del pedido no se tocan.

## Rutas

Salvo las rutas técnicas comunes, la tabla muestra rutas internas del servicio. El frontend entra por `/api/v1` a través del gateway; las rutas entre servicios no se publican al exterior.

| Ruta | Quién la usa | Para qué |
| --- | --- | --- |
| `GET`/`DELETE /carrito` · `POST /carrito/items` · `PATCH`/`DELETE /carrito/items/{platoId}` | cliente | Usar el carrito |
| `POST /pedidos` | cliente | Confirmar (con clave para no crear dos pedidos) |
| `GET /pedidos/mios` | cliente | Ver sus pedidos y su historial |
| `POST /pedidos/{id}/cancelar` · `/entregado` | cliente | Cancelar o confirmar que lo recibió |
| `GET /cocina/pedidos` | cocina | Ver los pedidos de su cocina |
| `POST /cocina/pedidos/{id}/aceptar` · `/rechazar` · `/avanzar` | cocina | Cambiar el estado |
| `GET /admin/metricas?cocinaId=` | admin | Ver las métricas de las cocinas de su localidad |

**Permisos:** un cliente solo ve sus pedidos y solo compra en una cocina activa; una cocina solo ve y cambia sus propios pedidos.

## Con quién habla

- **Llama a** Catálogo (para ver el plato y la cocina), a Inventario (para reservar, descontar, devolver y preguntar si algo se hizo) y al grupo proveedor. Siempre con un tiempo máximo de espera, reintentos con la misma clave y circuit breaker ([ADR-005](../../docs/adr/ADR-005-comunicacion-entre-servicios.md)).
- **Manda** `pedido.estado_cambiado`, con el mail del cliente adentro.
- **Escucha** `reserva.vencida`.

## Dependencias y fallas

| Dependencia | Para qué | Si falla |
| --- | --- | --- |
| MySQL (`pedidos_db`) | Carritos, pedidos, historial y pasos de recuperación | No se puede operar el carrito ni consultar o cambiar pedidos con garantías |
| Catálogo | Validar plato, cocina y precio | No se agrega el plato; máximo 500 ms por intento y un reintento |
| Inventario | Reservar, descontar y devolver | Máximo 800 ms por intento y un reintento con la misma clave; si el resultado es incierto, se consulta y se recupera |
| Grupo proveedor (Grupo 4) | Pedir la factura del pedido | La factura queda pendiente y se pide sola cuando vuelve, en segundo plano: el pedido se confirma igual |
| RabbitMQ | Publicar estados y recibir vencimientos | El outbox conserva los eventos; los avisos y la actualización de carritos vencidos pueden atrasarse |

Autenticación, llamadas, esperas entre reintentos y trabajo local comparten **3 s totales**. Si una confirmación no tiene resultado seguro dentro de ese plazo, se muestra "Estamos confirmando tu pedido" y continúa desde el paso persistido. No se responde que fracasó mientras se desconoce si el stock se descontó ([ADR-005](../../docs/adr/ADR-005-comunicacion-entre-servicios.md)).

## Procesos en segundo plano

- Publicación del outbox con confirmación del broker.
- Recuperación de confirmaciones incompletas desde su paso persistido.
- Recuperación de devoluciones pendientes: cancelar el pedido no se frena porque Inventario esté indisponible.
- Pedido de las facturas pendientes al proveedor.

El cambio de estado, su historial y el evento se guardan en una sola transacción. El admin solo consulta métricas de cocinas de su localidad; Pedidos guarda la localidad de la cocina como referencia al crear el pedido.

## Configuración

`PEDIDOS_DB_PASSWORD`, `RABBITMQ_USER`, `RABBITMQ_PASSWORD`, `PROVIDER_BASE_URL` y `PROVIDER_API_KEY` están en las [variables del `.env`](../../README.md#variables-del-env). Las direcciones internas de Catálogo e Inventario, el modo de Inventario (`CONSUMO_INVENTARIO`) y el mock del proveedor se arman en `docker-compose.yml`. El proveedor es el Grupo 4 (API de comprobantes).

## Pruebas

- Transiciones inválidas, cancelación después de aceptar y acceso a pedidos de otro cliente o cocina.
- Dos confirmaciones con la misma clave crean un pedido; cancelar y aceptar a la vez permite solo una transición.
- Caída en cada paso de la confirmación y recuperación al reiniciar; devolución pendiente sin duplicar stock.
- Contrato del proveedor contra su mock (`contrato_test.go`); creación conjunta del cambio de estado, historial y outbox.

Todo esto está cubierto por tests de Go: sin base para las reglas, y contra MySQL y RabbitMQ reales con `testdata/compose.yml`.
