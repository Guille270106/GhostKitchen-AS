# ADR-013 — Capacidad y costos

- **Estado:** Primera versión: método, escenarios, hipótesis y estimación inicial. Los resultados de la prueba de carga se agregan para la presentación final
- **Fecha:** 2026-10-16
- **Responsable:** Valentina Rey
- **Decisión del enunciado:** D13 (resultados de las pruebas de carga, principal límite identificado, alternativa de escalado y estimación de costos)
- **Relacionado con:** [ADR-003](ADR-003-persistencia.md) · [ADR-007](ADR-007-cache.md) · [ADR-011](ADR-011-observabilidad.md) · [ADR-012](ADR-012-balanceo-de-carga.md) · [SPEC](../../SPEC.md) (atributos de calidad)

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

Con margen, alcanza **una máquina virtual de 2 vCPU y 4 GB de RAM** corriendo Docker Compose. Como referencia, en DigitalOcean un Droplet Basic de 2 vCPU y 4 GiB cuesta **USD 24 por mes** y el de 4 vCPU y 8 GiB, **USD 48 por mes** ([precios](https://www.digitalocean.com/pricing/droplets), consultados el 16/10/2026). Para las semanas de la presentación serían unos USD 24–48, más el tráfico, que con nuestro volumen es despreciable. El proveedor y el tamaño final se eligen al desplegar, y esta estimación se corrige con la memoria real que midamos.

## Alternativas que pensamos

| Alternativa | A favor | En contra | Qué decidimos |
| --- | --- | --- | --- |
| **`hey` o `ab`** | Muy simples | Solo una URL por vez, sin escenarios ni umbrales | k6 |
| **JMeter** | Muy completo | Interfaz pesada; los escenarios no quedan como código fácil de revisar | k6 |
| **Un programa propio en Go** | Sin herramientas nuevas | Hay que programar lo que k6 ya trae (subida de usuarios, percentiles) | k6 |
| **Kubernetes con autoescalado** | Escala solo | Mucha infraestructura para el trabajo; el enunciado lo deja como desafío opcional | Docker Compose en una máquina |

## Consecuencias

- Los objetivos del SPEC se convierten en umbrales que la prueba verifica.
- Hasta que existan Inventario y Pedidos, solo podemos medir lecturas.
- Una sola máquina en la nube es un punto único de falla; lo aceptamos para el trabajo.

## Cómo lo vamos a validar

- **Entrega 2:** dejar listo el escenario 1 y correrlo contra la API publicada en local.
- **Presentación final:** los cuatro escenarios con sus resultados en este ADR, el principal límite con evidencia del tablero y la estimación de costos con la memoria real.

## Referencias

- Clase 6: réplicas y balanceo para atender más tráfico.
- Clase 3: medir antes y después de una caché.
- Clase 7: verificar la resiliencia bajo carga.
- Clase 2: bloqueos y concurrencia en la base relacional.
