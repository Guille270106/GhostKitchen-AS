# Catálogo

**Responsable:** Guillermina Sánchez.

> Entrega 1: diseño preliminar; todavía no hay implementación. Las carpetas, rutas, configuración y pruebas indicadas son previstas para el desarrollo desde la Entrega 2.

Dice **qué se vende y a qué precio**. Puerto **8082**, con **2 copias** para repartir la carga. Lo programamos en la Entrega 2.

## Qué hace

- Maneja las **cocinas**: el admin las crea, modifica o desactiva (solo las de su localidad).
- Maneja los **restaurantes** de cada cocina: **máximo 4 por cocina** y sin repetir.
- Maneja los **platos**: nombre, descripción, categoría, etiquetas (vegano, sin TACC…), precio y si está activo.
- Cada vez que cambia algo, manda un mensaje para que Búsqueda e Inventario se enteren.

**No** guarda recetas ni stock: eso lo maneja [Inventario](../inventario/README.md).

## Qué datos guarda (MongoDB)

| Colección | Qué guarda |
| --- | --- |
| `cocinas` | Localidad, dirección, ubicación, tiempos estimados de entrega, costo de envío, si está activa y sus restaurantes adentro (máximo 4) |
| `platos` | A qué cocina y restaurante pertenece, nombre, descripción, categoría, etiquetas, precio, foto y si está activo |
| `outbox` | Los mensajes que todavía hay que mandar a RabbitMQ |

**Caché:** guardamos en Memcached las consultas más comunes ("cocinas con sus restaurantes" y "menú de una cocina") durante 60 segundos. Si el admin cambia algo, se borra al momento. Las 2 copias del servicio comparten la misma caché.

Las lecturas usadas para autorizar cuentas o validar un plato al reservar se harán directamente contra MongoDB, evitando una caché obsoleta. D7 definirá cómo prevenir que una lectura concurrente vuelva a insertar un dato viejo después de invalidarlo, y cómo verificarlo entre ambas instancias.

## Cómo se organiza por dentro: capas + caché

Usamos capas (handler → servicio → repositorio). La caché es una capa que **envuelve al repositorio** (patrón *decorator*): el servicio pide los datos igual que siempre y no sabe si vienen de la caché o de la base. Así podemos prender y apagar la caché para medir cuánto mejora (Clase 3).

```
cmd/api/               arranca el servicio
internal/handlers/     rutas HTTP
internal/services/     reglas: localidad del admin, máximo 4 restaurantes
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
| `POST /admin/cocinas/{id}/platos` · `PATCH /admin/platos/{id}` | admin | Crear o cambiar platos |

La reconstrucción dispone de lectura paginada de todas las cocinas y platos; no puede limitarse a una página de resultados. Las lecturas internas para Usuarios y Pedidos evitan la caché. Las rutas exactas y sus parámetros se terminarán de definir al implementar.

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

## Configuración prevista

`CATALOGO_DB_PASSWORD` y las credenciales comunes de MongoDB/RabbitMQ están anticipadas en [`.env.example`](../../.env.example). Las direcciones de MongoDB, Memcached y RabbitMQ, el TTL de caché (60 s) y el identificador de cada instancia se definirán al implementar. Las dos instancias usan la misma imagen y comparten MongoDB y Memcached; los datos persistentes no dependen de su memoria.

## Pruebas previstas

- Rechazar restaurantes repetidos, el quinto restaurante y cambios de un admin de otra localidad.
- Con tres restaurantes existentes, dos altas simultáneas contra MongoDB real dejan exactamente cuatro: solo una tiene éxito.
- Una falla de Memcached provoca lectura desde MongoDB; las consultas críticas evitan datos comerciales obsoletos.
- Cambio y outbox se guardan juntos; tras una caída de RabbitMQ, se recupera la publicación sin perder eventos.
