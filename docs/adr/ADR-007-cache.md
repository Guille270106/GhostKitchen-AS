# ADR-007 — Caché

- **Estado:** Propuesta (se implementa y mide para la presentación final)
- **Fecha:** 2026-10-16
- **Responsable:** Guillermina Sánchez
- **Decisión del enunciado:** D7 (qué se guarda temporalmente, cómo se invalida, cuánto dura, qué pasa si falla y cómo medimos si sirve)
- **Relacionado con:** [ADR-002](ADR-002-arquitectura-interna.md) · [ADR-003](ADR-003-persistencia.md) · [ADR-011](ADR-011-observabilidad.md) · [ADR-012](ADR-012-balanceo-de-carga.md) · [README de Catálogo](../../services/catalogo/README.md)

## Contexto

Catálogo es el servicio con más lecturas: cada vez que alguien elige una cocina o mira un menú, se consultan las mismas cocinas y los mismos platos, y cambian poco (solo cuando el admin o la cocina los editan). Va a tener **2 copias** detrás de NGINX ([ADR-012](ADR-012-balanceo-de-carga.md)), así que la caché tiene que ser la misma para las dos.

El enunciado pide para la presentación final una caché integrada en un flujo de lectura relevante, **con medición antes y después**. Seguimos las siete preguntas de la Clase 3 (sección 9).

## Decisión

### 1. Qué operación es costosa y cuál es su fuente de verdad

Las lecturas de Catálogo sobre MongoDB (`catalogo_db`), que es la fuente de verdad. La caché es una copia: si se borra, no se pierde nada.

### 2. Qué se guarda (y qué no)

| Clave | Contenido | Quién la lee |
| --- | --- | --- |
| `catalogo:v1:cocinas:g{gen}` | Cocinas activas con sus restaurantes activos | Frontend (`/api/v1/cocinas`) |
| `catalogo:v1:menu:{cocinaId}:g{gen}` | Menú de una cocina | Frontend (`/api/v1/cocinas/{id}/platos`) |
| `catalogo:v1:plato:{platoId}:g{gen}` | Detalle público de un plato | Frontend (`/api/v1/platos/{id}`) |

**No se cachea:**
- el **stock** ni la disponibilidad (son de Inventario y no se pueden equivocar);
- las lecturas que usa **Pedidos para el precio** al agregar al carrito, ni las que usa **Usuarios** para validar una cocina: van siempre a MongoDB, porque deciden algo;
- las lecturas internas de reconstrucción de Búsqueda (`/internal/...`): leen todo una sola vez y llenarían la caché de datos que nadie más pide.

### 3. Dónde vive y con qué patrón

**Memcached** (lo vimos en la materia), una sola instancia compartida por las 2 copias de Catálogo. Patrón **cache-aside**, implementado como un *decorator* del repositorio ([ADR-002](ADR-002-arquitectura-interna.md)): el servicio pide los datos igual que siempre; el decorator busca en Memcached y, si no está, va a MongoDB y guarda el resultado.

### 4. Cuánto dura y cómo se invalida

- **TTL de 60 s**, con un margen al azar de ±10 % para que no venzan todas las claves juntas.
- **Invalidación al escribir**, combinada con el TTL como red de seguridad (Clase 3, sección 7). Usamos un **número de generación**: cada cambio en una cocina, un restaurante o un plato hace `incr` sobre `catalogo:v1:gen`. Las claves llevan la generación en el nombre, así que después de un cambio nadie vuelve a leer las viejas, y vencen solas por TTL.
- **Por qué generación y no borrar la clave:** si una lectura empezó antes del cambio y escribe en la caché después, con "borrar" volvería a dejar el dato viejo. Con generación, esa lectura escribe en una clave que ya nadie consulta. Es la forma de evitar que una lectura concurrente reinserte un dato viejo, que había quedado pendiente en el README de Catálogo.
- **Atraso máximo aceptado:** 0 s después de una escritura confirmada por el mismo sistema; hasta 60 s si alguien cambia MongoDB por fuera de Catálogo (por ejemplo, a mano en una prueba).

### 5. Qué pasa ante un miss, un pico o una caída

- **Miss:** se lee de MongoDB y se guarda.
- **Pico al vencer una clave popular (stampede):** el margen al azar dispersa los vencimientos y, dentro de cada copia, una sola solicitud reconstruye cada clave mientras las demás esperan su resultado (`singleflight`). No coordinamos entre las 2 copias: en el peor caso, dos lecturas a MongoDB por clave.
- **Memcached caído o lento:** timeout de 50 ms; si falla, se lee de MongoDB, se registra en el log y se cuenta en las métricas. El usuario no ve nada, solo respuestas un poco más lentas. Con 4 cocinas y 64 platos MongoDB aguanta esa carga; si no la aguantara, preferiríamos degradar antes que tirar la base (Clase 3, sección 8.3).
- **Arranque en frío:** no precargamos; las primeras lecturas llenan la caché.

### 6. Cómo la prendemos, apagamos y medimos

- Variable `CACHE_HABILITADA` (true/false) para medir **con y sin caché** con la misma carga.
- Métricas ([ADR-011](ADR-011-observabilidad.md)): aciertos y fallos por tipo de clave, latencia con acierto y con fallo, errores y timeouts de Memcached, y consultas por segundo a MongoDB.
- La prueba de carga ([ADR-013](ADR-013-capacidad-y-costos.md)) compara p50, p95, errores y consultas a MongoDB del menú de una cocina, con caché y sin caché.
- **Criterio para quedarnos con la caché:** que baje el p95 del menú o la carga sobre MongoDB de forma visible, sin servir datos viejos después de una escritura. Si no mejora, lo documentamos y la sacamos.

## Alternativas que pensamos

| Alternativa | A favor | En contra | Qué decidimos |
| --- | --- | --- | --- |
| **Caché en memoria de cada copia** | Sin otra pieza; más rápida | Cada copia tendría su versión: una podría mostrar un precio viejo mientras la otra ya lo actualizó | La descartamos |
| **Redis** | Más funciones | No las necesitamos; Memcached es el que vimos en la materia y alcanza para clave-valor con TTL | Elegimos Memcached |
| **Borrar la clave al escribir** | Más simple | Una lectura concurrente puede reinsertar el dato viejo | Usamos generación |
| **Solo TTL, sin invalidar** | Nada que coordinar | Un precio cambiado se vería viejo hasta 60 s | La descartamos |
| **Cachear también el stock** | Más rápido | Es el dato que no se puede equivocar | La descartamos |

## Consecuencias

**Lo bueno**

- Menos carga sobre MongoDB en las lecturas más repetidas.
- Las 2 copias de Catálogo ven lo mismo.
- Se puede apagar para medir o si da problemas, sin tocar el resto.

**Lo que aceptamos**

- Una pieza más (Memcached) para levantar y monitorear.
- Un cambio hecho por fuera de Catálogo puede verse hasta 60 s tarde.
- Hasta dos lecturas a MongoDB por clave cuando vence, una por copia.

## Cómo lo vamos a validar

- **Tests:** el decorator devuelve lo mismo que el repositorio; después de una escritura no se sirve el dato viejo (incluido el caso de la lectura concurrente); con Memcached caído se lee de MongoDB.
- **Prueba de carga:** menú de una cocina con y sin caché, con 1 y 2 copias de Catálogo. Los resultados van a [ADR-013](ADR-013-capacidad-y-costos.md).

## Referencias

- Clase 3: fuente de verdad y frescura, caché local vs. distribuida, cache-aside, TTL e invalidación explícita, claves con versión, stampede, arranque en frío, caída de la caché y métricas para decidir.
