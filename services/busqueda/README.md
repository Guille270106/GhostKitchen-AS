# Búsqueda

**Responsable:** Guillermina Sánchez (índice y búsqueda) · Lucía Rucci (capacidad publicada).

> **Entrega 2:** funcionando la **capacidad publicada** completa (`/v1/sedes`, `/v1/sedes/cercana`, `/v1/sedes/{sedeId}/platos`, `/v1/platos/{platoId}` y `POST /v1/proximidad`) sobre Solr, detrás del gateway con API key. Por ahora el índice se llena con **datos iniciales** (4 cocinas y 64 platos inventados); el lector de mensajes de Catálogo e Inventario, la búsqueda del frontend (`/platos/buscar`, sugerencias, zonas) y la reconstrucción llegan con esos servicios.

## Cómo levantarlo y probarlo

1. Copiar `.env.example` como `.env` y completar `PUBLIC_API_KEYS` con una clave de prueba, por ejemplo `PUBLIC_API_KEYS=clave-local`. **No subir el `.env`.**
2. Desde la raíz del repo: `docker compose up --build`. La primera vez tarda porque descarga Solr y compila el servicio. Búsqueda espera a que Solr arranque, crea los campos que faltan y carga los datos iniciales (en el log aparece `índice listo`).
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

Lo que guarda es una **copia** armada para buscar rápido (Clase 5). Sirve para mirar, pero **no decide el stock**: que un plato figure "disponible" es orientativo, y cuando el cliente lo agrega al carrito, Inventario vuelve a controlar.

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

El autocompletado es una consulta separada: no completa automáticamente el carrito ni reserva stock. Seleccionar una sugerencia ejecuta la búsqueda. Se implementará con campos de prefijos o un mecanismo equivalente de Solr, limitando las solicitudes del frontend mientras la persona escribe. Las sugerencias también conservan el alcance de la cocina y la disponibilidad solicitada, que sigue siendo orientativa.

No se promete reconocer singular y plural, búsquedas por significado ni información nutricional. Esos comportamientos requieren decisiones y datos adicionales.

Esta ampliación se define para el frontend. La API pública mantiene su contrato v1: no se agrega una operación pública de sugerencias ni se promete tolerancia a errores o sinónimos al otro grupo sin documentarlo y probarlo en el contrato correspondiente. La preparación de consultas permite configurar esos comportamientos por entrada.

## Cómo se organiza por dentro: capas, con dos entradas

Usamos capas, pero el servicio recibe trabajo por **dos lados**: por HTTP (las búsquedas) y por RabbitMQ (los mensajes que lo mantienen al día).

```
cmd/api/                       arranca el servidor HTTP y prepara el índice
internal/handlers/             entrada 1: rutas HTTP, validación y errores Problem Details
internal/services/             reglas: sede cercana, cobertura de 25 km, búsqueda de platos, proximidad
internal/repositories/         interfaces Lector e Indexador
internal/repositories/solr/    implementación sobre Solr (consultas, esquema, carga)
internal/repositories/memoria/ implementación en memoria, solo para tests
internal/domain/               cómo es un plato y una sede en la búsqueda, y sus errores
internal/geo/                  distancia Haversine
internal/seed/                 datos iniciales (4 cocinas, 64 platos) mientras no llegan mensajes
internal/middleware/           X-Correlation-Id y log JSON de cada solicitud
internal/router/               arma Gin con middleware y rutas
internal/config/               variables de entorno
```

Previsto para cuando existan Catálogo, Inventario y RabbitMQ: `internal/indexer/` (entrada 2, lector de mensajes) e `internal/clients/` (reconstrucción por API).

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
| `POST /internal/reindexar` | operación técnica autorizada en la red interna | Reconstruir la búsqueda desde cero |
| `GET /internal/apikey/verificar` | el gateway (`auth_request`) | Verificar la `X-API-Key` antes de dejar pasar una solicitud a `/public/v1`: 204 válida, 401 inválida |
| `GET /health` y `GET /ready` | Docker y el gateway | `/health`: el proceso está vivo. `/ready`: el índice está preparado y Solr responde (503 si no) |

Implementadas en la Entrega 2: las rutas `/v1/...`, `POST /v1/proximidad`, `/internal/apikey/verificar`, `/health` y `/ready`. Las demás llegan con el frontend y con Catálogo e Inventario.

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

## Con quién habla

- Solo para **reconstruir de cero**, les pide los datos a Catálogo e Inventario **por su API**. Nunca entra a sus bases.

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
| `SEED_AL_INICIAR` | `true` | Cargar los datos iniciales al arrancar |

Más adelante se suman las direcciones de RabbitMQ y de las API internas de Catálogo e Inventario. El radio de cobertura de la API publicada es 25 km, según el [contrato v1](../../docs/contracts/v1/openapi.yaml). Las claves de consumidores se validan en el gateway mediante `PUBLIC_API_KEYS`; no se incorporan al índice.

## Pruebas implementadas (Entrega 2)

`go test ./... -cover` (sin Docker):

- **Servicios:** cocina más cercana y `SIN_COBERTURA` (> 25 km), sedes inexistentes, solo disponibles y páginas, platos desactivados, falla del índice que no se disimula. Proximidad: 0 km, límite inclusivo sin redondear, `incluirDarkKitchen=false` sin Solr, Solr caído con `true` (503), cocina cerca del cliente aunque la sede esté lejos, radio aplicado `min(radioKm, 25)`, `FUERA_RANGO`, `SIN_SEDES` y empate por id.
- **Handlers:** respuestas y errores de las 5 operaciones con Problem Details, validaciones (booleano como texto, listas, campos extra, radio 0 o 501, coordenadas, cuerpo vacío o doble, más de 64 KiB), `X-Correlation-Id` recibido o generado, verificación de API key (incluida la falla cerrada sin claves) y `/ready`.
- **Repositorio Solr** contra un Solr simulado: consulta geoespacial, filtros y orden, escape de texto, esquema que solo agrega lo que falta, documentos guardados y errores de Solr como 503.
- **Datos iniciales:** 4 cocinas, 64 platos sin ids repetidos, carga idempotente y el plato `p5` igual al ejemplo del contrato.

Las respuestas se validaron contra el OpenAPI 1.1.0 con los datos en memoria, y el gateway se probó con `scripts/probar-api-publica.ps1` (401, 413, 429 y 503 con Problem Details). Queda pendiente sumar a la suite una prueba de integración contra un Solr real.

## Pruebas previstas

- Normalización de acentos y mayúsculas, errores de una edición en términos de al menos cinco letras solo si no hay resultados normales, sinónimos revisados y prioridad de coincidencias directas con relevancia.
- Autocompletado desde tres caracteres, hasta cinco nombres distintos de platos activos de la cocina, filtros conservados y errores de Solr sin simular ausencia.

- Mensajes repetidos y fuera de orden no sobrescriben datos más nuevos; las versiones de Catálogo e Inventario se comparan por separado.
- Filtros, orden, paginación e indexación contra Solr real; cocina más cercana y error `SIN_COBERTURA`.
- Proximidad: 0 km, límite inclusivo, sede fuera de rango, campos ausentes, coordenadas/radio inválidos y booleano como texto. No admite listas.
- false funciona sin Solr; true comprueba cercanía al cliente (no a la sede externa), radio efectivo, empates por ID de cocina y EN_RANGO/FUERA_RANGO/SIN_SEDES independientes; 503 ante caída sin guardar ubicaciones.
- Reconstrucción concurrente con eventos sin perder actualizaciones ni dejar sin servicio el índice anterior.
- Respuestas de la capacidad publicada conformes a OpenAPI, incluida la traducción de fallas a `503`.
