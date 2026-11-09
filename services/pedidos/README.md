# Pedidos

**Responsable:** Lucía Rucci.

> **L5:** servicio base hexagonal con MySQL y el **agregado Pedido** con sus estados. Aplica sus migraciones al arrancar (`mysql-startup`) y atiende en el puerto interno 8085, sin publicarse fuera de Docker. Ya funcionan las transiciones con sus permisos, el historial, el control de versión (si dos cambios llegan a la vez, gana el primero) y `GET /pedidos/{id}`.
>
> **L6:** el **carrito**. El cliente agrega, cambia y saca platos de una sola cocina; el precio sale de Catálogo y se guarda al agregar, y cada cambio deja la reserva de Inventario igual al carrito. Entra por el gateway en `/api/v1/carrito` (con login). La confirmación llega en L8.
>
> **L7:** el carrito muestra **cuánto le queda a la reserva** (`segundosRestantes`) y si ya venció (`vencido`).
>
> **L8:** **confirmar el pedido como saga**, con plazo de 3 s, circuit breaker (`sony/gobreaker`) y reintentos con backoff (`cenkalti/backoff`). El descuento en Inventario es **simulado** (`CONSUMO_INVENTARIO=simulado`) hasta que exista la V10.

Maneja **el pedido de principio a fin**. Puerto interno **8085**. Es uno de los dos servicios donde no se puede equivocar nada.

## Cómo está organizado (hexagonal, como en la Clase 8)

| Pieza | Rol en hexagonal | Archivo |
| --- | --- | --- |
| `PedidosController`, `CarritoController`, `ConfirmacionController` | Adaptador de entrada (Gin) | `internal/adapters/http/controllers.go` |
| `PedidosService`, `CarritoService`, `ConfirmacionService` | Casos de uso (núcleo) | `internal/application/` |
| `Pedido`, `Carrito`, `Confirmacion` y sus reglas | Dominio (núcleo) | `internal/domain/` |
| `PedidosRepo`, `CarritosRepo`, `ConfirmacionesRepo`, `Catalogo`, `Inventario`, `Consumo` | Puertos de salida (interfaces) | `internal/ports/` |
| `PedidosMySQL`, `CarritosMySQL`, `ConfirmacionesMySQL` | Adaptadores de salida a MySQL | `internal/adapters/mysql/` |
| `CatalogoHTTP`, `InventarioHTTP`, `ConsumoSimulado` | Adaptadores de salida a los otros servicios | `internal/adapters/catalogo/`, `internal/adapters/inventario/` |

Las piezas se arman a mano en `cmd/api/main.go` (`PedidosService{Repo: pedidosRepo}`), igual que en las demos de clase. En los tests, los puertos se reemplazan por mocks en memoria (`PedidosMock`, `CarritosMock`, `ConfirmacionesMock`, `CatalogoMock`, `InventarioMock`, `ConsumoMock`).

## El carrito (L6)

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

## Cuánto le queda a la reserva (L7)

Todas las respuestas del carrito traen:

```json
{ "venceEn": "2026-11-09T01:29:14.803Z", "segundosRestantes": 595, "vencido": false }
```

- El plazo es de **10 minutos desde el primer plato** y agregar más no lo renueva. Lo fija Inventario con su reloj y Pedidos guarda `venceEn`; `segundosRestantes` se calcula en cada respuesta, redondeando hacia arriba.
- Sin platos no hay reserva: `segundosRestantes` es 0 y `vencido` es `false`.
- Cuando se cumple el plazo, `GET /carrito` responde `vencido: true` y `segundosRestantes: 0`, pero **no cambia nada**: mirar el carrito no modifica datos. Inventario libera los ingredientes en su barrido de cada 15 s; el próximo cambio del cliente recibe `409 CARRITO_VENCIDO` y el carrito queda vacío.
- El frontend puede mostrar la cuenta regresiva a partir de `venceEn`, sin pedir el carrito cada segundo.

## Confirmar el pedido (L8)

| Ruta en el gateway | Para qué |
| --- | --- |
| `POST /api/v1/pedidos` | Confirmar el carrito. Pide el header `Idempotency-Key` |
| `GET /api/v1/confirmaciones/{id}` | Ver cómo va una confirmación que respondió 202 |
| `GET /api/v1/pedidos/{id}` | Ver el pedido |

Confirmar toca Pedidos e Inventario, así que no entra en una sola transacción: es una **saga coordinada por Pedidos**, y cada paso queda guardado en la tabla `confirmacion` (ADR-004).

1. **Anotar** la confirmación, con una copia del carrito. La misma `Idempotency-Key` del mismo cliente es la misma confirmación (doble clic: un solo pedido), y un carrito se confirma una sola vez aunque lleguen dos claves a la vez.
2. **Descontar** en Inventario (`POST /reservas/{id}/consumir`). Si no contesta, se reintenta con **backoff y jitter** (100 ms, 200 ms, 400 ms… con ±50 % de azar), siempre con la **misma clave**, hasta 2 s. El **circuit breaker** se abre si en 10 llamadas falla la mitad: durante 10 s responde al instante sin llamar a Inventario.
3. *(El paso del grupo proveedor va acá cuando la cátedra lo asigne: L15.)*
4. **Crear el pedido** en RECIBIDO, marcar la confirmación COMPLETADA y vaciar el carrito, en **una sola transacción**.

| Respuesta | Cuándo |
| --- | --- |
| `201` + el pedido | Se completó. Repetir con la misma clave devuelve el mismo pedido |
| `202` + `"Estamos confirmando tu pedido"` | No sabemos todavía si Inventario descontó. **No se dice que falló.** Un proceso en segundo plano sigue cada 10 s desde el paso guardado; se consulta en `GET /confirmaciones/{id}` |
| `409 CARRITO_VENCIDO` | La reserva venció: no se descontó nada, la confirmación queda FALLIDA y el carrito vacío |
| `409 CARRITO_VACIO` | No hay platos para confirmar |
| `400 IDEMPOTENCY_KEY_INVALIDA` | Falta el header o no es válido |

**Mientras no exista la V10**, el descuento es simulado: siempre sale bien, pero **no descuenta stock de verdad** (la reserva queda activa y se libera sola cuando venza). Cuando Vale la termine se cambia `CONSUMO_INVENTARIO: real` en `docker-compose.yml`; el adaptador real ya está hecho y probado con un Inventario falso.

## Mis pedidos y confirmar que lo recibí (L11)

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

## Pantalla de pedidos de la cocina (L12)

| Ruta (por el gateway) | Qué hace |
| --- | --- |
| `GET /api/v1/cocina/pedidos` | Los pedidos de mi cocina, de a 20 y del más nuevo al más viejo (`?estado=RECIBIDO`, `?pagina=2`) |
| `POST /api/v1/cocina/pedidos/{id}/aceptar` | Tomar el pedido: `RECIBIDO` → `ACEPTADO` |
| `POST /api/v1/cocina/pedidos/{id}/rechazar` | No se puede hacer: `RECIBIDO` → `CANCELADO` |
| `POST /api/v1/cocina/pedidos/{id}/avanzar` | Paso siguiente: `ACEPTADO` → `EN_PREPARACION` → `LISTO` → `EN_CAMINO` |

- Solo cuentas Cocina (un cliente recibe `403`), y solo los pedidos de **su** cocina: el pedido de otra cocina responde `404`, como si no existiera.
- Cada acción la decide el pedido (dominio) y se guarda con control de versión: si el cliente cancela mientras la cocina acepta, gana el primero y el otro recibe `409 PEDIDO_MODIFICADO`.
- Desde `EN_CAMINO` la cocina ya no avanza: lo cierra el cliente al confirmar que lo recibió.

## Probar

Sin Docker, las reglas del pedido y las rutas (con un repositorio en memoria):

```powershell
cd services/pedidos
go test ./...
```

Contra MySQL real, en una base descartable que no usa el `.env` ni los datos de desarrollo:

```powershell
docker compose -p gk-pedidos-check -f services/pedidos/testdata/compose.yml up -d --build --wait pedidos
docker compose -p gk-pedidos-check -f services/pedidos/testdata/compose.yml run --rm --build pruebas
docker compose -p gk-pedidos-check -f services/pedidos/testdata/compose.yml down -v
```

El test `TestMySQLCambiosSimultaneosGanaUnoSolo` manda 20 veces "cancelar" (cliente) y "aceptar" (cocina) al mismo pedido a la vez: en cada vuelta gana uno solo, la versión queda en 2 y el historial tiene dos entradas.

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
| `carrito` / `carrito_item` | El carrito: cocina, reserva, cuándo vence y sus platos |
| `pedido` | Cliente, copia de su mail, cocina, estado, total y un número de **versión** |
| `pedido_item` | Cada plato con su nombre, precio, restaurante y cantidad |
| `pedido_historial` | Cada cambio de estado, cuándo y quién lo hizo |
| `saga_paso` | En qué paso va cada confirmación o devolución |
| `idempotencia` · `outbox` · `mensaje_procesado` | Para no repetir operaciones, no perder mensajes y no procesar dos veces el mismo mensaje |

## Cómo se organiza por dentro: hexagonal + DDD

Igual que Inventario, las reglas quedan **en el centro**, separadas de la base, de HTTP y de los otros servicios. Además usamos una idea de DDD: el **pedido controla sus propias reglas**. No se le puede cambiar el estado desde cualquier lado; hay que pedírselo al pedido, y él decide si el cambio es válido (por ejemplo, no se puede cancelar si ya está ACEPTADO) (Clase 8).

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
| `GET /admin/cocinas/{id}/metricas` | admin | Ver métricas de su cocina |

**Permisos:** un cliente solo ve sus pedidos y solo compra en una cocina activa; una cocina solo ve y cambia sus propios pedidos.

## Con quién habla

- **Llama a** Catálogo (para ver el plato y la cocina), a Inventario (para reservar, descontar, devolver y preguntar si algo se hizo) y al grupo proveedor. Siempre con un tiempo máximo de espera, reintentos con la misma clave y circuit breaker ([ADR-005](../../docs/adr/ADR-005-comunicacion-entre-servicios.md)).
- **Manda** `pedido.estado_cambiado`, con el mail del cliente adentro.
- **Escucha** `reserva.vencida`.

## Dependencias y fallas previstas

| Dependencia | Para qué | Si falla |
| --- | --- | --- |
| MySQL (`pedidos_db`) | Carritos, pedidos, historial y pasos de recuperación | No se puede operar el carrito ni consultar o cambiar pedidos con garantías |
| Catálogo | Validar plato, cocina y precio | No se agrega el plato; máximo 500 ms por intento y un reintento |
| Inventario | Reservar, descontar y devolver | Máximo 800 ms por intento y un reintento con la misma clave; si el resultado es incierto, se consulta y se recupera |
| Grupo proveedor | Paso de confirmación pendiente de asignación | Usa el presupuesto restante; se recupera o compensa según su contrato, sin inventar el resultado |
| RabbitMQ | Publicar estados y recibir vencimientos | El outbox conserva los eventos; los avisos y la actualización de carritos vencidos pueden atrasarse |

Autenticación, llamadas, esperas entre reintentos y trabajo local comparten **3 s totales**. Si una confirmación no tiene resultado seguro dentro de ese plazo, se muestra "Estamos confirmando tu pedido" y continúa desde el paso persistido. No se responde que fracasó mientras se desconoce si el stock se descontó ([ADR-005](../../docs/adr/ADR-005-comunicacion-entre-servicios.md)).

## Procesos en segundo plano

- Publicación del outbox con confirmación del broker.
- Recuperación de confirmaciones incompletas desde su paso persistido.
- Recuperación de devoluciones pendientes: cancelar el pedido no se frena porque Inventario esté indisponible.

El cambio de estado, su historial y el evento se guardan en una sola transacción. El admin solo consulta métricas de cocinas de su localidad; Pedidos guarda la localidad de la cocina como referencia al crear el pedido.

## Configuración prevista

`PEDIDOS_DB_PASSWORD`, `PROVIDER_BASE_URL` y `PROVIDER_API_KEY`, junto con las credenciales comunes de MySQL/RabbitMQ, están en las [variables del `.env`](../../README.md#variables-del-env). Las conexiones, el puerto y las direcciones internas de Catálogo e Inventario se definirán al implementar. El proveedor todavía no fue asignado.

## Pruebas previstas

- Transiciones inválidas, cancelación después de aceptar y acceso a pedidos de otro cliente o cocina.
- Dos confirmaciones con la misma clave crean un pedido; cancelar y aceptar a la vez permite solo una transición.
- Caída en cada paso de la confirmación y recuperación al reiniciar; devolución pendiente sin duplicar stock.
- Contrato del proveedor cuando se conozca; creación conjunta del cambio de estado, historial y outbox.
