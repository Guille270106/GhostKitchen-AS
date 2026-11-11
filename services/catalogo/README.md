# Catálogo

**Responsable:** Guillermina Sánchez.

Es la **fuente de verdad** de cocinas, restaurantes y platos, sobre **MongoDB** (`catalogo_db`, con usuario propio). Al arrancar carga los datos iniciales que falten (4 cocinas, 16 restaurantes, 64 platos) sin pisar cambios. Atiende las lecturas del frontend y la lectura interna paginada con la que **Búsqueda arma su índice**, así que lo que devuelve la API publicada sale de esta base. El admin crea y modifica cocinas y restaurantes de su localidad, y la cuenta Cocina crea y cambia sus platos. Tiene caché con Memcached, corre en 2 copias y avisa cada cambio por mensajes (outbox).

## Flujo de los datos

```
MongoDB (catalogo_db) ──> Catálogo ──GET /internal/cocinas, /internal/platos──> Búsqueda ──> Solr ──> /public/v1 (otro grupo)
                              └──────── GET /api/v1/cocinas, /api/v1/platos/{id} (frontend, por el gateway)
```

Búsqueda reconstruye el índice al arrancar y cuando se le pide `POST /internal/reindexar`. Además, cada alta o cambio de una cocina o un plato se guarda en el outbox de Catálogo junto con el dato y sale por RabbitMQ (`cocina.*`, `plato.*`): Búsqueda e Inventario lo aplican solos, en segundos.

**Probar el recorrido de un cambio** (con todo levantado). Se cambia el precio como lo hace la cocina desde "Menú", no tocando MongoDB a mano: así pasa por las reglas, la versión del plato, el outbox y la invalidación de la caché. En Git Bash:

```bash
# 1. Entrar como la cocina de Nueva Córdoba (dueña del plato p1)
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login -H "Content-Type: application/json" \
  -d '{"email":"cocina.nueva-cordoba@ghostkitchen.dev","password":"<contraseña de la cocina>"}' | sed -E 's/.*"token":"([^"]+)".*/\1/')
# 2. Cambiar el precio
curl -X PATCH http://localhost:8080/api/v1/cocina/platos/p1 -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" -d '{"precio":15000}'
# 3. Catálogo lo muestra enseguida (la caché se invalida) y la búsqueda en menos de 5 s
curl http://localhost:8080/api/v1/platos/p1
curl http://localhost:8080/api/v1/busqueda/platos/p1
```

También se puede mirar la base con MongoDB Compass en `mongodb://root:<MONGO_ROOT_PASSWORD>@localhost:27017/?directConnection=true`. El `directConnection=true` hace falta porque MongoDB corre como replica set: sin él, Compass le pregunta al replica set por sus miembros, recibe `mongo:27017` (el nombre dentro de Docker) y no lo encuentra desde la compu.

Dice **qué se vende y a qué precio**. Puerto **8082**, con **2 copias** para repartir la carga. Lo programamos en la Entrega 2.

## Qué hace

- Maneja las **cocinas**: el admin las crea, modifica o desactiva (solo las de su localidad).
- Maneja los **restaurantes** de cada cocina: **máximo 4 por cocina** y sin repetir.
- El **rol Cocina** crea, modifica y desactiva los **platos** de sus restaurantes: nombre, descripción, categoría, etiquetas (vegano, sin TACC…), precio, foto opcional y si está activo. Admin no administra el menú.
- Cada vez que cambia algo, manda un mensaje para que Búsqueda e Inventario se enteren.

**No** guarda recetas ni stock: eso lo maneja [Inventario](../inventario/README.md).

## Escrituras del admin

El gateway las publica en `/api/v1/admin/cocinas` y verifica el token antes. Catálogo lee quién es de `X-Usuario-Id`, `X-Rol` y `X-Localidad` (que llega codificada para URL) y solo deja pasar a un **admin**, y solo sobre cocinas **de su localidad**.

| Ruta | Para qué | Respuestas |
| --- | --- | --- |
| `GET /admin/cocinas` | Las cocinas de su localidad, activas o no, con todos sus restaurantes | `200` |
| `POST /admin/cocinas` | Crear una cocina. La localidad no se manda: es la del admin. El id sale del nombre ("Cocina Güemes" → `guemes`) | `201` · `400` · `409 COCINA_REPETIDA` |
| `PATCH /admin/cocinas/{id}` | Cambiar nombre, dirección, ubicación, tiempos, costo de envío o `activa` (desactivar en lugar de borrar) | `200` · `400` · `403 OTRA_LOCALIDAD` · `404` |
| `POST /admin/cocinas/{id}/restaurantes` | Sumar un restaurante (`{"nombre": "..."}`) | `201` · `409 RESTAURANTE_REPETIDO` · `409 COCINA_LLENA` |
| `PATCH /admin/cocinas/{id}/restaurantes/{rid}` | Renombrar, desactivar o reactivar un restaurante | `200` · `404` · `409 RESTAURANTE_REPETIDO` |

Sin identidad, `401`; con otro rol, `403 SIN_PERMISO`. Un campo de más en el cuerpo (por ejemplo `localidad`) da `400`.

**Máximo 4 restaurantes, sin repetir.** El nombre se compara sin mayúsculas ni acentos ("Café Martínez" = "cafe martinez"). Un restaurante desactivado no se borra, así que sigue ocupando su lugar y su nombre.

**Dos admins al mismo tiempo: concurrencia optimista.** Cada cocina tiene un número de `version`. Para cambiarla, el servicio la lee, aplica las reglas del dominio y la guarda solo si la versión sigue siendo la que leyó (`ReplaceOne` con `_id` y `version` en el filtro, que en MongoDB es una sola operación atómica sobre el documento). Si otra solicitud la cambió en el medio, no se guarda nada y el servicio vuelve a empezar con la versión nueva (hasta 3 veces; después, `409 CONFLICTO`). Así las reglas siempre se controlan sobre lo último guardado: con 10 solicitudes a la vez sobre una cocina con 2 restaurantes, entran exactamente 2 (`TestNoSePasaDeCuatroConSolicitudesALaVez`). La misma `version` es la que usará Búsqueda para no guardar un dato viejo encima de uno nuevo.

Cada cambio sale como mensaje (ver "Mensajes") y Búsqueda lo aplica a su índice.

## Platos de la cocina

La cuenta Cocina maneja los platos de **su** cocina (la de `X-Cocina-Id`, que pone el gateway; nunca la del cuerpo). El gateway las publica en `/api/v1/cocina/...`.

| Ruta | Para qué | Respuestas |
| --- | --- | --- |
| `GET /cocina/platos` | Todos los platos de su cocina, activos o no | `200` |
| `POST /cocina/restaurantes/{rid}/platos` | Crear un plato en un restaurante activo de su cocina: nombre, descripción, categoría, precio, etiquetas y foto | `201` · `400` · `404` |
| `PATCH /cocina/platos/{id}` | Cambiar sus datos, desactivarlo o activarlo | `200` · `400` · `403 OTRA_COCINA` · `404` · `409 SIN_RECETA` · `503` |

**Un plato sin receta no se publica.** Por eso un plato nuevo arranca **desactivado**, aunque se pida activo. Para activarlo, Catálogo le pregunta a Inventario si tiene receta (`GET /cocina/platos/{id}/receta`, con la identidad de la cocina; 800 ms y un reintento si no hubo respuesta o fue 5xx). Si no tiene, `409 SIN_RECETA`; si Inventario no contesta, `503 INVENTARIO_NO_DISPONIBLE`, sin tomarlo como "no tiene receta". Cambiar el precio o desactivar no consulta a Inventario.

Las etiquetas válidas son Vegano, Vegetariano, Sin TACC y Picante (las mismas que usa Búsqueda). El precio de un plato que ya está en un carrito no cambia: Pedidos guarda el precio al agregarlo. Mismo control de concurrencia que las cocinas: cada plato tiene su `version`.

Catálogo publica `plato.creado`, así Inventario se entera del plato nuevo: la cocina le carga la receta y recién ahí lo puede activar.

## Mensajes: outbox y publicador

Cada escritura (cocina, restaurante o plato) guarda su evento en la colección **`outbox`** en la **misma transacción** que el cambio. Por eso MongoDB corre como replica set: o se guardan los dos, o ninguno. Si el cambio se guardó, su mensaje también, aunque RabbitMQ esté caído; y si la escritura choca con otra (versión vieja), no queda ni el cambio ni el evento.

| Cambio | Evento | `agregado_id` · `secuencia` |
| --- | --- | --- |
| Alta de cocina | `cocina.creada` | id · `version` de la cocina |
| Cambio de datos o de restaurantes, o reactivarla | `cocina.actualizada` | |
| Desactivarla | `cocina.desactivada` | |
| Alta de plato | `plato.creado` | id · `version` del plato |
| Cambio, o activarlo | `plato.actualizado` | |
| Desactivarlo | `plato.desactivado` | |

`datos` lleva la cocina o el plato completo, como `/internal/cocinas` y `/internal/platos`. El sobre sigue [`docs/contracts/eventos`](../../docs/contracts/eventos/README.md), con el `correlation_id` de la solicitud que hizo el cambio.

**El publicador** corre adentro de Catálogo (`internal/mensajeria`). Cada 500 ms lee los pendientes del más viejo al más nuevo y los manda al exchange `ghostkitchen.events` con el tipo como clave de ruteo (los reciben `busqueda.indexador` e `inventario.referencias`). Usa **publisher confirms**: marca un evento como publicado recién cuando RabbitMQ confirma que lo recibió. Si RabbitMQ no confirma o está caído, el evento sigue pendiente, el publicador cierra la conexión y reintenta esperando cada vez más (1 s, 2 s, 4 s… hasta 30 s), sin saltearse ninguno. Si se publica dos veces (por ejemplo, se cayó justo antes de marcarlo), el consumidor lo descarta por su `evento_id`: es entrega "al menos una vez".

Los eventos publicados se borran solos a los 7 días (índice TTL sobre `publicadoEn`).

Para mirar el outbox:

```bash
docker compose exec mongo sh -c 'mongosh --quiet -u root -p "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin catalogo_db --eval "db.outbox.find({}, {tipo:1, agregadoId:1, publicadoEn:1}).sort({creadoEn:-1}).limit(5)"'
```

Y los mensajes que llegaron, en el panel de RabbitMQ (`http://localhost:15672`, colas `busqueda.indexador` e `inventario.referencias`).

## Caché

Las lecturas públicas (cocinas, una cocina, el menú y el detalle de un plato) pasan por **Memcached**, compartido por las copias de Catálogo. Patrón cache-aside con un decorator del repositorio, TTL de 60 s ±10 % e **invalidación por generación**: cada escritura hace `incr` sobre `catalogo:v1:gen`, y las claves llevan la generación en el nombre. Después de un cambio guardado no se sirve el dato viejo. Si Memcached no responde en 50 ms, se lee de MongoDB y se sigue. El detalle y por qué está en el [ADR-007](../../docs/adr/ADR-007-cache.md).

Para medir con y sin caché: `CACHE_HABILITADA=false` en el `.env` y `docker compose up -d catalogo`.

## Dos copias

Catálogo corre como `catalogo-1` y `catalogo-2`, detrás del gateway y con la misma MongoDB y la misma Memcached ([ADR-012](../../docs/adr/ADR-012-balanceo-de-carga.md)). Cada respuesta lleva `X-Instancia` con la copia que la atendió. Solo `catalogo-1` publica el outbox (`PUBLICAR_OUTBOX`).

Para ver el reparto y la caída de una copia:

```bash
curl -s -D - -o /dev/null http://localhost:8080/api/v1/cocinas | grep -i x-instancia   # repetir: alterna 1 y 2
docker compose stop catalogo-1                                                         # siguen todas con 200, desde catalogo-2
docker compose start catalogo-1                                                        # a los 10 s vuelve a alternar
```

## Qué datos guarda (MongoDB)

| Colección | Qué guarda |
| --- | --- |
| `cocinas` | Localidad, dirección, ubicación, tiempos estimados de entrega, costo de envío, si está activa y sus restaurantes adentro (máximo 4) |
| `platos` | A qué cocina y restaurante pertenece, nombre, descripción, categoría, etiquetas, precio, foto y si está activo |
| `outbox` | Los mensajes que todavía hay que mandar a RabbitMQ |

**Caché:** guardamos en Memcached las consultas más comunes ("cocinas con sus restaurantes" y "menú de una cocina") durante 60 segundos. Si Admin cambia cocinas/restaurantes o Cocina cambia platos propios, se invalida la caché correspondiente. Las 2 copias del servicio comparten la misma caché.

Las lecturas usadas para autorizar cuentas o validar un plato al reservar se harán directamente contra MongoDB, evitando una caché obsoleta. D7 definirá cómo prevenir que una lectura concurrente vuelva a insertar un dato viejo después de invalidarlo, y cómo verificarlo entre ambas instancias.

## Cómo se organiza por dentro: capas + caché

Usamos capas (handler → servicio → repositorio). La caché es una capa que **envuelve al repositorio** (patrón *decorator*): el servicio pide los datos igual que siempre y no sabe si vienen de la caché o de la base. Así podemos prender y apagar la caché para medir cuánto mejora.

```
cmd/api/               arranca el servicio
internal/handlers/     rutas HTTP
internal/services/     reglas: localidad del admin, cocina asignada para platos, máximo 4 restaurantes
internal/repositories/ acceso a MongoDB
internal/cache/        la caché que envuelve al repositorio
internal/domain/       qué es una cocina, un restaurante y un plato
internal/events/       outbox y envío de mensajes
```

## Rutas

Salvo las rutas técnicas comunes, la tabla muestra rutas internas del servicio. El frontend entra por `/api/v1` a través del gateway; las rutas entre servicios no se publican al exterior.

| Ruta | Quién la usa | Para qué |
| --- | --- | --- |
| `GET /cocinas` · `GET /cocinas/{id}` | cualquiera y otros servicios | Ver las cocinas con sus restaurantes |
| `GET /platos/{id}` | cualquiera y Pedidos | Ver un plato (Pedidos lo usa para el precio) |
| `GET /cocinas/{id}/platos` | frontend y Búsqueda | El menú de una cocina; se usa también para navegar si Búsqueda falla |
| `POST /admin/cocinas` · `PATCH /admin/cocinas/{id}` | admin | Crear o cambiar cocinas |
| `POST /admin/cocinas/{id}/restaurantes` · `PATCH .../restaurantes/{id}` | admin | Crear o cambiar restaurantes |
| `POST /cocina/restaurantes/{restauranteId}/platos` · `PATCH /cocina/platos/{id}` | cocina | Crear, modificar o desactivar platos propios (activo=false; no borrar) |

La reconstrucción dispone de lectura paginada de todas las cocinas y platos; no puede limitarse a una página de resultados. Las lecturas internas para Usuarios y Pedidos evitan la caché. Las rutas exactas y sus parámetros se terminarán de definir al implementar.

**Permisos:** Admin administra cocinas y restaurantes de su localidad, no platos. Cocina opera menú de la cocina asignada en su token; Catálogo valida pertenencia de restaurante y plato. Rol incorrecto: 403; recurso ajeno: 404. El cliente no puede elegir una cocina arbitraria en el cuerpo para autorizarse. Recetas siguen en Inventario con el mismo rol Cocina.

## Mensajes que manda

| Mensaje | Quién lo escucha |
| --- | --- |
| `cocina.*` (se creó, cambió o se desactivó una cocina) | Búsqueda e Inventario |
| `plato.*` (se creó, cambió o se desactivó un plato) | Búsqueda e Inventario |

No escucha mensajes ni llama a otros servicios.

## Dependencias y fallas

| Dependencia | Para qué | Si falla |
| --- | --- | --- |
| MongoDB (`catalogo_db`) | Datos comerciales y outbox | No se puede modificar ni validar el catálogo contra la fuente; las lecturas ya indexadas en Búsqueda siguen disponibles |
| Memcached | Lecturas frecuentes | Se registra la falla y se consulta MongoDB; la caché no es necesaria para operar |
| RabbitMQ | Publicar cambios | Los eventos permanecen en el outbox y se publican al recuperarse; Búsqueda e Inventario pueden quedar atrasados |

MongoDB debe correr como **replica set**: el cambio comercial y el evento del outbox se guardan en una misma transacción. El publicador marca el evento como enviado después de la confirmación del broker; los consumidores toleran publicaciones duplicadas ([ADR-005](../../docs/adr/ADR-005-comunicacion-entre-servicios.md)).

## Configuración

| Variable | Por defecto | Para qué |
| --- | --- | --- |
| `PORT` | `8082` | Puerto HTTP |
| `MONGO_URI` | `mongodb://mongo:27017` | Conexión a MongoDB. En Docker Compose usa el usuario `catalogo` y `CATALOGO_DB_PASSWORD` de `.env` |
| `MONGO_DB` | `catalogo_db` | Base del servicio |
| `MONGO_TIMEOUT_MS` | `2000` | Tiempo máximo de cada operación contra MongoDB |
| `INVENTARIO_URL` | `http://inventario:8084` | Para saber si un plato tiene receta antes de activarlo |
| `RABBITMQ_URL` | (vacía) | Donde se publican los eventos. Sin ella, los eventos quedan en el outbox |
| `CACHE_HABILITADA` | `true` | Prende o apaga la caché |
| `MEMCACHED_ADDR` | `memcached:11211` | Dónde está Memcached |
| `INSTANCIA` | el nombre del host | Qué copia es (`catalogo-1`, `catalogo-2`) |
| `PUBLICAR_OUTBOX` | `true` | Si esta copia publica el outbox (solo una) |
| `SEED_AL_INICIAR` | `true` | Insertar los datos iniciales que falten (no pisa los existentes) |

El usuario `catalogo` lo crea [`infra/mongo/01-catalogo.js`](../../infra/mongo/01-catalogo.js) la primera vez que arranca MongoDB, con permiso `readWrite` solo sobre `catalogo_db`. Si se cambia la contraseña en `.env`, hay que borrar el volumen (`docker compose down -v`). Más adelante se suman Memcached (TTL de 60 s), RabbitMQ y el identificador de cada instancia.

**Replica set:** MongoDB corre como replica set de un solo miembro (`rs0`), porque las transacciones del outbox solo existen en un replica set. En el compose:

- `mongod` arranca con `--replSet rs0` y una `keyFile` que se genera al iniciar el contenedor (con usuarios, un replica set la exige; montarla desde Windows no respeta los permisos que pide MongoDB).
- El healthcheck inicia el replica set la primera vez (`rs.initiate`) y solo da `healthy` cuando el nodo es el primario, así Catálogo no arranca antes de poder escribir.
- `MONGO_URI` lleva `replicaSet=rs0`.

No hace falta borrar el volumen: un MongoDB con datos pasa a replica set sin perderlos. Para comprobarlo:

```bash
docker compose exec mongo sh -c 'mongosh --quiet -u root -p "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin --eval "rs.status().members[0].stateStr"'
# PRIMARY
```

## Rutas de lectura

| Ruta | Quién la usa | Para qué |
| --- | --- | --- |
| `GET /cocinas` · `GET /cocinas/{id}` | frontend (por el gateway en `/api/v1/cocinas`) | Cocinas activas con sus restaurantes activos |
| `GET /cocinas/{id}/platos` | frontend | Menú: platos activos de restaurantes activos |
| `GET /platos/{id}` | frontend (`/api/v1/platos/{id}`) y Pedidos (precio al agregar al carrito) | Un plato activo |
| `GET /internal/cocinas` | Búsqueda | Todas las cocinas, activas o no, con todos sus restaurantes |
| `GET /internal/platos?page=&pageSize=` | Búsqueda | Todos los platos, activos o no, de a páginas (máximo 500), ordenados por id |
| `GET /health` · `GET /ready` | Docker | Proceso vivo · MongoDB responde |

## Pruebas

- Handlers con repositorio en memoria: cocinas, menú, plato desactivado (404 en lo público, presente en lo interno), páginas internas y sus límites, restaurante desactivado que oculta sus platos, MongoDB caído (503 con Problem Details) y datos iniciales que no pisan cambios.
- **Integración contra MongoDB real** (`internal/repositories/mongo`): inserción sin pisar duplicados, filtros de activos, orden, páginas y cocina inexistente. Se ejecuta con `MONGO_TEST_URI` (ver el comentario del test); sin esa variable se saltea.

- **Escrituras:** restaurantes repetidos y el quinto restaurante rechazados, admin de otra localidad rechazado, cuenta Cocina sin acceso a platos de otra cocina, y dos altas al mismo tiempo con tres restaurantes dejan exactamente cuatro.
- **Platos:** un plato nuevo arranca desactivado y solo se activa si Inventario dice que tiene receta; si Inventario no contesta, no se toma como "sin receta".
- **Caché:** devuelve lo mismo que MongoDB, no sirve el dato viejo después de escribir y, con Memcached caído, se lee de MongoDB. Hay un test contra Memcached real.
- **Mensajes:** cada cambio deja su evento en el outbox; con RabbitMQ caído los eventos esperan y se publican en orden cuando vuelve. Hay un test contra RabbitMQ real.
