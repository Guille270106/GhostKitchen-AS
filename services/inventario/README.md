# Inventario

**Responsable:** Valentina Rey.

> Entrega 1: diseño preliminar; todavía no hay implementación. Las carpetas, rutas, configuración y pruebas indicadas son previstas para el desarrollo desde la Entrega 2.

Dice **con qué se hace cada plato y si alcanza**. Puerto **8084**. Es uno de los dos servicios donde no se puede equivocar nada. Lo programamos en la Entrega 2.

## Qué hace

- Guarda los **ingredientes** de cada cocina con su stock: cuánto hay en total, cuánto está reservado y cuánto queda disponible (`total − reservado`, que nunca puede ser negativo).
- Guarda las **recetas**: qué ingredientes y cuántos usa cada plato. Solo se pueden usar ingredientes de la cocina del plato.
- **Reserva** los ingredientes cuando el cliente agrega un plato: **todos o ninguno**, por 10 minutos.
- Ajusta o libera la reserva cuando el cliente cambia el carrito, y la **libera sola** cuando pasan los 10 minutos.
- **Descuenta** el stock cuando se confirma el pedido y lo **devuelve** si se cancela.
- Permite **reponer** stock y registrar **mermas** (ingredientes que se perdieron o vencieron).
- Avisa cuando un plato pasa a estar disponible o deja de estarlo.

## Qué datos guarda (MySQL)

| Tabla | Qué guarda |
| --- | --- |
| `ingrediente` | Cocina, nombre, unidad, total y reservado |
| `receta_item` | Qué ingrediente y cuánto usa cada plato |
| `reserva` / `reserva_item` | Cada reserva, su estado, cuándo vence y cuánto aparta de cada ingrediente |
| `movimiento` | El historial: reposiciones, mermas, descuentos y devoluciones |
| `cocina_ref` / `plato_ref` | Una copia, que llega por mensajes, de la localidad de cada cocina y de la cocina de cada plato |
| `idempotencia` · `outbox` · `mensaje_procesado` | Para no repetir operaciones, no perder mensajes y no procesar dos veces el mismo mensaje |

## Cómo se organiza por dentro: hexagonal liviano

Las reglas del stock quedan **en el centro** y no dependen de la base de datos, de Gin ni de RabbitMQ. El centro dice lo que necesita (por ejemplo, "guardar una reserva") y por afuera hay **adaptadores** que lo hacen con MySQL, HTTP o RabbitMQ. Así podemos testear las reglas sin levantar una base (Clase 8). Es "liviano" porque no usamos todos los patrones de DDD.

```
cmd/api/                    arranca el servicio
internal/domain/            las reglas: disponible ≥ 0, la receta usa ingredientes de la misma cocina, estados de la reserva
internal/application/       las acciones: Reservar, Ajustar, Liberar, Descontar, Devolver, Reponer, RegistrarMerma, CargarReceta
internal/ports/             lo que el centro ofrece y lo que necesita
internal/adapters/http/     rutas HTTP
internal/adapters/mysql/    acceso a MySQL
internal/adapters/rabbitmq/ envío y lectura de mensajes
internal/adapters/catalogo/ convierte los mensajes de Catálogo en la copia local
internal/adapters/jobs/     libera las reservas vencidas cada 15 segundos
migrations/                 tablas
```

**Cómo reserva "todo o nada":** en una sola transacción, para cada ingrediente de la receta:

```sql
UPDATE ingrediente SET reservado = reservado + :n
WHERE id = :id AND total - reservado >= :n;
-- si no cambió ninguna fila, no alcanza: se deshace todo
```

La base solo suma si alcanza. Por eso, si dos personas quieren el último plato a la vez, solo una lo consigue.

## Rutas

Salvo las rutas técnicas comunes, la tabla muestra rutas internas del servicio. El frontend entra por `/api/v1` a través del gateway; las rutas entre servicios no se publican al exterior.

| Ruta | Quién la usa | Para qué |
| --- | --- | --- |
| `POST /reservas` · `PATCH /reservas/{id}` · `POST /reservas/{id}/liberar` | Pedidos | Reservar, cambiar o liberar (con clave para no repetir) |
| `POST /reservas/{id}/consumir` | Pedidos | Descontar el stock al confirmar |
| `POST /consumos/{pedidoId}/devolver` | Pedidos | Devolver el stock al cancelar |
| `GET /reservas/{id}` · `GET /consumos/{pedidoId}` | Pedidos | Preguntar si algo se hizo, cuando no hubo respuesta a tiempo |
| `GET`/`POST /admin/cocinas/{id}/ingredientes` | admin | Ver el stock y cargar ingredientes |
| `POST /admin/ingredientes/{id}/reposiciones` · `/mermas` | admin | Reponer o registrar mermas |
| `PUT /admin/platos/{id}/receta` | admin | Cargar la receta de un plato |
| `GET /cocina/recetas` | cocina | Ver las recetas de su cocina |
| `GET /platos/disponibilidad` | Búsqueda | Reconstruir la búsqueda |

**Permisos:** el admin solo toca cocinas de su localidad, y una cocina solo ve sus propias recetas.

## Mensajes

| Manda | Quién lo escucha | Para qué |
| --- | --- | --- |
| `disponibilidad.cambiada` | Búsqueda | Mostrar si un plato se puede pedir |
| `reserva.vencida` | Pedidos | Marcar el carrito como vencido |

| Escucha | Para qué |
| --- | --- |
| `cocina.*` | Saber la localidad de cada cocina |
| `plato.*` | Saber de qué cocina es cada plato y si sigue activo |

## Con quién habla

**No llama a ningún otro servicio.** Lo hicimos así a propósito: lo más importante del sistema no depende de que otro servicio esté andando.

## Dependencias y fallas previstas

| Dependencia | Para qué | Si falla |
| --- | --- | --- |
| MySQL (`inventario_db`) | Stock, recetas, reservas, movimientos e idempotencia | No se puede reservar ni confirmar con resultado seguro; las devoluciones pendientes se recuperan desde Pedidos |
| RabbitMQ | Recibir referencias y publicar eventos | El outbox conserva los cambios; si falta la referencia de una cocina, se rechazan cambios que no se puedan autorizar |

El plazo de 10 minutos empieza con la primera reserva del carrito: agregar platos o cambiar cantidades no lo reinicia. El barrido cada 15 s libera reservas vencidas, y la confirmación comprueba el vencimiento aunque el barrido todavía no las haya procesado. Descontar y vencer bloquean la misma reserva; solo una operación tiene efecto.

Las operaciones internas que modifican reservas o consumos reciben `Idempotency-Key` y guardan la clave junto con el resultado en la transacción. Una repetición con la misma clave y datos devuelve el resultado previo; reutilizarla con otros datos se rechaza. Las consultas `GET` permiten recuperar un resultado incierto y no necesitan una clave de escritura.

Una devolución usa las cantidades efectivamente descontadas, no una receta que pudo cambiar después. La merma no puede dejar el total por debajo de lo reservado. Mensajes y cambios de stock se guardan juntos en el outbox; la deduplicación del consumidor se persiste con su efecto ([ADR-005](../../docs/adr/ADR-005-comunicacion-entre-servicios.md)).

## Configuración prevista

`INVENTARIO_DB_PASSWORD` y las credenciales comunes de MySQL/RabbitMQ están anticipadas en [`.env.example`](../../.env.example). La conexión, el puerto, la duración de reserva (10 min) y el intervalo del barrido (15 s) se definirán al implementar.

## Pruebas previstas

- Recetas con ingredientes de otra cocina, cantidades inválidas y mermas que comprometen reservas.
- Reservas simultáneas del último plato contra MySQL real: ninguna sobreventa y ninguna reserva parcial.
- Descuento y vencimiento concurrentes; repetir reservas, descuentos y devoluciones con la misma clave.
- Cambiar la receta después de reservar o descontar no altera las cantidades que deben consumirse o devolverse.
