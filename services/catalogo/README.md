# Catálogo

**Responsable:** Guillermina Sánchez.

> **Entrega 2:** funcionando como **fuente de verdad** de cocinas, restaurantes y platos sobre **MongoDB** (`catalogo_db`, con usuario propio). Al arrancar carga los datos iniciales que falten (4 cocinas, 16 restaurantes, 64 platos) sin pisar cambios. Expone las lecturas para el frontend y la lectura interna paginada con la que **Búsqueda arma su índice**, así que lo que devuelve la API publicada sale de esta base. Desde G11, el admin crea y modifica cocinas y restaurantes de su localidad (ver abajo). Las altas de platos de la cocina, la caché con Memcached, las 2 copias y los mensajes por outbox vienen después.

## Flujo de los datos (Entrega 2)

```
MongoDB (catalogo_db) ──> Catálogo ──GET /internal/cocinas, /internal/platos──> Búsqueda ──> Solr ──> /public/v1 (otro grupo)
                              └──────── GET /api/v1/cocinas, /api/v1/platos/{id} (frontend, por el gateway)
```

Búsqueda reconstruye el índice al arrancar y cuando se le pide `POST /internal/reindexar`. Cuando exista RabbitMQ, los cambios van a llegar solos con los mensajes `cocina.*` y `plato.*`.

**Probar que el dato viene de MongoDB** (con `docker compose up --build` corriendo y las claves de tu `.env`):

```powershell
# 1. Cambiar el precio de un plato directamente en MongoDB
docker compose exec mongo mongosh -u root -p <MONGO_ROOT_PASSWORD> --quiet --eval "db.getSiblingDB('catalogo_db').platos.updateOne({_id:'p1'},{`$set:{precio:15000}})"
# 2. Pedirle a Búsqueda que vuelva a armar su índice
docker compose exec busqueda wget -qO- --post-data= http://127.0.0.1:8083/internal/reindexar
# 3. El cambio se ve en Catálogo y en la API publicada
curl.exe http://localhost:8080/api/v1/platos/p1
curl.exe -H "X-API-Key: <clave>" http://localhost:8080/public/v1/platos/p1
```

También se puede mirar la base con MongoDB Compass en `mongodb://root:<MONGO_ROOT_PASSWORD>@localhost:27017/?directConnection=true`. El `directConnection=true` hace falta desde G10: sin él, Compass le pregunta al replica set por sus miembros, recibe `mongo:27017` (el nombre dentro de Docker) y no lo encuentra desde la compu.

Dice **qué se vende y a qué precio**. Puerto **8082**, con **2 copias** para repartir la carga. Lo programamos en la Entrega 2.

## Qué hace

- Maneja las **cocinas**: el admin las crea, modifica o desactiva (solo las de su localidad).
- Maneja los **restaurantes** de cada cocina: **máximo 4 por cocina** y sin repetir.
- El **rol Cocina** crea, modifica y desactiva los **platos** de sus restaurantes: nombre, descripción, categoría, etiquetas (vegano, sin TACC…), precio, foto opcional y si está activo. Admin no administra el menú.
- Cada vez que cambia algo, manda un mensaje para que Búsqueda e Inventario se enteren.

**No** guarda recetas ni stock: eso lo maneja [Inventario](../inventario/README.md).

## Escrituras del admin (G11 · HU14, HU16)

El gateway las publica en `/api/v1/admin/cocinas` y verifica el token antes (G9). Catálogo lee quién es de `X-Usuario-Id`, `X-Rol` y `X-Localidad` (que llega codificada para URL) y solo deja pasar a un **admin**, y solo sobre cocinas **de su localidad**.

| Ruta | Para qué | Respuestas |
| --- | --- | --- |
| `GET /admin/cocinas` | Las cocinas de su localidad, activas o no, con todos sus restaurantes | `200` |
| `POST /admin/cocinas` | Crear una cocina. La localidad no se manda: es la del admin. El id sale del nombre ("Cocina Güemes" → `guemes`) | `201` · `400` · `409 COCINA_REPETIDA` |
| `PATCH /admin/cocinas/{id}` | Cambiar nombre, dirección, ubicación, tiempos, costo de envío o `activa` (desactivar en lugar de borrar, RN19) | `200` · `400` · `403 OTRA_LOCALIDAD` · `404` |
| `POST /admin/cocinas/{id}/restaurantes` | Sumar un restaurante (`{"nombre": "..."}`) | `201` · `409 RESTAURANTE_REPETIDO` · `409 COCINA_LLENA` |
| `PATCH /admin/cocinas/{id}/restaurantes/{rid}` | Renombrar, desactivar o reactivar un restaurante | `200` · `404` · `409 RESTAURANTE_REPETIDO` |

Sin identidad, `401`; con otro rol, `403 SIN_PERMISO`. Un campo de más en el cuerpo (por ejemplo `localidad`) da `400`.

**Máximo 4 restaurantes, sin repetir (RN20).** El nombre se compara sin mayúsculas ni acentos ("Café Martínez" = "cafe martinez"). Un restaurante desactivado no se borra, así que sigue ocupando su lugar y su nombre.

**Dos admins al mismo tiempo: concurrencia optimista.** Cada cocina tiene un número de `version`. Para cambiarla, el servicio la lee, aplica las reglas del dominio y la guarda solo si la versión sigue siendo la que leyó (`ReplaceOne` con `_id` y `version` en el filtro, que en MongoDB es una sola operación atómica sobre el documento). Si otra solicitud la cambió en el medio, no se guarda nada y el servicio vuelve a empezar con la versión nueva (hasta 3 veces; después, `409 CONFLICTO`). Así las reglas siempre se controlan sobre lo último guardado: con 10 solicitudes a la vez sobre una cocina con 2 restaurantes, entran exactamente 2 (`TestNoSePasaDeCuatroConSolicitudesALaVez`). La misma `version` es la que usará Búsqueda para no guardar un dato viejo encima de uno nuevo.

**Hasta G15**, Búsqueda se entera de estos cambios recién cuando reconstruye su índice (al arrancar). Con G13 y G15 se va a enterar por mensajes.

## Qué datos guarda (MongoDB)

| Colección | Qué guarda |
| --- | --- |
| `cocinas` | Localidad, dirección, ubicación, tiempos estimados de entrega, costo de envío, si está activa y sus restaurantes adentro (máximo 4) |
| `platos` | A qué cocina y restaurante pertenece, nombre, descripción, categoría, etiquetas, precio, foto y si está activo |
| `outbox` | Los mensajes que todavía hay que mandar a RabbitMQ |

**Caché:** guardamos en Memcached las consultas más comunes ("cocinas con sus restaurantes" y "menú de una cocina") durante 60 segundos. Si Admin cambia cocinas/restaurantes o Cocina cambia platos propios, se invalida la caché correspondiente. Las 2 copias del servicio comparten la misma caché.

Las lecturas usadas para autorizar cuentas o validar un plato al reservar se harán directamente contra MongoDB, evitando una caché obsoleta. D7 definirá cómo prevenir que una lectura concurrente vuelva a insertar un dato viejo después de invalidarlo, y cómo verificarlo entre ambas instancias.

## Cómo se organiza por dentro: capas + caché

Usamos capas (handler → servicio → repositorio). La caché es una capa que **envuelve al repositorio** (patrón *decorator*): el servicio pide los datos igual que siempre y no sabe si vienen de la caché o de la base. Así podemos prender y apagar la caché para medir cuánto mejora (Clase 3).

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

## Dependencias y fallas previstas

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
| `SEED_AL_INICIAR` | `true` | Insertar los datos iniciales que falten (no pisa los existentes) |

El usuario `catalogo` lo crea [`infra/mongo/01-catalogo.js`](../../infra/mongo/01-catalogo.js) la primera vez que arranca MongoDB, con permiso `readWrite` solo sobre `catalogo_db`. Si se cambia la contraseña en `.env`, hay que borrar el volumen (`docker compose down -v`). Más adelante se suman Memcached (TTL de 60 s), RabbitMQ y el identificador de cada instancia.

**Replica set (G10):** MongoDB corre como replica set de un solo miembro (`rs0`), porque las transacciones del outbox (G13) solo existen en un replica set. En el compose:

- `mongod` arranca con `--replSet rs0` y una `keyFile` que se genera al iniciar el contenedor (con usuarios, un replica set la exige; montarla desde Windows no respeta los permisos que pide MongoDB).
- El healthcheck inicia el replica set la primera vez (`rs.initiate`) y solo da `healthy` cuando el nodo es el primario, así Catálogo no arranca antes de poder escribir.
- `MONGO_URI` lleva `replicaSet=rs0`.

No hace falta borrar el volumen: un MongoDB con datos pasa a replica set sin perderlos. Para comprobarlo:

```bash
docker compose exec mongo sh -c 'mongosh --quiet -u root -p "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin --eval "rs.status().members[0].stateStr"'
# PRIMARY
```

## Rutas implementadas (Entrega 2)

| Ruta | Quién la usa | Para qué |
| --- | --- | --- |
| `GET /cocinas` · `GET /cocinas/{id}` | frontend (por el gateway en `/api/v1/cocinas`) | Cocinas activas con sus restaurantes activos |
| `GET /cocinas/{id}/platos` | frontend | Menú: platos activos de restaurantes activos |
| `GET /platos/{id}` | frontend (`/api/v1/platos/{id}`) y, más adelante, Pedidos | Un plato activo |
| `GET /internal/cocinas` | Búsqueda | Todas las cocinas, activas o no, con todos sus restaurantes |
| `GET /internal/platos?page=&pageSize=` | Búsqueda | Todos los platos, activos o no, de a páginas (máximo 500), ordenados por id |
| `GET /health` · `GET /ready` | Docker | Proceso vivo · MongoDB responde |

## Pruebas implementadas (Entrega 2)

- Handlers con repositorio en memoria: cocinas, menú, plato desactivado (404 en lo público, presente en lo interno), páginas internas y sus límites, restaurante desactivado que oculta sus platos, MongoDB caído (503 con Problem Details) y datos iniciales que no pisan cambios.
- **Integración contra MongoDB real** (`internal/repositories/mongo`): inserción sin pisar duplicados, filtros de activos, orden, páginas y cocina inexistente. Se ejecuta con `MONGO_TEST_URI` (ver el comentario del test); sin esa variable se saltea.

## Pruebas previstas

- Rechazar restaurantes repetidos, el quinto restaurante y cambios de un admin de otra localidad.
- Admin rechazado al crear/modificar/desactivar platos (403); Cocina sin acceso a platos o restaurantes de otra cocina (404), aunque cambie IDs o headers.
- Precios modificados no alteran carritos ni pedidos existentes (RN15); desactivar conserva los pedidos históricos.
- Con tres restaurantes existentes, dos altas simultáneas contra MongoDB real dejan exactamente cuatro: solo una tiene éxito.
- Una falla de Memcached provoca lectura desde MongoDB; las consultas críticas evitan datos comerciales obsoletos.
- Cambio y outbox se guardan juntos; tras una caída de RabbitMQ, se recupera la publicación sin perder eventos.
