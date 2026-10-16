# ADR-006 — Búsqueda

- **Estado:** Aceptada (Entrega 2). La actualización por mensajes se suma cuando exista RabbitMQ
- **Fecha:** 2026-10-16
- **Responsable:** Guillermina Sánchez
- **Decisión del enunciado:** D6 (qué se indexa, cómo se consulta, cómo se actualiza o reconstruye el índice y cuánto atraso aceptamos)
- **Relacionado con:** [ADR-001](ADR-001-limites-de-los-servicios.md) · [ADR-003](ADR-003-persistencia.md) · [ADR-005](ADR-005-comunicacion-entre-servicios.md) · [ADR-008](ADR-008-contrato-propio.md) · [README de Búsqueda](../../services/busqueda/README.md)

## Contexto

Tenemos que resolver tres consultas que una base de las operaciones hace mal:

- **buscar platos** de una cocina por texto, con filtros (restaurante, categoría, etiqueta, precio, disponibilidad), orden y páginas, aunque el usuario escriba sin acentos o en mayúsculas;
- **encontrar la cocina más cercana** a un punto, con un límite de cobertura de 25 km (RN24);
- **atender la capacidad publicada** para otro grupo sin cargar a Catálogo ni a Inventario con tráfico externo.

Los datos no son de Búsqueda: los comerciales son de Catálogo y la disponibilidad es de Inventario ([ADR-001](ADR-001-limites-de-los-servicios.md)). La Clase 5 plantea que el índice es una **vista derivada** para recuperar y ordenar, que nunca es la fuente de verdad y que tiene que poder reconstruirse.

## Decisión

### Motor y qué se indexa

Usamos **Solr** (el motor que vimos en la materia), con un core por tipo de documento. El esquema lo crea Búsqueda al arrancar con la Schema API, y solo agrega lo que falta.

| Core | Campos | Para qué |
| --- | --- | --- |
| `cocinas` | `id`, nombre, localidad, dirección, `ubicacion` (tipo `location`), tiempos de entrega, costo de envío, restaurantes activos (guardados como JSON, sin indexar), `activo` | Listar sedes y calcular la más cercana con `geodist()` |
| `platos` | `nombre`, `descripcion`, `restaurante_nombre` y `categoria_txt` como texto; `categoria`, `etiquetas`, `restaurante_id`, `cocina_id` como valores exactos; `precio`, `disponible`, `activo`, `actualizado_en` | Buscar, filtrar, ordenar y mostrar |
| `zonas` (previsto) | Barrios y ciudades con su ubicación | Sugerir cocina a partir de lo que escribe el cliente (frontend) |

Guardamos **solo lo necesario para buscar, filtrar y mostrar**. No indexamos recetas, stock en unidades ni las coordenadas que manda el otro grupo en `POST /v1/proximidad`, que se calculan en Go y no se guardan.

**Texto:** un tipo propio, `text_gk`, que separa palabras, pasa a minúsculas y **quita acentos** (`ASCIIFoldingFilter`). Así `CAFE` encuentra `Café` y `hamburguesa` encuentra `Hamburguesas`. El texto original se guarda para mostrarlo tal cual.

### Cómo se consulta

- **Texto:** `edismax` sobre `nombre^3 restaurante_nombre^2 categoria_txt^2 descripcion`. Un plato cuyo nombre coincide aparece antes que uno donde la palabra está solo en la descripción. Con `mm=100%`, todas las palabras tienen que aparecer. Los caracteres especiales del usuario se escapan para que no se interpreten como operadores.
- **Filtros** (`fq`, no afectan la relevancia y Solr los reutiliza entre consultas): `activo:true`, la cocina, `disponible:true` por defecto, y los filtros opcionales del contrato.
- **Orden:** por relevancia (`score desc, id asc`) o por precio (`precio asc|desc, id asc`). El `id` desempata para que la paginación sea estable.
- **Cocina más cercana:** `sort=geodist() asc, id asc` sobre las cocinas activas; si la más cercana está a más de 25 km, `404 SIN_COBERTURA`. La distancia que mostramos se calcula con Haversine en Go con el mismo radio terrestre que usa Solr.
- **Disponibilidad:** es orientativa (RN21). Búsqueda **nunca decide el stock**: al agregar al carrito, Inventario vuelve a controlar.

### Cómo se mantiene al día

| Mecanismo | Cuándo | Estado |
| --- | --- | --- |
| **Reconstrucción completa** desde la API de Catálogo (`/internal/cocinas` y `/internal/platos`, de a 100) | Al arrancar y con `POST /internal/reindexar` | Implementado (Entrega 2) |
| **Mensajes** `cocina.*`, `plato.*` y `disponibilidad.cambiada` | Cada vez que cambia algo | Cuando exista RabbitMQ (Entrega final) |

- La reconstrucción **agrega o reemplaza por id** y guarda página por página. Si Catálogo falla a mitad de camino, lo ya guardado queda y el resto conserva la versión anterior; la operación informa el error y se puede repetir.
- Un plato solo es visible si **él, su restaurante y su cocina** están activos (RN19). Lo desactivado se guarda con `activo=false`, así una consulta nunca lo devuelve.
- Mientras no existe Inventario, la disponibilidad sale de una **tabla provisional**. Un plato que no está en la tabla (por ejemplo, uno nuevo, todavía sin receta) figura como **no disponible**.

### Atraso aceptable

| Dato | Atraso aceptado | Por qué |
| --- | --- | --- |
| Nombre, descripción, precio, fotos | Hasta 5 s con mensajes; en la Entrega 2, hasta la siguiente reconstrucción | El precio que vale es el de Catálogo al agregar al carrito (RN15) |
| Disponibilidad | Hasta 5 s (RN21) | Es orientativa; Inventario controla al reservar |
| Alta o baja de una cocina | Hasta 5 s | Una cocina desactivada no puede recibir pedidos aunque aparezca un momento: Catálogo y Pedidos lo validan |

Si el atraso supera lo aceptado, lo tratamos como incidente aunque Solr siga respondiendo (Clase 5). Lo vamos a medir desde la Entrega final como "tiempo entre el cambio en Catálogo y su llegada a Solr" ([ADR-011](ADR-011-observabilidad.md)).

### Si algo falla

- **Solr caído o lento** (más de 2 s): `503 SERVICIO_NO_DISPONIBLE` con `Retry-After`, **nunca una lista vacía** que parezca "no hay platos". La proximidad sin `incluirDarkKitchen` sigue funcionando porque no usa Solr.
- **Catálogo caído durante una reconstrucción:** 5 s por intento y 3 reintentos; si no responde, el índice anterior sigue sirviendo consultas.

## Alternativas que pensamos

| Alternativa | A favor | En contra | Qué decidimos |
| --- | --- | --- | --- |
| **Buscar directo en MongoDB** (índice de texto) | Un motor menos | Sin relevancia por campos, sin quitar acentos de forma configurable y sin distancia combinada con filtros; además la carga de búsqueda caería sobre la base que vende | La descartamos |
| **Elasticsearch / OpenSearch** | Muy usados | Hacen lo mismo que necesitamos; Solr es el de la materia | Elegimos Solr |
| **Calcular la cocina más cercana en Go** recorriendo todas | Sin motor geoespacial | Funciona con 4 cocinas, no con muchas; y no aprovecha el índice | Solo para la proximidad externa (dos puntos que no se guardan) |
| **Que Búsqueda lea las bases de Catálogo e Inventario** | Siempre al día | Rompe "una base por servicio" ([ADR-001](ADR-001-limites-de-los-servicios.md)) | La descartamos: API para reconstruir y mensajes para cambios |
| **Sinónimos y tolerancia a errores ya en la Entrega 2** | Mejor búsqueda | Mal configurados empeoran la precisión (Clase 5) y requieren pruebas con consultas reales | Los dejamos para el frontend, con pruebas |

## Consecuencias

**Lo bueno**

- Las búsquedas y la API publicada no cargan a Catálogo ni a Inventario.
- Si se pierde Solr, se reconstruye desde las fuentes: el índice es descartable.
- Las consultas son predecibles: mismo orden y mismas páginas para la misma búsqueda.

**Lo que aceptamos**

- Hasta que exista RabbitMQ, un cambio en Catálogo solo se ve después de reconstruir.
- La reconstrucción reescribe sobre el mismo core. No creamos una versión nueva del índice para cambiarla de golpe, como sugiere la Clase 5 para cambios de esquema. Con 64 platos no hace falta; con miles habría que hacerlo.
- La disponibilidad es provisional hasta que exista Inventario.
- No hay sinónimos ni corrección de errores de tipeo todavía.

**Riesgos**

| Riesgo | Qué hacemos |
| --- | --- |
| Un mensaje viejo pisa un dato nuevo | Versión por fuente en cada documento (Catálogo y Inventario por separado), al implementar los mensajes |
| El índice queda atrasado sin que nadie lo note | Medir el atraso y alertar ([ADR-011](ADR-011-observabilidad.md)) |
| Una reconstrucción a medias deja datos mezclados | Se informa el error y se repite; es idempotente |

## Cómo lo validamos

- **Entrega 2 (hecho):** tests del armado de consultas (texto, filtros, orden, páginas, escape), de la cocina más cercana y `SIN_COBERTURA`, de la reconstrucción contra un Catálogo simulado (activos, páginas, falla a mitad de camino) y del esquema idempotente. Las respuestas cumplen el OpenAPI 1.1.0.
- **Pendiente:** test de integración contra un Solr real en Docker Compose (acentos, relevancia y `geodist()`); medir el atraso de indexación cuando existan los mensajes.

## Referencias

- Clase 5: el índice como vista derivada, analizadores (tokenización, minúsculas, acentos), filtros vs. relevancia, sinónimos con cuidado, consistencia eventual, *freshness* y reindexación.
- Clase 2: una base por servicio; copias de datos por eventos.
