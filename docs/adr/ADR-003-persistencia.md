# ADR-003 — Qué base de datos usa cada servicio

- **Estado:** Primera versión (se valida en la Entrega 2)
- **Fecha:** 2026-10-09
- **Responsable:** Valentina Rey
- **Decisión del enunciado:** D3 (qué almacenamiento usa cada servicio, cómo se usan los datos y qué limitaciones aceptamos)
- **Relacionado con:** [ADR-001](ADR-001-limites-de-los-servicios.md) · [ADR-005](ADR-005-comunicacion-entre-servicios.md) · [ARCHITECTURE](../ARCHITECTURE.md) (secciones 4 y 6)

## Contexto

Cada servicio tiene su propia base ([ADR-001](ADR-001-limites-de-los-servicios.md)), así que cada uno puede elegir la que más le sirve. El enunciado pide usar al menos una base relacional y una no relacional, y tener al menos una operación que no pueda duplicar ni perder datos.

La pregunta no es "¿SQL o NoSQL?", sino **qué necesita cada servicio** (Clase 2). Para cada uno miramos:

- cómo son los datos;
- qué consultas se hacen;
- si se lee más de lo que se escribe, o al revés;
- qué tan importante es que el dato esté siempre bien;
- qué sabemos usar (MySQL y MongoDB ya los usamos en Desarrollo de Software y en el práctico).

## Decisión

| Servicio | Base | Nombre |
| --- | --- | --- |
| Usuarios | **MySQL** | `usuarios_db` |
| Inventario | **MySQL** | `inventario_db` |
| Pedidos | **MySQL** | `pedidos_db` |
| Catálogo | **MongoDB** + **Memcached** (caché) | `catalogo_db` |
| Notificaciones | **MongoDB** | `notificaciones_db` |
| Búsqueda | **Solr** | Índices `platos`, `cocinas` y `zonas` |

Así cumplimos con una base **relacional (MySQL)** y una **no relacional (MongoDB)**. La operación que **no puede duplicar ni perder información** es **reservar, descontar y devolver stock** en Inventario, sobre MySQL (el detalle va en el ADR-004).

**Una base por servicio, pero en pocas instancias.** En local levantamos **un solo MySQL** con tres bases y **un solo MongoDB** con dos bases. Cada servicio tiene un usuario que solo puede entrar a su base. La Clase 2 permite esto: lo importante es que **ningún servicio entre a la base de otro**.

### Por qué cada una

#### Inventario → MySQL

- **Qué pasa:** muchas personas reservando a la vez los mismos ingredientes, sobre todo cuando se están terminando. Nunca puede quedar más reservado que lo que hay.
- **Por qué MySQL:** reservar un plato cambia **varios ingredientes a la vez**, y tiene que ser todo o nada. MySQL lo hace con una transacción. Para cada ingrediente, la base suma lo reservado **solo si alcanza**:
  ```sql
  UPDATE ingrediente SET reservado = reservado + :n
  WHERE id = :id AND total - reservado >= :n;
  -- si no cambió ninguna fila, no alcanza: se deshace todo
  ```
  Como la base controla esto, si dos personas quieren el último plato a la vez, solo una lo consigue.
- **Qué guarda:** ingredientes (total y reservado), recetas, reservas con su vencimiento, el historial de movimientos (reposiciones, mermas, descuentos y devoluciones), y una copia de la localidad de cada cocina y de la cocina de cada plato.

#### Pedidos → MySQL

- **Qué pasa:** se crean pedidos, cambian de estado y se consultan por cliente o por cocina.
- **Por qué MySQL:** cuando cambia el estado, tenemos que guardar **juntos** el pedido, su historial y el mensaje para el mail. Con una transacción, o se guarda todo o nada. Además, si el cliente cancela justo cuando la cocina acepta, el pedido tiene un número de **versión**: solo se guarda el primer cambio.
- **Qué guarda:** carritos, pedidos con la copia de sus platos y del mail del cliente, historial de estados, y en qué paso va cada confirmación.

#### Usuarios → MySQL

- Son datos simples y fijos (usuario, rol, dirección, cocina elegida). Necesitamos que el mail no se repita, y MySQL lo controla solo (`UNIQUE`).
- **La contraseña se guarda encriptada con bcrypt** y nunca se devuelve. Las cuentas de admin se cargan al iniciar el sistema.

#### Catálogo → MongoDB + Memcached

- **Qué pasa:** se lee mucho ("el menú de esta cocina", "este plato") y cambia poco (solo el admin).
- **Por qué MongoDB:** una cocina con sus restaurantes es un documento: se lee siempre junta y tiene como máximo 4 restaurantes, así que los guardamos adentro de la cocina (Clase 2). Además, el límite de 4 se controla en **una sola operación** sobre ese documento, sin transacciones. Los platos van en otra colección, porque se consultan solos y son muchos.
- **Caché:** Memcached guarda las lecturas más comunes para no ir siempre a la base. Lo comparten las 2 copias de Catálogo. No es la fuente de la verdad: si se borra, no se pierde nada (Clase 3).

#### Notificaciones → MongoDB

- Guarda un registro por mail: qué mensaje fue, a quién y si ya se mandó. Son datos simples, sin relaciones, y se buscan por el ID del mensaje. Cada tipo de aviso guarda datos un poco distintos, y un documento se adapta bien a eso.

#### Búsqueda → Solr

- Es un motor de búsqueda: encuentra platos por texto, filtra, ordena y también calcula distancias (para la cocina más cercana).
- Los índices de platos y cocinas son **copias** y se reconstruyen por las API de Catálogo e Inventario. Las zonas (barrios y ciudades con su ubicación) son datos de referencia propios de Búsqueda, cargados al iniciar; se restauran desde esa carga inicial.

### Cómo se organiza cada base

| Base | Tablas o colecciones | Restricciones e índices importantes |
| --- | --- | --- |
| `usuarios_db` | `usuarios` | `UNIQUE` en el correo |
| `catalogo_db` | `cocinas` (con sus restaurantes adentro), `platos`, `outbox` | Platos por cocina y activo; platos por restaurante |
| `inventario_db` | `ingrediente` (total y reservado), `receta`, `reserva`, `reserva_item` (ingredientes y cantidades que apartó cada reserva), `movimiento`, copias de cocinas y platos, `outbox`, `mensajes_procesados` | `CHECK (reservado >= 0)` y `CHECK (reservado <= total)`; `UNIQUE` de la reserva por carrito |
| `pedidos_db` | `carrito`, `carrito_item`, `confirmacion`, `pedido`, `pedido_item`, `historial`, `outbox`, `mensajes_procesados` | Un solo carrito activo por cliente (`UNIQUE` sobre una columna que solo tiene valor si el carrito está activo); `version` en `pedido` |
| `notificaciones_db` | `envios` | `unique` en el id del mensaje |
| Solr | `platos`, `cocinas`, `zonas` | Solo los campos necesarios para buscar, filtrar y mostrar (Clase 5) |

## Alternativas que pensamos

| Alternativa | A favor | En contra | Qué decidimos |
| --- | --- | --- | --- |
| **Todo en MySQL** | Una sola base | El catálogo quedaría en muchas tablas; y el enunciado pide una base no relacional | La descartamos |
| **Inventario en MongoDB** | Una base menos | Lo que hace Inventario es justo lo que mejor hace una base relacional | La descartamos |
| **Pedidos en MongoDB** | El pedido es un documento | Necesitamos guardar pedido, historial y mensaje juntos, y buscar por cocina y estado: MySQL es más seguro | La descartamos |
| **PostgreSQL en vez de MySQL** | Muy completo | Ya usamos MySQL con GORM en Desarrollo de Software, y MySQL 8 tiene lo que necesitamos (`CHECK`, `SKIP LOCKED`, columnas generadas) | Elegimos MySQL |
| **Notificaciones en MySQL** | Un motor menos para ese servicio | Funcionaría, pero son registros sueltos, sin relaciones y con forma variable según el aviso | Elegimos MongoDB |
| **Redis para las reservas** | Muy rápido | Podría perder datos y es más difícil reservar varios ingredientes a la vez | La descartamos |
| **Un MySQL y un MongoDB por servicio** | Más separado | Mucha memoria en cada compu | La descartamos para local |
| **Elasticsearch en vez de Solr** | Muy usado | Solr es el que vimos en la materia y hace lo mismo que necesitamos | La descartamos |

## Consecuencias

**Lo bueno**

- Reservar es **todo o nada** y la base lo controla: no se puede vender de más.
- El pedido, su historial y su mensaje se guardan juntos: no se pierde ningún aviso.
- El catálogo se lee en una sola consulta y es fácil de cachear.

**Lo que aceptamos**

- Hay que levantar y mantener varias tecnologías (MySQL, MongoDB, Solr y Memcached).
- **Como es una instancia de MySQL para tres servicios, si se cae MySQL se caen los tres.** Lo aceptamos para local.
- No se pueden hacer JOIN entre servicios: si una pantalla necesita datos de varios, se piden por API.
- Hay datos repetidos a propósito (la copia del plato en el pedido, los datos en la búsqueda).
- MongoDB tiene que correr en un modo especial (replica set) para poder usar transacciones en Catálogo.
- Cuando muchos reservan el mismo ingrediente, MySQL se puede poner lento. Lo vamos a medir en el test de carga.
- Sin backups en el entorno académico.

## Pendiente para la Entrega 2

- Confirmar los índices con los tests de carga.
- Definir cómo se crean y cambian las tablas: migraciones en archivos SQL que se ejecutan al iniciar cada servicio.

## Cómo lo vamos a validar

- **Entrega 2:** Inventario y Catálogo funcionando con sus bases reales, con tests que prueben reservas al mismo tiempo.
- **Entrega final:** el test de carga tiene que dar 0 platos vendidos de más, y agregar al carrito tiene que tardar menos de 500 ms con 100 usuarios a la vez.

## Referencias

- Clase 2: una base por servicio y sus variantes, cómo elegir entre SQL y NoSQL, embeber o referenciar en MongoDB, atomicidad por documento.
- Clase 3: la caché no es la fuente de la verdad.
- Clase 5: el índice de búsqueda como copia que se puede reconstruir.
