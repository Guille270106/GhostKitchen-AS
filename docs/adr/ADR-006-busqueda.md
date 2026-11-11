# ADR-006 — Búsqueda

- **Estado:** Aceptada e implementada. El índice se actualiza por mensajes de Catálogo e Inventario
- **Fecha:** 2026-10-16
- **Responsable:** Guillermina Sánchez
- **Decisión del enunciado:** D6 (qué se indexa, cómo se consulta, cómo se actualiza o reconstruye el índice y cuánto atraso aceptamos)
- **Relacionado con:** [ADR-001](ADR-001-limites-de-los-servicios.md) · [ADR-003](ADR-003-persistencia.md) · [ADR-005](ADR-005-comunicacion-entre-servicios.md) · [ADR-008](ADR-008-contrato-propio.md) · [README de Búsqueda](../../services/busqueda/README.md)

## Contexto

Tenemos que resolver tres consultas que una base de las operaciones hace mal:

- **buscar platos** de una cocina por texto, con filtros (restaurante, categoría, etiqueta, precio, disponibilidad), orden y páginas, aunque el usuario escriba sin acentos o en mayúsculas;
- **encontrar la cocina más cercana** a un punto, con un límite de cobertura de 25 km;
- **atender la capacidad publicada** para otro grupo sin cargar a Catálogo ni a Inventario con tráfico externo.

Los datos no son de Búsqueda: los comerciales son de Catálogo y la disponibilidad es de Inventario ([ADR-001](ADR-001-limites-de-los-servicios.md)). Por eso el índice es una **copia derivada**, armada para buscar y ordenar: nunca es la fuente de verdad y se tiene que poder reconstruir desde cero.

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

- **Texto:** `edismax` sobre `nombre^3 restaurante_nombre^2 categoria_txt^2 descripcion`. Un plato cuyo nombre coincide aparece antes que uno donde la palabra está solo en la descripción. Todas las palabras tienen que aparecer (cada una va con `+`). Los caracteres especiales del usuario se descartan, así no se interpretan como operadores.
  - **La última palabra también vale como comienzo** (`hambur` encuentra *Hamburguesa*), desde 3 letras; con menos, casi todo coincidiría. Va `hambur OR hambur*` porque el comodín no pasa por la raíz del español.
  - **Errores de tipeo:** una palabra de **6 letras o más** acepta una letra de diferencia sobre su raíz (`sorentinos`, `hamburgesa`, `milaneza`, `empanda` encuentran el plato). Se midió con los 64 platos del menú: con 4 o 5 letras una letra cambia de plato (`pollo` traía los *roll* de sushi, `mega` el pan de *miga*, `papa` el café "*para* preparar"); desde 6 no hay cruces. Lo ideal sería exigir que la primera letra coincida, pero la consulta de Solr que usamos no lo permite. Los errores en palabras cortas los cubren las sugerencias, que aparecen desde la 3.ª letra, antes de llegar a la letra equivocada.
  - **Con stock primero:** ordenando por relevancia, los platos disponibles suben (`bq=disponible:true^2`) sin esconder los demás.
- **Filtros** (`fq`, no afectan la relevancia y Solr los reutiliza entre consultas): `activo:true`, la cocina, `disponible:true` por defecto, y los filtros opcionales del contrato.
- **Orden:** por relevancia (`score desc, id asc`) o por precio (`precio asc|desc, id asc`). El `id` desempata para que la paginación sea estable.
- **Cocina más cercana:** `sort=geodist() asc, id asc` sobre las cocinas activas; si la más cercana está a más de 25 km, `404 SIN_COBERTURA`. La distancia que mostramos se calcula con Haversine en Go con el mismo radio terrestre que usa Solr.
- **Disponibilidad:** es orientativa. Búsqueda **nunca decide el stock**: al agregar al carrito, Inventario vuelve a controlar.

### Cómo se mantiene al día

| Mecanismo | Cuándo | Estado |
| --- | --- | --- |
| **Reconstrucción completa** desde la API de Catálogo (`/internal/cocinas` y `/internal/platos`, de a 100) | Al arrancar y con `POST /internal/reindexar` | Implementado (Entrega 2) |
| **Mensajes** `cocina.*`, `plato.*` y `disponibilidad.cambiada` | Cada vez que cambia algo | Cuando exista RabbitMQ (Entrega final) |

- La reconstrucción **agrega o reemplaza por id** y guarda página por página. Si Catálogo falla a mitad de camino, lo ya guardado queda y el resto conserva la versión anterior; la operación informa el error y se puede repetir.
- Un plato solo es visible si **él, su restaurante y su cocina** están activos. Lo desactivado se guarda con `activo=false`, así una consulta nunca lo devuelve.
- La disponibilidad sale de **Inventario**, por el mensaje `disponibilidad.cambiada` y al reconstruir el índice. Un plato sin receta (por ejemplo, uno nuevo) figura como **no disponible**.

### Atraso aceptable

| Dato | Atraso aceptado | Por qué |
| --- | --- | --- |
| Nombre, descripción, precio, fotos | Hasta 5 s, con mensajes | El precio que vale es el de Catálogo al agregar al carrito |
| Disponibilidad | Hasta 5 s | Es orientativa; Inventario controla al reservar |
| Alta o baja de una cocina | Hasta 5 s | Una cocina desactivada no puede recibir pedidos aunque aparezca un momento: Catálogo y Pedidos lo validan |

Si el atraso supera lo aceptado, lo tratamos como incidente aunque Solr siga respondiendo. Lo medimos como "tiempo entre el cambio en Catálogo y su llegada a Solr" ([ADR-011](ADR-011-observabilidad.md)).

### Si algo falla

- **Solr caído o lento** (más de 2 s): `503 SERVICIO_NO_DISPONIBLE` con `Retry-After`, **nunca una lista vacía** que parezca "no hay platos". La proximidad sin `incluirDarkKitchen` sigue funcionando porque no usa Solr.
- **Catálogo caído durante una reconstrucción:** 5 s por intento y 3 reintentos; si no responde, el índice anterior sigue sirviendo consultas.

## Alternativas que pensamos

| Alternativa | A favor | En contra | Qué decidimos |
| --- | --- | --- | --- |
| **Buscar directo en MongoDB** (índice de texto) | Un motor menos | Sin relevancia por campos, sin quitar acentos de forma configurable y sin distancia combinada con filtros; además la carga de búsqueda caería sobre la base que vende | La descartamos |
| **Elasticsearch / OpenSearch** | Muy usados | Hacen lo mismo que necesitamos; Solr ya lo conocíamos | Elegimos Solr |
| **Calcular la cocina más cercana en Go** recorriendo todas | Sin motor geoespacial | Funciona con 4 cocinas, no con muchas; y no aprovecha el índice | Solo para la proximidad externa (dos puntos que no se guardan) |
| **Que Búsqueda lea las bases de Catálogo e Inventario** | Siempre al día | Rompe "una base por servicio" ([ADR-001](ADR-001-limites-de-los-servicios.md)) | La descartamos: API para reconstruir y mensajes para cambios |
| **Sinónimos y tolerancia a errores ya en la Entrega 2** | Mejor búsqueda | Mal configurados empeoran la precisión y requieren pruebas con consultas reales | Los dejamos para la entrega final, con pruebas |
| **Tolerancia a errores desde 4 letras** | También encuentra `piza` o `polo` | Con el menú real trae platos de otro tipo (`pollo` → *roll*) | Desde 6 letras: preferimos que falte un resultado a mostrar uno equivocado |

## Consecuencias

**Lo bueno**

- Las búsquedas y la API publicada no cargan a Catálogo ni a Inventario.
- Si se pierde Solr, se reconstruye desde las fuentes: el índice es descartable.
- Las consultas son predecibles: mismo orden y mismas páginas para la misma búsqueda.

**Lo que aceptamos**

- Un cambio en Catálogo o en el stock tarda unos segundos en verse: llega por mensaje (ver "Atraso aceptable").
- La reconstrucción reescribe sobre el mismo core. No creamos una versión nueva del índice para cambiarla de golpe, que sería lo recomendable para cambios de esquema. Con 64 platos no hace falta; con miles habría que hacerlo.
- La disponibilidad del índice es orientativa: quien decide el stock es Inventario, al agregar al carrito.
- No hay sinónimos. Los errores de tipeo solo se toleran en palabras de 6 letras o más: `piza` no encuentra *Pizza* (lo cubren las sugerencias desde `piz`).

**Riesgos**

| Riesgo | Qué hacemos |
| --- | --- |
| Un mensaje viejo pisa un dato nuevo | Versión por fuente en cada documento (Catálogo y Inventario por separado) |
| El índice queda atrasado sin que nadie lo note | Medir el atraso y alertar ([ADR-011](ADR-011-observabilidad.md)) |
| Una reconstrucción a medias deja datos mezclados | Se informa el error y se repite; es idempotente |

## Cómo lo validamos

- **Entrega 2 (hecho):** tests del armado de consultas (texto, filtros, orden, páginas, escape), de la cocina más cercana y `SIN_COBERTURA`, de la reconstrucción contra un Catálogo simulado (activos, páginas, falla a mitad de camino) y del esquema idempotente. Las respuestas cumplen el OpenAPI 1.1.0.
- **Entrega final (hecho):** el lector de mensajes tiene un test contra RabbitMQ real; el atraso entre el cambio y el índice se mide (`busqueda_indice_atraso_seconds`) y la alerta `IndiceAtrasado` salta si el p95 pasa los 5 s ([ADR-011](ADR-011-observabilidad.md)).
- **Errores de tipeo (entrega final):** la consulta a Solr y la búsqueda en memoria usan el mismo umbral (`texto.MinDifuso`). Los tests cubren los dos lados con platos del menú: lo que tiene que encontrar (`sorentinos`, `milaneza`) y lo que no (`pollo` no encuentra *roll*).
- **No llegamos:** un test de integración contra un Solr real (acentos, relevancia y `geodist()`). Hoy se prueba con un Solr simulado.

