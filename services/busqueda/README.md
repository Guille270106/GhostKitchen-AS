# Búsqueda

**Responsable:** Guillermina Sánchez (índice y búsqueda) · Lucía Rucci (capacidad publicada).

> **Entrega 2:** funcionando la **capacidad publicada** completa (`/v1/sedes`, `/v1/sedes/cercana`, `/v1/sedes/{sedeId}/platos`, `/v1/platos/{platoId}` y `POST /v1/proximidad`) sobre Solr, detrás del gateway con API key. El índice se **reconstruye desde Catálogo** (MongoDB) al arrancar y con `POST /internal/reindexar`. La disponibilidad todavía es una tabla provisional (es dato de Inventario, que aún no existe): un plato nuevo, sin receta, figura no disponible. El lector de mensajes y la búsqueda del frontend (`/platos/buscar`, sugerencias, zonas) llegan con RabbitMQ y el frontend.
>
> **G15:** el índice se mantiene al día con los mensajes `cocina.*`, `plato.*` (Catálogo) y `disponibilidad.cambiada` (Inventario), y la disponibilidad sale de Inventario. Detalle en [Mensajes que escucha](#mensajes-que-escucha).

## Cómo levantarlo y probarlo

1. Crear el `.env` con las variables del [README](../../README.md#variables-del-env), poniendo una clave de prueba en `PUBLIC_API_KEYS`, por ejemplo `PUBLIC_API_KEYS=clave-local`. **No subir el `.env`.**
2. Desde la raíz del repo: `docker compose up --build`. La primera vez tarda porque descarga Solr y compila el servicio. Búsqueda espera a que Solr y Catálogo respondan, crea los campos que faltan y arma el índice con los datos de Catálogo (en el log aparece `índice reconstruido desde Catálogo` y después `índice listo`).
3. Probar por el gateway:
   ```powershell
   curl.exe -H "X-API-Key: clave-local" http://localhost:8080/public/v1/sedes
   .\scripts\probar-api-publica.ps1 -Clave clave-local
   ```
4. Tests: desde `services/busqueda`, `go test ./... -cover`. No necesitan Docker: usan un repositorio en memoria y un Solr simulado.

El panel de Solr queda en http://localhost:8983 (solo para desarrollo). Búsqueda no publica su puerto: desde afuera se entra por el gateway.

Sirve para **encontrar platos** y la **cocina más cercana**. También atiende **lo que publicamos para otro grupo**. Puerto **8083**. Lo programamos en la Entrega 2.

## Qué hace

- Busca platos de la cocina elegida por texto, con filtros (restaurante, categoría, etiqueta, precio máximo, disponibilidad), orden (relevancia o precio) y páginas.
- Sugiere la **cocina más cercana** a la zona que escribe el cliente.
- Compara una sede externa con el cliente en Go, sin guardar coordenadas; opcionalmente consulta nuestra cocina cercana al cliente.
- Atiende la capacidad **"Cocinas, platos y proximidad de sedes externas"** que publicamos ([contrato](../../docs/contracts/README.md)).
- Mantiene sus datos al día con los mensajes que recibe, y los puede **reconstruir de cero** si hace falta.

Lo que guarda es una **copia** armada para buscar rápido. Sirve para mirar, pero **no decide el stock**: que un plato figure "disponible" es orientativo, y cuando el cliente lo agrega al carrito, Inventario vuelve a controlar.

## Qué datos guarda (Solr)

| Índice | Qué guarda |
| --- | --- |
| `platos` | Nombre, descripción, categoría, etiquetas, precio, foto, restaurante, cocina, si está activo, si está disponible, si tiene receta y cuándo se actualizó |
| `cocinas` | Nombre, localidad, dirección, ubicación (para calcular distancias), tiempos estimados de entrega, costo de envío y sus restaurantes |
| `zonas` | Barrios y ciudades con su ubicación, cargados al iniciar como datos de referencia propios de Búsqueda |

Un cambio en un plato o en su stock se tiene que ver acá en **5 segundos como máximo**. Un plato sin receta no aparece como disponible.

## Experiencia de búsqueda prevista para el frontend

Estas mejoras quedan definidas como diseño para implementar y probar desde la Entrega 2; todavía no funcionan en el mock. No cambian la arquitectura ni requieren otro motor de búsqueda.

| Comportamiento | Ejemplo | Regla prevista |
| --- | --- | --- |
| Mayúsculas y acentos | `CAFE` encuentra `Café` | Normalizar el texto al indexar y consultar, conservando el nombre original para mostrarlo |
| Errores pequeños de escritura | `hamburgesa` encuentra `hamburguesa` | Si la búsqueda normal no obtiene resultados, intentar una búsqueda aproximada con hasta una edición por término de cinco letras o más |
| Sinónimos definidos | `burger` encuentra `hamburguesa` | Configurar equivalencias de comida explícitas y revisadas; no asumir que Solr entiende cualquier palabra parecida |
| Autocompletado | `ham` sugiere `Hamburguesa vegetariana` | Desde tres caracteres, ofrecer hasta cinco nombres distintos de platos activos de la cocina seleccionada cuyo nombre comience con el texto normalizado |

Las búsquedas normales incluyen los sinónimos configurados. La alternativa aproximada conserva los mismos filtros: cocina, restaurante, categoría, etiqueta, precio y disponibilidad. Con orden por relevancia, las coincidencias directas deben tener prioridad sobre las equivalencias. Con orden por precio, se respeta el orden elegido.

**Palabras a medias (G18):** la última palabra, si tiene al menos 3 letras, también vale como comienzo de palabra: "hambur" encuentra "Hamburguesa" mientras se escribe. Se busca `hambur OR hambur*`, porque el comodín no pasa por la raíz en español ("hamburguesas*" no encontraría nada) y la palabra entera sí. Las anteriores tienen que estar completas.

El autocompletado es una consulta separada: no completa automáticamente el carrito ni reserva stock. Seleccionar una sugerencia ejecuta la búsqueda. Se implementará con campos de prefijos o un mecanismo equivalente de Solr, limitando las solicitudes del frontend mientras la persona escribe. Las sugerencias también conservan el alcance de la cocina y la disponibilidad solicitada, que sigue siendo orientativa.

No se promete reconocer singular y plural, búsquedas por significado ni información nutricional. Esos comportamientos requieren decisiones y datos adicionales.

Esta ampliación se define para el frontend. La API pública mantiene su contrato v1: no se agrega una operación pública de sugerencias ni se promete tolerancia a errores o sinónimos al otro grupo sin documentarlo y probarlo en el contrato correspondiente. La preparación de consultas permite configurar esos comportamientos por entrada.

## Cómo se organiza por dentro: capas, con dos entradas

Usamos capas, pero el servicio recibe trabajo por **dos lados**: por HTTP (las búsquedas) y por RabbitMQ (los mensajes que lo mantienen al día).

```
cmd/api/                       arranca el servidor HTTP y prepara el índice
internal/handlers/             entrada 1: rutas HTTP, validación y errores Problem Details
internal/services/             reglas: sede cercana, cobertura de 25 km, búsqueda de platos, proximidad
internal/repositories/         interfaces Lector, Indexador y Actualizador (cambios sin pisar datos más nuevos)
internal/repositories/solr/    implementación sobre Solr (consultas, esquema, carga)
internal/repositories/memoria/ implementación en memoria, solo para tests
internal/domain/               cómo es un plato y una sede en la búsqueda, y sus errores
internal/geo/                  distancia Haversine
internal/indexer/              reglas para aplicar cambios, reconstrucción desde Catálogo e Inventario, y eventos (G15)
internal/mensajeria/           lector de la cola busqueda.indexador de RabbitMQ (G15)
internal/clients/              clientes HTTP de Catálogo e Inventario: 5 s por intento y 3 reintentos (trabajo de fondo)
internal/seed/                 datos iniciales para probar Búsqueda sola y disponibilidad provisional
internal/middleware/           X-Correlation-Id y log JSON de cada solicitud
internal/router/               arma Gin con middleware y rutas
internal/config/               variables de entorno
```

Desde G15 están el lector de mensajes (`internal/mensajeria`) y el cliente de Inventario (`internal/clients/inventario.go`).

**Esquema de Solr:** no hay archivos de configuración aparte. Al arrancar, Búsqueda usa la Schema API para crear los campos que falten en los cores `cocinas` y `platos`, y un tipo de texto `text_gk` que pasa a minúsculas y quita acentos (así `CAFE` encuentra `Café`). Es idempotente: se puede ejecutar en cada arranque.

## Rutas

Salvo las rutas técnicas comunes, la tabla muestra rutas internas del servicio. El frontend entra por `/api/v1` a través del gateway; las rutas entre servicios no se publican al exterior.

| Ruta | Quién la usa | Para qué |
| --- | --- | --- |
| `GET /platos/buscar` | el frontend | Buscar platos de una cocina |
| `GET /platos/sugerencias?q=&cocinaId=&soloDisponibles=` | el frontend | Desde 3 caracteres, hasta 5 nombres distintos; cocinaId obligatorio; disponibilidad true por defecto; respuesta con sugerencias; errores de parámetros 400 |
| `GET /zonas?q=` | el frontend | Sugerir la cocina más cercana a una zona |
| `POST /v1/proximidad` | el gateway para el otro grupo (API key) | Proximidad externa y cocina opcional; contrato 1.1.0 |
| `GET /v1/...` | el gateway, para el otro grupo (con API key) | Lo que publicamos: ver el [contrato](../../docs/contracts/v1/openapi.yaml). El gateway expone `/public/v1/...` y elimina `/public` al reenviar |
| `POST /internal/reindexar` | operación técnica autorizada en la red interna | Reconstruir la búsqueda desde cero. 200 con lo indexado; 409 `RECONSTRUCCION_EN_CURSO` (con `Retry-After`) si ya corre otra; 503 con lo que se llegó a guardar (`parcial`) si falla |
| `GET /internal/apikey/verificar` | el gateway (`auth_request`) | Verificar la `X-API-Key` antes de dejar pasar una solicitud a `/public/v1`: 204 válida, 401 inválida |
| `GET /health` y `GET /ready` | Docker y el gateway | `/health`: el proceso está vivo. `/ready`: el índice está preparado y los dos cores de Solr responden (503 si no) |

**Reconstrucción más robusta (G22):**
- Nunca corren dos a la vez: la del arranque y la manual comparten un candado (`indexer.Exclusiva`). La del arranque espera su turno; la manual responde 409 enseguida.
- La manual tiene su propio contexto, con plazo de 25 s: si quien la pidió corta la conexión, no queda a medias. Conserva el `correlation_id`.
- Los errores salen como Problem Details, igual que el resto del servicio.
- Las escrituras no esperan el commit (`commitWithin` de 1 s). Al terminar de armar el índice se hace un commit, así `/ready` dice "listo" con todo visible.
- `/ready` revisa los dos cores (`cocinas` y `platos`): si se borró uno, Búsqueda no está lista.

Implementadas en la Entrega 2: las rutas `/v1/...`, `POST /v1/proximidad`, `POST /internal/reindexar`, `/internal/apikey/verificar`, `/health` y `/ready`. Las demás llegan con el frontend y con Catálogo e Inventario.

## Proximidad externa prevista

Recibe exactamente dos ubicaciones: ubicacionCliente y ubicacionSede, más radioKm (> 0 y hasta 500) e incluirDarkKitchen=false por defecto. Valida latitud entre -90 y 90 y longitud entre -180 y 180. Go calcula Haversine y compara sin redondear contra el radio inclusivo; muestra kilómetros con tres decimales. sedeExterna informa distanciaKm y dentroDelRango de la sede enviada. El consumidor puede ofrecerla si está dentro del rango. No selecciona entre varias sedes ni guarda, indexa o registra las coordenadas recibidas.

Con false, darkKitchen=null y no depende de Solr. Con true, busca la cocina activa más cercana al cliente, no a la sede externa; radio efectivo min(radioKm, 25). Devuelve EN_RANGO, FUERA_RANGO o SIN_SEDES independientemente del resultado de la sede externa. La cocina propia puede estar cerca aunque la sede externa esté lejos. Empates entre cocinas por ID; caída de Solr devuelve 503 para toda esa consulta. El consumidor geocodifica direcciones si hace falta y selecciona la sede que compara.

## Mensajes que escucha

| Mensaje | Qué hace |
| --- | --- |
| `cocina.*` | Actualiza la cocina y sus restaurantes |
| `plato.*` | Agrega, cambia o saca un plato |
| `disponibilidad.cambiada` | Marca si el plato se puede pedir y si tiene receta |

Si un mensaje llega dos veces, no pasa nada: actualiza el mismo plato. Y si llega un mensaje viejo, lo ignora porque compara el número de versión.

La versión se compara por fuente: Catálogo actualiza datos comerciales e Inventario actualiza disponibilidad. Un evento de stock no sobrescribe el precio y uno de Catálogo no sobrescribe el stock. El ACK se realiza después de persistir la actualización idempotente en Solr. El objetivo de 5 segundos se mide en operación normal; las caídas generan atraso observable y requieren recuperación.

### Cómo quedó implementado (G15)

- **Dónde:** `internal/mensajeria` lee la cola `busqueda.indexador` (de a un mensaje, prefetch 1) y `internal/indexer/aplicar.go` decide qué guardar. La reconstrucción usa las mismas reglas, así que las dos pueden correr a la vez.
- **Versiones guardadas en Solr:** cada plato tiene `version_catalogo` (la `version` del plato en Catálogo) y `version_inventario` (el contador de Inventario por plato); cada cocina, `version_catalogo`. Un mensaje con una versión menor que la guardada se ignora; con la misma, se escribe lo mismo.
- **Sin perder cambios:** cada mensaje lee el documento con el real-time get de Solr y lo guarda con el `_version_` leído. Si otro escribió en el medio (por ejemplo, la reconstrucción), Solr responde 409 y se vuelve a leer y aplicar. Los mensajes no esperan el commit (`commitWithin` de 1 s): con commit por mensaje, cada uno tardaría casi 1 s.
- **Cocina:** actualiza la sede y recalcula sus platos (nombre de la cocina y del restaurante, y si se muestran). Un plato se muestra solo si él, su restaurante y su cocina están activos (RN19); por eso cada plato guarda también `plato_activo`, lo suyo.
- **Plato antes que su cocina:** se guarda inactivo con el id de la cocina, y se completa cuando llega la cocina. No se devuelve a la cola: podría quedar delante del mensaje de la cocina y trabarlo.
- **Stock antes que el plato:** se guarda un plato inactivo con solo la disponibilidad, que se completa cuando llega el de Catálogo.
- **Sin receta, nunca disponible**, aunque Inventario diga lo contrario.
- **Respuestas a RabbitMQ:** guardado → ack. No cumple el contrato (sobre inválido, el tipo no coincide con la ruta, los datos no coinciden con el sobre) → reject, y RabbitMQ lo manda a `busqueda.indexador.dlq`. Solr caído → espera 1, 2, 4… hasta 10 s y lo devuelve a la cola; al quinto intento va a la DLQ (`x-delivery-limit`).
- **RabbitMQ caído:** se reconecta esperando 1, 2, 4… hasta 10 s. Mientras tanto las búsquedas andan con lo que hay y los cambios esperan en la cola.
- **Observabilidad:** cada mensaje deja un log `evento aplicado` con tipo, `evento_id`, plato o cocina, `correlation_id` del cambio original y `demora_ms` (desde que pasó el cambio hasta que quedó en el índice; el objetivo es menos de 5000), y un span `procesar <tipo>` en la traza.

## Con quién habla

- Solo para **reconstruir de cero**, les pide los datos a Catálogo (`/internal/cocinas`, `/internal/platos`) e Inventario (`GET /platos/disponibilidad`) **por su API**. Nunca entra a sus bases.
- Si Inventario no contesta, la reconstrucción sigue sin él (`sinInventario: true` en la respuesta y en el log): los platos conservan la disponibilidad que tenían y los mensajes la ponen al día.
- La lista de Inventario no trae el contador de cada plato, así que solo se usa para platos que nunca recibieron un mensaje de stock: no se puede saber si es más nueva que el mensaje. Si Inventario agrega `secuencia` a cada item, se usa sin cambiar nada.

## Dependencias y fallas previstas

| Dependencia | Para qué | Si falla |
| --- | --- | --- |
| Solr | Índices y consultas | Responde `503`, nunca una lista vacía que simule falta de resultados; se puede navegar desde Catálogo |
| RabbitMQ | Recibir cambios | Aumenta el atraso del índice; se observa y se recupera al volver la mensajería |
| Catálogo e Inventario | Reconstrucción por API paginada | Se retoma el proceso al recuperarse; mientras tanto se conserva el índice anterior |

Las llamadas de reconstrucción admiten hasta 5 s por intento y tres reintentos: son trabajo de fondo, separado del plazo de 3 s de una solicitud de usuario ([ADR-005](../../docs/adr/ADR-005-comunicacion-entre-servicios.md)). Los índices `platos` y `cocinas` se reconstruyen desde sus fuentes; `zonas` se restaura desde su carga inicial propia. El término público `sede` no cambia el nombre interno del índice `cocinas`.

## Configuración

| Variable | Por defecto | Para qué |
| --- | --- | --- |
| `PORT` | `8083` | Puerto HTTP |
| `SOLR_URL` | `http://solr:8983/solr` | Dirección de Solr |
| `SOLR_TIMEOUT_MS` | `2000` | Tiempo máximo de cada llamada a Solr |
| `PUBLIC_API_KEYS` | vacío | Claves de los grupos consumidores, separadas por coma. Vacío = se rechazan todas |
| `CATALOGO_URL` | vacío | Dirección de Catálogo (`http://catalogo:8082` en Docker). Si está, el índice se arma desde Catálogo |
| `SEED_AL_INICIAR` | `true` | Solo sin `CATALOGO_URL`: cargar los datos iniciales embebidos (para probar Búsqueda sola) |
| `INVENTARIO_URL` | vacío | Dirección de Inventario (`http://inventario:8084` en Docker). Sin ella, los platos nuevos usan la tabla provisional de los datos de ejemplo |
| `RABBITMQ_URL` | vacío | Conexión a RabbitMQ, con usuario y contraseña (nunca se registra). Sin ella, el índice solo se actualiza al reconstruirlo |

El radio de cobertura de la API publicada es 25 km, según el [contrato v1](../../docs/contracts/v1/openapi.yaml). Las claves de consumidores se validan en el gateway mediante `PUBLIC_API_KEYS`; no se incorporan al índice.

## Pruebas implementadas (Entrega 2)

`go test ./... -cover` (sin Docker):

- **Servicios:** cocina más cercana y `SIN_COBERTURA` (> 25 km), sedes inexistentes, solo disponibles y páginas, platos desactivados, falla del índice que no se disimula. Proximidad: 0 km, límite inclusivo sin redondear, `incluirDarkKitchen=false` sin Solr, Solr caído con `true` (503), cocina cerca del cliente aunque la sede esté lejos, radio aplicado `min(radioKm, 25)`, `FUERA_RANGO`, `SIN_SEDES` y empate por id.
- **Handlers:** respuestas y errores de las 5 operaciones con Problem Details, validaciones (booleano como texto, listas, campos extra, radio 0 o 501, coordenadas, cuerpo vacío o doble, más de 64 KiB), `X-Correlation-Id` recibido o generado, verificación de API key (incluida la falla cerrada sin claves) y `/ready`.
- **Repositorio Solr** contra un Solr simulado: consulta geoespacial, filtros y orden, escape de texto, esquema que solo agrega lo que falta, documentos guardados y errores de Solr como 503.
- **Datos iniciales:** 4 cocinas, 64 platos sin ids repetidos, carga idempotente y el plato `p5` igual al ejemplo del contrato.
- **Reconstrucción** contra un Catálogo simulado: solo se muestran platos activos de restaurantes y cocinas activas, se ignoran restaurantes desconocidos, se recorren todas las páginas y, si Catálogo falla a mitad de camino, se conserva lo ya guardado y se informa el error.

**G15 (mensajes):**

- **Reglas** (`indexer/aplicar_test.go`): cada fuente cambia solo sus campos, mensajes viejos ignorados (plato, stock y cocina), repetido sin cambios, stock antes que el plato, plato antes que su cocina, plato nuevo no disponible, sin receta nunca disponible, la lista sin contador no pisa un mensaje, desactivar y reactivar la cocina o un restaurante llega a sus platos, Solr caído es pasajero.
- **Reconstrucción con mensajes:** no vuelve atrás un plato ni un stock más nuevos que llegaron por mensaje; con Inventario caído sigue y lo informa.
- **Contrato** (`indexer/eventos_test.go`): sobre inválido, tipo distinto a la ruta, secuencia distinta a la versión, datos de otro plato, tipo desconocido.
- **Solr simulado:** con un 409 vuelve a leer y escribe con el `_version_` nuevo, exacto; un plato nuevo se escribe con `_version_` -1; un mensaje viejo no escribe nada.
- **RabbitMQ real** (`mensajeria/integracion_test.go`, con `RABBITMQ_TEST_URL`): plato antes que su cocina, mensaje roto a la DLQ, cocina y stock; el plato queda visible, con el restaurante y disponible.

Las respuestas se validaron contra el OpenAPI 1.1.0 con los datos en memoria, y el gateway se probó con `scripts/probar-api-publica.ps1` (401, 413, 429 y 503 con Problem Details). Queda pendiente sumar a la suite una prueba de integración contra un Solr real.

## Pruebas previstas

- Normalización de acentos y mayúsculas, errores de una edición en términos de al menos cinco letras solo si no hay resultados normales, sinónimos revisados y prioridad de coincidencias directas con relevancia.
- Autocompletado desde tres caracteres, hasta cinco nombres distintos de platos activos de la cocina, filtros conservados y errores de Solr sin simular ausencia.

- Filtros, orden, paginación e indexación contra Solr real; cocina más cercana y error `SIN_COBERTURA`.
- Proximidad: 0 km, límite inclusivo, sede fuera de rango, campos ausentes, coordenadas/radio inválidos y booleano como texto. No admite listas.
- false funciona sin Solr; true comprueba cercanía al cliente (no a la sede externa), radio efectivo, empates por ID de cocina y EN_RANGO/FUERA_RANGO/SIN_SEDES independientes; 503 ante caída sin guardar ubicaciones.
- Respuestas de la capacidad publicada conformes a OpenAPI, incluida la traducción de fallas a `503`.
