# ADR-013 — Capacidad y costos

- **Estado:** Mediciones locales completadas. Costo de nube documentado como estimación del despliegue.
- **Fecha:** 2026-10-16
- **Responsable:** Valentina Rey
- **Decisión del enunciado:** D13 (resultados de las pruebas de carga, principal límite identificado, alternativa de escalado y estimación de costos)
- **Relacionado con:** [ADR-003](ADR-003-persistencia.md) · [ADR-007](ADR-007-cache.md) · [ADR-011](ADR-011-observabilidad.md) · [ADR-012](ADR-012-balanceo-de-carga.md) · [SPEC](../../SPEC.md) (atributos de calidad)

- **Última actualización:** 2026-11-09

## Contexto

El enunciado pide un test de carga ejecutado, con resultados, análisis de capacidad y el principal límite identificado, más una estimación de costos. Los objetivos ya están en el [SPEC](../../SPEC.md):

- 200 clientes queriendo el último plato en 10 s → **0 platos vendidos de más**;
- con 100 usuarios a la vez, **agregar al carrito < 500 ms** y **buscar < 300 ms** (p95).

En la Entrega 2 funcionan Búsqueda, Catálogo y el gateway. Inventario y Pedidos llegan después, así que esta versión define **cómo** vamos a medir y qué esperamos encontrar; los números se completan cuando el flujo principal exista.

## Decisión

### Herramienta: k6

**k6** **[Adicional]**: los escenarios se escriben en JavaScript, permite subir los usuarios de a poco y define umbrales (`p(95)<300`) que hacen fallar la prueba si no se cumplen, así el resultado no depende de mirar un gráfico. Corre en Docker, sin instalar nada en las computadoras.

### Escenarios

| # | Qué | Carga | Qué medimos | Cuándo |
| --- | --- | --- | --- | --- |
| 1 | **API publicada**: sedes, cocina más cercana, buscar platos, proximidad | Subir de 10 a 100 usuarios en 2 min y sostener 3 min | p50/p95/p99, solicitudes por segundo, % de errores (sin contar los 429 esperados) | Ya se puede correr (Entrega 2) |
| 2 | **Menú de Catálogo**, con y sin caché, con 1 y 2 copias | 100 usuarios, 3 min por combinación | p95, solicitudes por segundo, consultas a MongoDB, aciertos de caché | Con caché y balanceo ([ADR-007](ADR-007-cache.md), [ADR-012](ADR-012-balanceo-de-carga.md)) |
| 3 | **Agregar al carrito** con muchos pidiendo el mismo plato | 200 usuarios en 10 s sobre un plato con stock para 20 | Platos vendidos de más (tiene que ser 0), p95, reintentos por *deadlock*, rechazos por falta de stock | Con Inventario y Pedidos |
| 4 | **Caída de una copia de Catálogo** durante el escenario 2 | 100 usuarios | Errores visibles y tiempo hasta que NGINX la saca | Con balanceo |

El escenario 1 le pega **directo a Búsqueda** dentro de la red de Docker (`http://busqueda:8083/v1`), sin pasar por el gateway: el límite de 60 solicitudes por minuto por clave cortaría la prueba y estaríamos midiendo el límite, no el sistema. El límite del gateway se prueba aparte (el 429 ya está probado). El script es [`tests/carga/api-publica.js`](../../tests/carga/api-publica.js).

### Resultados locales del 9 de noviembre de 2026

Usamos k6 0.54.0 y bases descartables en Docker local. Docker tenía 12 CPU lógicas y 7,54 GiB de RAM asignadas durante V18. Los comandos y las condiciones de repetición quedan en la [guía de carga](../../tests/carga/README.md).

| Escenario | Base | Solicitudes | Solicitudes/s | p50 (ms) | p95 (ms) | p99 (ms) | Errores inesperados |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 · API publicada, directo a Búsqueda | b25025f | 102.692 | 310,28 | 2,80 | 10,11 | 18,44 | 0 |
| 3 · Agregar al carrito por el gateway | 2f37248 | 200 | 20 programadas | 19,22 | 34,63 | 42,83 | 0 |

En el escenario 1, buscar platos dio **13,82 ms de p95** en 25.673 consultas y cumplió el objetivo de 300 ms. Se usaron Búsqueda, Solr y el seed de Catálogo, con caché deshabilitada y sin la integración de disponibilidad de Inventario.

En el escenario 3, **20 carritos fueron aceptados y 180 recibieron SIN_STOCK**. Inventario terminó con 20 unidades reservadas y cero disponibles. La carga fue escalonada a 20 solicitudes por segundo durante 10 segundos. El resultado verifica que el carrito no excedió el stock en ese patrón. El escenario separado de 100 usuarios concurrentes sobre el carrito y el conteo de reintentos por deadlock quedan sin medir en esta corrida.

### Escenarios 2 y 4 · Catálogo y caída

Base a769d3b, 100 usuarios virtuales durante tres minutos por caso, a través del gateway. Los comandos de MongoDB incluyen la preparación de menús de k6.

| Configuración | Solicitudes | Solicitudes/s | p50 (ms) | p95 (ms) | p99 (ms) | Errores | Mongo find + aggregate | Aciertos de menú |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Sin caché, una copia | 17.900 | 99,31 | 4,02 | 13,44 | 24,76 | 0 | 35.842 | 0 |
| Sin caché, dos copias | 17.900 | 99,27 | 3,95 | 14,05 | 31,69 | 0 | 35.843 | 0 |
| Con caché, una copia | 17.900 | 99,39 | 3,62 | 11,66 | 23,67 | 0 | 67 | 17.879 |
| Con caché, dos copias | 17.900 | 99,41 | 3,38 | 12,49 | 23,41 | 0 | 82 | 17.872 |
| Con caché, caída de una de dos copias | 17.936 | 99,08 | 3,25 | 10,32 | 20,79 | 0 | 66 | 17.870 |

Con ambas copias activas, cada una atendió 8.950 solicitudes en los casos estables. Después de apagar una copia, el log del gateway registró 8.815 respuestas exitosas de la restante. El último intento observado hacia la IP apagada ocurrió 83 segundos después del inicio del apagado. Hubo solicitudes de hasta aproximadamente un segundo durante la falla. El tiempo observado corresponde al último intento registrado hacia esa IP.

### Límite observado y alternativa

La mayor presión medida fue la cantidad de comandos a MongoDB para leer menús sin caché. Con una copia, Memcached redujo los comandos de 35.842 a 67 con el mismo volumen de solicitudes. Sumar una segunda copia repartió el tráfico sin reducir esa carga de MongoDB. Mantener la caché compartida reduce esas lecturas y mantener dos copias permite sostener el servicio ante la caída de una. La carga aplicada no identificó un punto de saturación del sistema. El bloqueo del ingrediente más demandado sigue siendo una hipótesis para una prueba con más concurrencia efectiva.

### Cómo registramos los resultados

Por escenario: fecha y versión (commit), máquina usada, tabla de p50/p95/p99, solicitudes por segundo y errores, gráfico del tablero de Grafana en el mismo período, y qué recurso se saturó primero (CPU, memoria, conexiones, bloqueos). Los scripts quedan en `tests/carga/` y los resultados en este ADR.

### Hipótesis del principal límite

1. **Las filas del ingrediente más pedido en MySQL** (escenario 3): todas las reservas del mismo ingrediente esperan su turno por el bloqueo de fila. Es justamente lo que garantiza no vender de más; esperamos que el p95 de agregar al carrito suba con la cantidad de usuarios que piden **lo mismo**, no con la cantidad total.
2. **Solr con 512 MB de memoria** (escenario 1): con 64 platos no debería ser un límite; lo confirmamos.
3. **El gateway como instancia única**: un solo punto de entrada; lo vamos a mirar en CPU.

### Alternativas de escalado (según lo que encontremos)

| Si el límite es… | Alternativa |
| --- | --- |
| Lecturas de Catálogo | Caché ([ADR-007](ADR-007-cache.md)) y más copias ([ADR-012](ADR-012-balanceo-de-carga.md)) |
| Búsqueda | Más copias de Búsqueda detrás de NGINX; Solr con réplicas |
| Bloqueos en Inventario | No se escala con más copias del servicio, porque el cuello está en la fila: lotes de stock por ingrediente o reservas por cocina en tablas separadas. Lo evaluamos solo si los números lo piden |
| Gateway | Una segunda instancia detrás de un balanceador del proveedor de nube |

### Estimación de costos (primera versión)

Para la presentación final hay que desplegar en la nube con una URL pública. Sumamos la memoria aproximada de cada contenedor:

| Componente | Memoria aproximada |
| --- | --- |
| Solr (512 MB de heap) | 0,8–1 GB |
| MySQL | 0,4–0,6 GB |
| MongoDB | 0,3–0,5 GB |
| RabbitMQ | 0,15–0,25 GB |
| Prometheus, Grafana y Jaeger | 0,4–0,6 GB |
| Servicios en Go (7 contenedores, contando 2 copias de Catálogo) | 0,15–0,3 GB en total |
| NGINX, Memcached, Mailpit | 0,1–0,2 GB |
| **Total** | **≈ 2,5–3,5 GB** |

Esta tabla estima la memoria para el entorno completo. El [despliegue de Lu](../../infra/nube/README.md) usa Azure for Students, región Chile Central, una VM Standard B2als v2 con 2 vCPU, 4 GiB de RAM y 2 GiB de swap. Publica solo la API de lectura. Las corridas locales de este ADR se realizaron en un entorno distinto.

La guía del despliegue registra un costo aproximado de USD 0,053 por hora encendida, unos USD 38 por mes de uso continuo con el crédito estudiantil. Al detener la VM quedan los cargos de disco e IP según esa guía. Para completar el gasto efectivamente facturado faltan las horas del período y el desglose de Azure Cost Management.

## Alternativas que pensamos

| Alternativa | A favor | En contra | Qué decidimos |
| --- | --- | --- | --- |
| **`hey` o `ab`** | Muy simples | Solo una URL por vez, sin escenarios ni umbrales | k6 |
| **JMeter** | Muy completo | Interfaz pesada; los escenarios no quedan como código fácil de revisar | k6 |
| **Un programa propio en Go** | Sin herramientas nuevas | Hay que programar lo que k6 ya trae (subida de usuarios, percentiles) | k6 |
| **Kubernetes con autoescalado** | Escala solo | Mucha infraestructura para el trabajo; el enunciado lo deja como desafío opcional | Docker Compose en una máquina |

## Consecuencias

- Los objetivos del SPEC se convierten en umbrales que la prueba verifica.
- El entorno local permite medir el carrito con Inventario y Pedidos. El despliegue público cubre las lecturas.
- Una sola máquina en la nube es un punto único de falla; lo aceptamos para el trabajo.

## Cómo lo vamos a validar

- **Entrega 2:** dejar listo el escenario 1 y correrlo contra la API publicada en local.
- **Presentación final:** adjuntar una captura del tablero para la corrida que se quiera mostrar y completar el gasto facturado del despliegue cuando esté disponible.

## Referencias

- Clase 6: réplicas y balanceo para atender más tráfico.
- Clase 3: medir antes y después de una caché.
- Clase 7: verificar la resiliencia bajo carga.
- Clase 2: bloqueos y concurrencia en la base relacional.
