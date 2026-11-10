# Inventario

**Responsable:** Valentina Rey.

Dice **con qué se hace cada plato y si alcanza**. Puerto interno **8084**, solo dentro de Docker. Es uno de los dos servicios donde no se puede equivocar nada: nunca se puede vender más de lo que hay ([ADR-004](../../docs/adr/ADR-004-consistencia-y-concurrencia.md)).

## Qué hace

- Guarda los **ingredientes** de cada cocina con su stock: cuánto hay en total, cuánto está reservado y cuánto queda disponible (`total − reservado`, que nunca puede ser negativo).
- Guarda las **recetas**: qué ingredientes y cuánto usa cada plato. Solo se pueden usar ingredientes de la cocina del plato.
- **Reserva** los ingredientes cuando el cliente agrega un plato: **todos o ninguno**, por 10 minutos.
- Ajusta o libera la reserva cuando el cliente cambia el carrito, y la **libera sola** cuando pasan los 10 minutos.
- **Descuenta** el stock cuando se confirma el pedido y lo **devuelve** si se cancela.
- La **cuenta Cocina** administra ingredientes, recetas, reposiciones y mermas, solo de su cocina. El admin no administra el inventario.
- Avisa cuando un plato pasa a estar disponible o deja de estarlo.

**No llama a ningún otro servicio.** Lo hicimos así a propósito: lo más importante del sistema no depende de que otro servicio esté andando. Lo que necesita saber de Catálogo (qué cocinas y platos existen y si están activos) le llega por mensajes.

## Cómo levantarlo

Desde la raíz del repositorio, con las contraseñas de MySQL en el `.env`:

```bash
docker compose up -d --build inventario
docker compose logs inventario
docker compose exec inventario wget -qO- http://127.0.0.1:8084/ready
```

Al arrancar aplica sus migraciones ([guía de MySQL](../../infra/mysql/README.md)); si una falla, no arranca. `/health` dice si el proceso está vivo y `/ready` si llega a MySQL.

| Variable | Valor |
| --- | --- |
| `PORT` | `8084` |
| `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_NAME` | `mysql`, `3306`, `inventario`, `inventario_db` |
| `DB_PASSWORD` | Sale de `INVENTARIO_DB_PASSWORD` del `.env` |

## Cómo se organiza por dentro: hexagonal liviano

Las reglas del stock quedan **en el centro** y no dependen de la base de datos, de Gin ni de RabbitMQ. El centro dice lo que necesita (por ejemplo, "guardar una reserva") y por afuera hay **adaptadores** que lo hacen con MySQL, HTTP o RabbitMQ. Así podemos testear las reglas sin levantar una base. Es "liviano" porque no usamos todos los patrones de DDD.

```
cmd/api/                     arranca el servicio y los procesos de fondo (vencer reservas cada 15 s y publicar el outbox)
internal/domain/             las reglas: disponible ≥ 0, recetas, estados de la reserva
internal/application/        las acciones: ingredientes, recetas, reservas y consumos
internal/ports/              lo que el centro necesita de afuera
internal/adapters/http/      rutas HTTP
internal/adapters/mysql/     acceso a MySQL, outbox y disponibilidad
internal/adapters/rabbitmq/  publica los eventos y lee los de Catálogo
migrations/                  tablas
```

**Cómo reserva "todo o nada":** en una sola transacción, para cada ingrediente de la receta:

```sql
UPDATE ingrediente SET reservado = reservado + :n
WHERE id = :id AND total - reservado >= :n;
-- si no cambió ninguna fila, no alcanza: se deshace todo
```

La base solo suma si alcanza. Por eso, si dos personas quieren el último plato a la vez, solo una lo consigue.

## Qué datos guarda (MySQL)

| Tabla | Qué guarda |
| --- | --- |
| `ingrediente` | Cocina, nombre, unidad, total y reservado |
| `receta` / `receta_item` | Qué ingredientes y cuánto usa cada plato, con su versión |
| `reserva` / `reserva_plato` / `reserva_item` | Cada reserva, su estado, cuándo vence y cuánto aparta de cada ingrediente |
| `consumo` | Lo que se descontó al confirmar cada pedido, para poder devolverlo igual |
| `movimiento_stock` | El historial: reposiciones y mermas |
| `cocina_ref` / `plato_ref` | La copia local de las cocinas y platos de Catálogo, que llega por mensajes |
| `disponibilidad_estado` / `disponibilidad_bloqueo` | Si cada plato se puede pedir, para avisar solo cuando cambia |
| `idempotencia` · `outbox` · `mensaje_procesado` | Para no repetir operaciones, no perder mensajes y no procesar dos veces el mismo mensaje |

## Rutas

El frontend entra por `/api/v1` a través del gateway; las rutas que usa Pedidos son internas y no se publican.

| Ruta | Quién la usa | Para qué |
| --- | --- | --- |
| `GET`·`POST /cocina/ingredientes` · `GET /cocina/ingredientes/{id}` | Cocina | Ver el stock y cargar ingredientes |
| `POST /cocina/ingredientes/{id}/reposiciones` · `/mermas` | Cocina | Reponer o registrar una merma |
| `PUT`·`GET /cocina/platos/{id}/receta` · `GET /cocina/recetas` | Cocina | Cargar, ver o listar recetas |
| `POST /reservas` · `PATCH /reservas/{id}` · `POST /reservas/{id}/liberar` | Pedidos | Reservar, cambiar o liberar (con `Idempotency-Key`) |
| `POST /reservas/{id}/consumir` | Pedidos | Descontar el stock al confirmar |
| `POST /consumos/{pedidoId}/devolver` | Pedidos | Devolver el stock al cancelar |
| `GET /reservas/{id}` · `GET /consumos/{pedidoId}` | Pedidos | Preguntar si algo se hizo, cuando no hubo respuesta a tiempo |
| `GET /platos/disponibilidad` | Búsqueda | Reconstruir el índice |
| `GET /health` · `GET /ready` · `GET /metrics` | Docker y Prometheus | Rutas técnicas |

**Permisos:** solo la cuenta Cocina administra el inventario, y solo el de su cocina (la que viene en el token verificado por el gateway, nunca una que mande el cliente). El admin recibe `403`; un recurso de otra cocina, `404`.

### Ingredientes

```json
{"nombre":"Harina","unidad":"gramos","stockInicial":1000}
```

Nombre y unidad son obligatorios (hasta 120 y 30 caracteres); el stock inicial es entero y no negativo. La cocina se toma de la identidad, no del cuerpo. El ingrediente nace con reservado 0. MySQL también impide un stock negativo o un reservado mayor al total. Una merma nunca puede dejar el total por debajo de lo reservado.

### Recetas

El `PUT` recibe los ingredientes para **una unidad del plato**:

```json
{"items":[{"ingredienteId":"UUID-del-ingrediente","cantidad":200}]}
```

Lista no vacía, ingredientes sin repetir y cantidades enteras mayores a cero. Guardar reemplaza la receta completa en una transacción: si algo falla, queda la anterior. Cada guardado sube la `version`. Cambiar una receta **no cambia las reservas ni los consumos que ya existen**: esos guardan las cantidades que usaron.

### Reservas

```json
{
  "carritoId":"carrito-123",
  "cocinaId":"nueva-cordoba",
  "items":[{"platoId":"p1","cantidad":2},{"platoId":"p2","cantidad":1}]
}
```

- Hay **una reserva por carrito**. El `PATCH` recibe el **contenido final completo**: omitir un plato libera sus ingredientes y `{"items":[]}` libera todo.
- Si un ingrediente no alcanza, se deshace **todo** el intento y responde `409 SIN_STOCK`; un ajuste que falla deja la reserva como estaba.
- Los **10 minutos** empiezan con la primera reserva del carrito: agregar platos o cambiar cantidades no los reinicia. Un proceso cada 15 s libera las reservas vencidas y publica `reserva.vencida`; confirmar también revisa el vencimiento. Descontar y vencer bloquean la misma reserva, así que solo una de las dos tiene efecto.
- **Idempotencia:** cada escritura lleva `Idempotency-Key` (1 a 128 caracteres). La misma clave con los mismos datos devuelve el resultado original; con otros datos, `409 IDEMPOTENCIA_CONFLICTO`. Los rechazos del negocio (como `SIN_STOCK`) también quedan guardados: para volver a intentar hace falta otra clave. Un error técnico deshace todo y se puede repetir con la misma clave. Dos pedidos iguales al mismo tiempo esperan el mismo resultado; si tarda demasiado, `409 OPERACION_EN_PROCESO` con `Retry-After: 1`. Ante un *deadlock* se reintenta la transacción hasta dos veces.
- Otros errores: `RESERVA_VENCIDA` y `RESERVA_CONFLICTO` dan `409`; un plato sin receta, inactivo o de otra cocina da `404`; datos inválidos, `400`.

## Mensajes

| Manda | Quién lo escucha | Para qué |
| --- | --- | --- |
| `disponibilidad.cambiada` | Búsqueda | Mostrar si un plato se puede pedir |
| `reserva.vencida` | Pedidos | Marcar el carrito como vencido |

| Escucha (cola `inventario.referencias`) | Para qué |
| --- | --- |
| `cocina.*` | Conocer cada cocina y si está activa |
| `plato.*` | Saber de qué cocina es cada plato y si sigue activo |

Los eventos se guardan en el outbox **en la misma transacción** que el cambio de stock, y un proceso los publica cada segundo. Los mensajes que escucha se registran en `mensaje_procesado` junto con su efecto, así un mensaje repetido no se aplica dos veces ([ADR-005](../../docs/adr/ADR-005-comunicacion-entre-servicios.md)).

## Si algo falla

| Dependencia | Para qué | Si falla |
| --- | --- | --- |
| MySQL (`inventario_db`) | Stock, recetas, reservas, consumos e idempotencia | No se puede reservar ni confirmar. Pedidos responde "Estamos confirmando tu pedido" y las devoluciones pendientes las reintenta Pedidos |
| RabbitMQ | Recibir referencias y publicar eventos | El outbox conserva los eventos hasta que vuelva. Si falta la referencia de una cocina, se rechazan los cambios que no se pueden autorizar |

## Cómo lo probamos

```bash
# Desde services/inventario: reglas, validaciones y permisos, sin base.
go test ./...

# Desde la raíz: MySQL y la API reales, en un proyecto de Docker descartable.
docker compose -p gk-inventario-check -f services/inventario/testdata/compose.yml up -d --build --wait inventario
docker compose -p gk-inventario-check -f services/inventario/testdata/compose.yml run --rm --build pruebas
docker compose -p gk-inventario-check -f services/inventario/testdata/compose.yml down -v
```

Sin `TEST_MYSQL=1` se saltean los tests contra MySQL (el servicio `pruebas` de ese compose ya la configura). Con MySQL real cubren: reserva todo o nada, claves repetidas y reutilizadas con otros datos, duplicados al mismo tiempo, ingredientes compartidos entre platos, ajustes después de cambiar la receta, vencimiento sin renovación, consumo y devolución repetidos, y permisos por cocina.

### 200 clientes por el último plato

`TestV8UltimoPlato200Solicitudes` arma una receta que necesita 2 unidades de un ingrediente y 1 de otro, con stock para **un solo plato**. Manda 200 solicitudes HTTP al mismo tiempo, con carritos y claves distintos, contra la API real y MySQL.

Exige **1 respuesta `201` y 199 `409 SIN_STOCK`**, sin errores técnicos. Después revisa MySQL: una sola reserva activa, reservado 2 y 1, disponible 998 y 0, y que lo reservado coincida con la suma del detalle. Para repetirla:

```bash
docker compose -p gk-ultimo-plato -f services/inventario/testdata/compose.yml run --rm --build pruebas go test -count=1 -v -run '^TestV8UltimoPlato200Solicitudes$' ./internal/adapters/mysql
docker compose -p gk-ultimo-plato -f services/inventario/testdata/compose.yml down -v --remove-orphans
```

En la primera corrida hubo un ganador pero 49 respuestas técnicas, por la espera entre transacciones que querían escribir un ingrediente que sobraba antes de ver que faltaba el último. Lo resolvimos con una lectura previa que descarta enseguida el stock que ya no alcanza; la reserva igual la decide el `UPDATE` condicionado de arriba. El detalle está en el [ADR-004](../../docs/adr/ADR-004-consistencia-y-concurrencia.md).
