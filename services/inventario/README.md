# Inventario

**Responsable:** Valentina Rey.

> **V5 implementado:** servicio base hexagonal, MySQL e ingredientes (HU19). El entrypoint aplica sus migraciones antes de arrancar HTTP. Ver [guía de migraciones](../../infra/mysql/README.md) y [ADR-004](../../docs/adr/ADR-004-consistencia-y-concurrencia.md).

> Recetas, reservas, movimientos y mensajería siguen siendo diseño previsto para V6 y siguientes. No están implementados en V5.

Dice **con qué se hace cada plato y si alcanza**. Puerto interno **8084**.

## Usar V5

Desde la raíz del repositorio, con las contraseñas de MySQL configuradas en `.env`:

```powershell
docker compose up -d --build inventario
docker compose logs inventario
docker compose exec inventario wget -qO- http://127.0.0.1:8084/ready
```

`/health` comprueba que el proceso está vivo; `/ready` comprueba acceso a la tabla de ingredientes en MySQL. Un fallo de migración impide arrancar HTTP. El servicio no publica puertos al host. La migración 000002 crea `ingrediente` y las referencias de las cuatro cocinas iniciales de Catálogo, con versión 0; V12 actualizará esas referencias por eventos. Una referencia ausente o inactiva impide crear ingredientes.

### Operaciones disponibles

| Método y ruta interna | Resultado |
| --- | --- |
| `POST /cocina/ingredientes` | Crear ingrediente; 201 y Location del recurso |
| `GET /cocina/ingredientes` | Listar únicamente ingredientes de la cocina asignada, en `items` |
| `GET /cocina/ingredientes/{id}` | Consultar un ingrediente propio; 404 si no existe o pertenece a otra cocina |

El POST recibe un único objeto JSON:

```json
{"nombre":"Harina","unidad":"gramos","stockInicial":1000}
```

Nombre y unidad son obligatorios (máximo 120 y 30 caracteres); el stock inicial es obligatorio, entero y no negativo. No admite `cocinaId`, `reservado` ni otros campos extra: la cocina se toma de la identidad. El ingrediente nace con reservado 0 y disponible igual al total. MySQL también impide stock negativo o reservado mayor al total. El POST todavía no tiene deduplicación: cada alta crea un ingrediente nuevo.

### Límite de autenticación de V5

El adaptador HTTP consume las cabeceras internas `X-Usuario-Id`, `X-Rol: cocina` y `X-Cocina-Id`. Sin identidad devuelve 401; otros roles, incluido Admin, reciben 403. **Estas cabeceras no prueban por sí solas un login.** Se usan en pruebas internas mientras Guille completa G9. Antes de habilitar acceso desde el frontend, el gateway debe verificar el JWT, eliminar cabeceras de identidad del cliente e inyectar las verificadas. V5 no incorpora rutas públicas ni un login alternativo.

Ejemplo solo dentro del contenedor para probar la capa interna:

```powershell
docker compose exec inventario wget -qO- --header="Content-Type: application/json" --header="X-Usuario-Id: prueba-interna" --header="X-Rol: cocina" --header="X-Cocina-Id: cerro" --post-data='{"nombre":"Harina","unidad":"gramos","stockInicial":1000}' http://127.0.0.1:8084/cocina/ingredientes
```

### Configuración y pruebas de V5

`PORT=8084`, `DB_HOST=mysql`, `DB_PORT=3306`, `DB_USER=inventario`, `DB_NAME=inventario_db` y `DB_PASSWORD` (obligatoria). En Compose, esta última sale de `INVENTARIO_DB_PASSWORD`. El runner recibe las mismas variables y usa `/migrations`.

```powershell
# Desde services/inventario: pruebas de validación, permisos y aislamiento HTTP.
go test ./...

# Desde la raíz: MySQL y API reales en un proyecto descartable.
docker compose -p gk-v5-check -f services/inventario/testdata/compose.yml up -d --build --wait inventario
docker compose -p gk-v5-check -f services/inventario/testdata/compose.yml run --rm --build pruebas
# Eliminar solo los recursos y datos descartables de esa prueba.
docker compose -p gk-v5-check -f services/inventario/testdata/compose.yml down -v
```

La prueba MySQL verifica persistencia, consulta por cocina, rechazo de referencias desconocidas/inactivas y restricciones de stock. Sin `TEST_MYSQL=1` se omite esa prueba; las pruebas HTTP usan un repositorio de memoria como doble, no como almacenamiento del servicio.

## Qué hace

- Guarda los **ingredientes** de cada cocina con su stock: cuánto hay en total, cuánto está reservado y cuánto queda disponible (`total − reservado`, que nunca puede ser negativo).
- Guarda las **recetas**: qué ingredientes y cuántos usa cada plato. Solo se pueden usar ingredientes de la cocina del plato.
- **Reserva** los ingredientes cuando el cliente agrega un plato: **todos o ninguno**, por 10 minutos.
- Ajusta o libera la reserva cuando el cliente cambia el carrito, y la **libera sola** cuando pasan los 10 minutos.
- **Descuenta** el stock cuando se confirma el pedido y lo **devuelve** si se cancela.
- El **rol Cocina** administra ingredientes, recetas, reposiciones, mermas y stock solo de su cocina asignada. El admin no administra el inventario.
- Avisa cuando un plato pasa a estar disponible o deja de estarlo.

## Qué datos guarda (MySQL)

| Tabla | Qué guarda |
| --- | --- |
| `ingrediente` | Cocina, nombre, unidad, total y reservado |
| `receta_item` | Qué ingrediente y cuánto usa cada plato |
| `reserva` / `reserva_item` | Cada reserva, su estado, cuándo vence y cuánto aparta de cada ingrediente |
| `movimiento` | El historial: reposiciones, mermas, descuentos y devoluciones |
| `cocina_ref` / `plato_ref` | Una copia, que llega por mensajes, de la identidad y estado de cada cocina y de la cocina de cada plato |
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
| `GET`/`POST /cocina/ingredientes` | cocina | Ver el stock y cargar ingredientes |
| `POST /cocina/ingredientes/{id}/reposiciones` · `/mermas` | cocina | Reponer o registrar mermas |
| `PUT /cocina/platos/{id}/receta` | cocina | Cargar la receta de un plato |
| `GET /cocina/recetas` | cocina | Ver las recetas de su cocina |
| `GET /platos/disponibilidad` | Búsqueda | Reconstruir la búsqueda |

**Permisos:** solo Cocina administra inventario y recetas de su cocina asignada en el token verificado. Inventario comprueba la pertenencia del ingrediente o plato, sin confiar en una cocina enviada por el cliente. Admin recibe 403; recursos de otra cocina, 404. Las reservas y consumos son operaciones internas solicitadas por Pedidos.

## Mensajes

| Manda | Quién lo escucha | Para qué |
| --- | --- | --- |
| `disponibilidad.cambiada` | Búsqueda | Mostrar si un plato se puede pedir |
| `reserva.vencida` | Pedidos | Marcar el carrito como vencido |

| Escucha | Para qué |
| --- | --- |
| `cocina.*` | Conocer identidad y estado de cada cocina |
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

La conexión MySQL y el puerto de V5 están definidos arriba. `INVENTARIO_DB_PASSWORD` y las credenciales comunes se describen en el [README principal](../../README.md#variables-del-env). La reserva de 10 minutos, el barrido cada 15 segundos y RabbitMQ se incorporan en sus tareas posteriores.

## Pruebas previstas

- Admin rechazado con 403; Cocina sin acceso a ingredientes, movimientos o recetas ajenos (404), aunque manipule IDs o headers.
- Recetas con ingredientes de otra cocina, cantidades inválidas y mermas que comprometen reservas.
- Reservas simultáneas del último plato contra MySQL real: ninguna sobreventa y ninguna reserva parcial.
- Descuento y vencimiento concurrentes; repetir reservas, descuentos y devoluciones con la misma clave.
- Cambiar la receta después de reservar o descontar no altera las cantidades que deben consumirse o devolverse.
