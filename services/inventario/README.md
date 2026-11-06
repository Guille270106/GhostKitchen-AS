# Inventario

**Responsable:** Valentina Rey.

> **V5–V7 implementados:** base hexagonal, MySQL, ingredientes (HU19), recetas (HU18, RN18) y reservas todo o nada con idempotencia (RN04, RN05). El entrypoint aplica sus migraciones antes de arrancar HTTP. Ver [guía de migraciones](../../infra/mysql/README.md) y [ADR-004](../../docs/adr/ADR-004-consistencia-y-concurrencia.md).

> **V8:** prueba HTTP de 200 solicitudes sobre el último plato, con MySQL real. El vencimiento automático y sus eventos siguen siendo V9; el consumo y devolución son V10.

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

## V6 — Recetas de la cocina

| Método y ruta interna | Resultado |
| --- | --- |
| `PUT /cocina/platos/{id}/receta` | Cargar o reemplazar la receta completa del plato; devuelve 200 |
| `GET /cocina/platos/{id}/receta` | Consultar una receta propia; 404 si falta o pertenece a otra cocina |
| `GET /cocina/recetas` | Listar recetas de la cocina asignada, en `items` |

El PUT recibe ingredientes por **una unidad del plato**, en la unidad definida para cada ingrediente:

```json
{"items":[{"ingredienteId":"UUID-del-ingrediente","cantidad":200}]}
```

Exige una lista no vacía, ingredientes sin repetir y cantidades enteras mayores a cero. Campos extras, cantidades fraccionarias, nulas o ausentes producen 400. La cocina se obtiene de la misma identidad interna de V5: no se recibe en el cuerpo. Admin y otros roles reciben 403; platos o ingredientes ajenos, desconocidos o referencias inactivas reciben 404. No hay una verificación nueva de JWT: la integración de G9 sigue pendiente.

La migración **000003** agrega `plato_ref`, `receta` y `receta_item`. Las referencias de los 64 platos iniciales se copiaron del seed actual de Catálogo, calculando cocina y estado con su restaurante y cocina; `p64` permanece inactivo. Son referencias locales con versión 0: V12 incorporará la actualización por eventos. Un plato nuevo sin referencia local no admite receta. No se consulta la base ni la API de Catálogo durante una solicitud.

Guardar bloquea la referencia del plato y la cabecera de receta, valida ingredientes en orden de ID y reemplaza los ítems en una transacción. Si falla, se conservan todos los ítems y la versión anteriores. Las claves foráneas compuestas impiden mezclar cocinas también en MySQL. Cada guardado exitoso incrementa `version`; dos ediciones concurrentes se serializan y queda la última receta completa. Reenviar el mismo PUT conserva sus ingredientes, pero incrementa la versión: no se deduplica por Idempotency-Key. V6 no reserva ni modifica stock.

Las pruebas se ejecutan con los comandos de V5. Agregan validación HTTP, permisos, persistencia de recetas, rollback ante ingredientes ajenos o inexistentes, restricciones SQL y reemplazos concurrentes completos. El GET lee cada receta completa en una sola consulta, para no mezclar versiones.

## V7 — Reservas internas de Pedidos

| Método y ruta | Resultado |
| --- | --- |
| `POST /reservas` | Crear la reserva única del carrito; 201 y Location |
| `PATCH /reservas/{id}` | Reemplazar las cantidades finales de todos sus platos; 200 |
| `POST /reservas/{id}/liberar` | Liberar todos los ingredientes; 200. Acepta cuerpo vacío o `{}` |
| `GET /reservas/{id}` | Consultar la reserva actual, sin clave de escritura |

Son operaciones internas solicitadas por Pedidos, dentro de la red Docker y sin rutas en el gateway. Pedidos valida que el carrito sea del cliente antes de llamar. No usan las cabeceras del rol Cocina ni proporcionan autenticación entre servicios por sí solas; no deben exponerse directamente al frontend.

Las tres escrituras exigen `Idempotency-Key`, de 1 a 128 caracteres ASCII visibles, sin espacios. El POST recibe:

```json
{
  "carritoId":"carrito-123",
  "cocinaId":"nueva-cordoba",
  "items":[{"platoId":"p1","cantidad":2},{"platoId":"p2","cantidad":1}]
}
```

Los identificadores admiten hasta 64 letras ASCII, números, guiones y guiones bajos. Las cantidades son enteras positivas y no se repiten platos. El PATCH recibe solo `items` con el **contenido final completo**: omitir un plato libera sus ingredientes; `{"items":[]}` libera la reserva. Omitir `items` o enviar null es un error, no una orden de vaciar.

La respuesta incluye `id`, `carritoId`, `cocinaId`, `estado`, `venceEn` UTC e `items` con plato y cantidad. Usar el ID devuelto para ajustar, liberar o consultar. Un segundo POST con otra clave para el mismo carrito devuelve 409: no crea otra reserva, tampoco si la anterior fue liberada. Para empezar de nuevo se necesita otro carrito.

### Atomicidad, recetas e idempotencia

La migración **000004** incorpora `reserva`, `reserva_plato`, `reserva_item` e `idempotencia`. Una transacción bloquea la reserva, lee las recetas coherentes bajo bloqueo de cabecera, agrupa ingredientes compartidos y actualiza su stock en orden de ID, solo si alcanza. Ante un ingrediente insuficiente se deshace **todo** el intento, incluida la reserva nueva; un ajuste fallido conserva su contenido anterior. También se rechazan cantidades cuyo cálculo desborda BIGINT.

La primera reserva fija `venceEn` a 10 minutos con el reloj MySQL; ajustar no lo renueva. Los aumentos usan la receta actual solo para las unidades adicionales. Los lotes guardan cantidades y versión de receta: al reducir se liberan primero las últimas unidades agregadas; liberar usa el detalle efectivamente apartado. Cambiar la receta no modifica reservas existentes. Total no disminuye al reservar ni aumenta al liberar: solo cambia reservado.

Cada clave se limita a una operación de Pedidos (reservar, ajustar o liberar), y su huella incluye recurso y datos normalizados. Reordenar los platos no crea una operación distinta. Con la misma clave y datos se devuelve el resultado original, incluso si después la reserva cambió; consultar GET para su estado actual. Reutilizar una clave con otros datos produce `IDEMPOTENCIA_CONFLICTO` (409).

Efectos y resultado se guardan juntos; los rechazos de negocio, como `SIN_STOCK`, también quedan registrados sin cambios de stock. Para intentar otra vez tras reponer ingredientes se necesita una nueva clave. Fallos técnicos hacen rollback y permiten repetir la misma clave. Los duplicados simultáneos esperan el resultado; si vence el plazo al adquirir la clave, se informa `OPERACION_EN_PROCESO` (409, `Retry-After: 1`). Ante deadlock se reintenta hasta dos veces la transacción completa con la misma clave y dentro del contexto disponible. No se registra un éxito antes del commit.

Otros resultados: `SIN_STOCK`, `RESERVA_VENCIDA` y `RESERVA_CONFLICTO` dan 409; referencias de cocina/plato desconocidas, inactivas o de otra cocina y platos sin receta dan 404; solicitudes inválidas dan 400. Una reserva liberada no se reactiva. Liberarla otra vez no vuelve a modificar stock.

### Límite de vencimiento de V7

Al alcanzar el plazo, GET informa VENCIDA y se rechazan aumentos o ajustes no vacíos. Se permite liberarla explícitamente. **El barrido que libera automáticamente el stock y publica `reserva.vencida` queda para V9**: en V7 el stock de una reserva vencida permanece apartado hasta su liberación explícita. GET no modifica datos ni publica eventos. No hay consumo, devolución, outbox ni publicador todavía.

Las pruebas existentes ahora cubren rollback, claves reutilizadas, replay durable, duplicados simultáneos, ingredientes compartidos, ajustes tras cambiar recetas, vencimiento sin renovación y liberación repetida. No sustituyen el escenario de 200 solicitudes de V8. Se ejecutan con los mismos comandos de pruebas de arriba.

## V8 — Prueba del último plato con 200 solicitudes

`TestV8UltimoPlato200Solicitudes` prepara una cocina aislada y una receta que necesita 2 unidades de un ingrediente y 1 de otro. Hay stock total 1000 y 1, respectivamente: alcanza para un solo plato. Las 200 solicitudes HTTP usan carritos y claves distintos y esperan una barrera común de salida; se ejecutan contra la API real y sus transacciones MySQL, sin repositorios de memoria ni reintentos HTTP en el cliente de prueba.

La prueba exige exactamente **1 respuesta 201 y 199 respuestas 409 SIN_STOCK**, sin errores técnicos. Después consulta MySQL y exige una reserva ACTIVA del ganador, un plato con cantidad 1, dos detalles de ingredientes con cantidades 2 y 1, stock total sin cambios, reservado 2 y 1, disponible 998 y 0, ningún reservado inválido y 200 resultados de idempotencia durables. Comprueba además que la suma del detalle reservado coincida con el stock apartado. Un timeout o una respuesta inesperada hace fallar la prueba.

Para repetirla desde la raíz del repositorio, con Docker en ejecución:

```powershell
docker compose -p gk-v8-check -f services/inventario/testdata/compose.yml run --rm --build pruebas go test -count=1 -v -run '^TestV8UltimoPlato200Solicitudes$' ./internal/adapters/mysql

# Al terminar, eliminar solo los contenedores y el volumen de este entorno descartable.
docker compose -p gk-v8-check -f services/inventario/testdata/compose.yml down -v --remove-orphans
```

No usa `.env` ni los datos de desarrollo. El test elimina sus registros después de inspeccionarlos y muestra el resultado en la salida de Go. Sin `TEST_MYSQL=1` se omite: un `go test` local sin MySQL no demuestra V8. El servicio `pruebas` de ese Compose ya configura la variable.

Durante V8 se incorporó una lectura previa para descartar stock ya insuficiente sin esperar a escribir otros ingredientes. Es una optimización: las escrituras siguen usando la condición atómica de stock y la transacción completa; esa lectura nunca autoriza por sí sola una reserva. No se amplió el plazo HTTP de 3 segundos.

La duración registrada describe una ejecución local con una instancia de Inventario; no es una medición de capacidad en producción. Esta prueba comprueba la reserva sin sobreventa; no ejecuta creación de pedidos, consumo ni devolución de stock. La evidencia de ejecución se registra en el [ADR-004](../../docs/adr/ADR-004-consistencia-y-concurrencia.md).

## Diseño de las funciones restantes

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
