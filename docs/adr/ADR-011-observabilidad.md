# ADR-011 — Observabilidad: logs, trazas, métricas y alertas

- **Estado:** Primera versión. Logs y trazas implementados (Entrega 2); `/metrics` en los servicios (G19); Prometheus, tablero en Grafana y alertas (G20)
- **Fecha:** 2026-10-16
- **Responsable:** Guillermina Sánchez
- **Decisión del enunciado:** D11 (estrategia de logs, métricas, trazas y tableros, objetivo de servicio y criterios de alerta)
- **Relacionado con:** [ADR-005](ADR-005-comunicacion-entre-servicios.md) · [ADR-006](ADR-006-busqueda.md) · [ADR-007](ADR-007-cache.md) · [ADR-010](ADR-010-resiliencia.md) · [ADR-012](ADR-012-balanceo-de-carga.md) · [README](../../README.md#logs-y-trazas)

## Contexto

Una solicitud a la API publicada pasa por el gateway, Búsqueda y Solr; una reconstrucción del índice, por Búsqueda, Catálogo y MongoDB. Cuando algo falla o tarda, el error puede estar en cualquiera de esas piezas. Ese es el costo de usar microservicios: hace falta **trazabilidad distribuida**, propagando un identificador por todos los servicios, y una observabilidad centralizada.

Nuestro objetivo de calidad ([SPEC](../../SPEC.md)) es **encontrar la causa de una falla en menos de 10 minutos**. Además, no podemos guardar datos sensibles: contraseñas, tokens, API keys, ni las coordenadas que mandan otros grupos (no registrar lo que no es necesario para la traza).

## Decisión

Usamos las tres señales, cada una para una pregunta distinta:

| Señal | Pregunta que responde | Herramienta | Estado |
| --- | --- | --- | --- |
| **Logs** | ¿Qué pasó en esta solicitud y con qué error? | JSON por salida estándar (`log/slog` en Go, `log_format` en NGINX) | ✅ Entrega 2 |
| **Trazas** | ¿Por dónde pasó y dónde se demoró? | OpenTelemetry + Jaeger | ✅ Entrega 2 |
| **Métricas** | ¿El sistema está sano en general? ¿Cumple el objetivo? | Prometheus + Grafana | Presentación final |

### 1. Logs estructurados y correlacionados

Una línea JSON por solicitud, con los mismos campos en todos los servicios:

```json
{"time":"…","level":"INFO","msg":"solicitud","servicio":"busqueda","correlation_id":"mi-prueba-1",
 "trace_id":"0af7651916cd43dd8448eb211c80319c","span_id":"9e84c856448291d1",
 "metodo":"POST","ruta":"/v1/proximidad","estado":200,"duracion_ms":3.2}
```

- **`correlation_id`**: identificador de negocio. Lo manda el cliente en `X-Correlation-Id` o lo genera el gateway; viaja entre servicios y en los mensajes ([ADR-005](ADR-005-comunicacion-entre-servicios.md)), y se devuelve en la respuesta. Sirve para que el otro grupo nos diga "falló la solicitud X".
- **`trace_id`**: identificador técnico de la traza. Une cada log con su traza en Jaeger.
- **`ruta`** es la plantilla (`/v1/sedes/:sedeId/platos`), sin parámetros ni cuerpo. **No** se registran coordenadas, textos de búsqueda, API keys, tokens ni contraseñas.
- Errores: nivel `ERROR` para respuestas 5xx, con el detalle técnico; el usuario solo ve el Problem Details.
- Por ahora se consultan con `docker compose logs` y se filtran por `correlation_id` o `trace_id`. Para la nube, el proveedor elegido o Loki (se decide al desplegar).

### 2. Trazas distribuidas

- **OpenTelemetry** en cada servicio y en el gateway (módulo oficial de NGINX). Es el estándar abierto: no ata el código a una herramienta de visualización.
- La traza viaja en el header **W3C `traceparent`**: el gateway la empieza, Búsqueda la continúa, y Búsqueda la pasa a Catálogo cuando le pide datos.
- **Spans:** uno por solicitud en cada servicio (nombre = plantilla de la ruta), uno por consulta a Solr (`solr platos/select`, sin la URL con parámetros), uno por llamada a Catálogo y uno por comando de Catálogo a MongoDB. La reconstrucción del índice tiene su propio span (`reconstruir índice`).
- Cada span lleva `ghostkitchen.correlation_id`, para encontrar una traza a partir del id de negocio.
- **Privacidad:** el módulo de NGINX guarda la URL completa en el span, con sus parámetros. Por eso el gateway solo crea su span cuando la solicitud no tiene parámetros; las demás se trazan desde Búsqueda.
- **Muestreo:** en desarrollo, todas las trazas. En la nube, un porcentaje (por ejemplo, 10 %) más todas las que terminen con error.
- **Jaeger** (all-in-one) recibe las trazas por OTLP y las muestra en http://localhost:16686.

### 3. Métricas y tablero (presentación final)

Prometheus lee `/metrics` de cada servicio; Grafana muestra un tablero con:

| Panel | Métrica | Para qué |
| --- | --- | --- |
| Tráfico, errores y duración por ruta | Solicitudes por segundo, % de 5xx, p50/p95/p99 | Ver si se cumple el objetivo (método RED) |
| Dependencias | Duración y errores de Solr, MongoDB, Catálogo | Saber qué pieza está lenta |
| Instancias de Catálogo | Solicitudes por copia (desde el log del gateway y desde cada copia) | Mostrar el balanceo ([ADR-012](ADR-012-balanceo-de-carga.md)) |
| Caché | Aciertos/fallos, latencia con y sin caché, consultas a MongoDB | Decidir si la caché sirve ([ADR-007](ADR-007-cache.md)) |
| Índice | Atraso entre el cambio en Catálogo y su llegada a Solr | Controlar los 5 s aceptados ([ADR-006](ADR-006-busqueda.md)) |
| Mensajería | Mensajes pendientes, reintentos y DLQ | Ver un consumidor detenido |
| Pedidos | Confirmaciones pendientes y recuperadas | Ver pedidos a medias |

#### Cómo quedó `/metrics` (G19)

Cada servicio en Go (Usuarios, Catálogo, Búsqueda, Inventario y Pedidos) publica `GET /metrics` con `prometheus/client_golang`. Solo se llega desde adentro de Docker: el gateway no la publica. Notificaciones la suma cuando exista su código (V13), con el mismo paquete `internal/metricas`.

| Métrica | Tipo | Etiquetas | Panel |
| --- | --- | --- | --- |
| `http_requests_total` | contador | `method`, `route`, `status` | Tráfico y errores (RED) |
| `http_request_duration_seconds` | histograma (5 ms a 5 s, con 300 ms, 500 ms y 3 s) | `method`, `route` | Duración p50/p95/p99 |
| `catalogo_cache_total` | contador | `tipo` (cocinas, cocina, menu, plato), `resultado` (acierto, fallo, error) | Caché ([ADR-007](ADR-007-cache.md)) |
| `busqueda_solr_duration_seconds` | histograma | `operacion` (ej. `platos/select`), `resultado` | Dependencias |
| `busqueda_eventos_total` | contador | `tipo`, `resultado` (aplicado, rechazado, reintento) | Mensajería |
| `busqueda_indice_atraso_seconds` | histograma (0,1 a 60 s, con 5 s) | — | Índice: atraso del cambio hasta Solr |

Además vienen las de Go y del proceso (memoria, goroutines, CPU). Decisiones:

- **La ruta es la plantilla de Gin** (`/platos/:id`), nunca la URL real: no se registran ids, texto buscado ni coordenadas, y la cantidad de series queda acotada. Las rutas que no existen van todas a `sin_ruta`.
- **El servicio y la copia no van como etiqueta:** los agrega Prometheus al leer (`job` e `instance`). Así las dos copias de Catálogo se ven por separado sin cambiar el código.
- **El middleware va primero**, antes de `gin.Recovery`: una solicitud que termina en panic cuenta como 500.
- **Las lecturas de `/metrics` no cuentan** como tráfico ni crean trazas en Jaeger.

#### Cómo quedaron Prometheus, el tablero y las alertas (G20)

- **Prometheus** (http://localhost:9090) lee `/metrics` de cada servicio cada 15 s (`infra/prometheus/prometheus.yml`) y guarda 7 días, la ventana de los objetivos. Cada servicio es un `job` y cada copia una `instance`: las dos copias de Catálogo son `catalogo-1:8082` y `catalogo-2:8082`.
- **RabbitMQ** se lee con su plugin de Prometheus (`/metrics/detailed?family=queue_coarse_metrics`, puerto 15692), que da los mensajes de cada cola y de cada DLQ. No hizo falta código.
- **Grafana** (http://localhost:3000) arranca con Prometheus configurado y con el tablero **GhostKitchen** como inicio (`infra/grafana`). Se ve sin entrar, en modo lectura. El tablero está en el repo; un cambio hecho en la pantalla no se guarda.
- **Paneles:**
  - **Tráfico:** solicitudes por segundo, % de 5xx, p95 por servicio, rutas más lentas y respuestas por código.
  - **Catálogo:** solicitudes por copia, copias arriba, % de aciertos de la caché y caché por resultado.
  - **Búsqueda:** p95 de Solr por operación, atraso del índice (p50 y p95, con la línea de 5 s) y mensajes aplicados, rechazados o reintentados.
  - **Mensajería y estado:** mensajes en cada cola, mensajes en las DLQ, servicios arriba y alertas activas.
- **Alertas** (`infra/prometheus/alertas.yml`): las cinco de la tabla de arriba.

  | Alerta | Expresión | Diferencia con lo planeado |
  | --- | --- | --- |
  | `MuchosErrores5xx` | Por servicio | Solo con más de 0,1 solicitudes por segundo, para que dos errores sueltos con poco tráfico no la disparen |
  | `BusquedaLenta` | — | — |
  | `ServicioCaido` | `up == 0` durante 1 minuto | Se usa `up` en lugar de `/ready`: Prometheus no puede leer `/metrics` de esa copia |
  | `MensajesEnDLQ` | — | — |
  | `IndiceAtrasado` | — | — |

  Se ven en http://localhost:9090/alerts y en el panel "Alertas activas". No hay Alertmanager (no manda mails ni mensajes): para el trabajo alcanza con verlas en el tablero.
- **Probado:**
  - `promtool check` de la configuración y de las reglas, y las 22 expresiones del tablero pasadas por el parser de Prometheus.
  - Tests de las alertas con `promtool test rules` (`infra/prometheus/alertas_test.yml`): 10 % de 5xx salta a los 5 minutos y un error suelto no; una copia caída salta al minuto; un mensaje en la DLQ salta a los 5 minutos.
  - Prometheus real contra Usuarios y RabbitMQ: todas las consultas del tablero responden y `ServicioCaido` se disparó para los servicios apagados.
- **Pendiente:** las métricas de Pedidos (confirmaciones pendientes) y de Notificaciones, cuando las publiquen sus servicios.

### 4. Objetivos de servicio (SLO)

| Qué | Objetivo | Ventana |
| --- | --- | --- |
| Disponibilidad de la API publicada | 99 % de las solicitudes sin error 5xx | 7 días |
| Latencia de búsqueda y sedes (API publicada y frontend) | p95 < 300 ms | 7 días |
| Latencia de agregar al carrito | p95 < 500 ms con 100 usuarios | Prueba de carga |
| Atraso del índice | 95 % de los cambios visibles en ≤ 5 s | 7 días |

Salen de los atributos de calidad del [SPEC](../../SPEC.md). Los confirmamos o ajustamos con la prueba de carga ([ADR-013](ADR-013-capacidad-y-costos.md)).

### 5. Criterios de alerta

Alertamos sobre lo que afecta al usuario, no sobre cada error suelto:

| Alerta | Condición | Por qué |
| --- | --- | --- |
| Errores en la API publicada | Más de 5 % de 5xx durante 5 minutos | Consume rápido el objetivo de disponibilidad |
| Búsqueda lenta | p95 > 300 ms durante 10 minutos | Fuera del objetivo de latencia |
| Servicio no listo | `/ready` falla durante más de 1 minuto | Una dependencia caída |
| Mensajes en la DLQ | Hay mensajes durante más de 5 minutos | Algo no se puede procesar ([ADR-005](ADR-005-comunicacion-entre-servicios.md)) |
| Índice atrasado | Atraso > 5 s durante 5 minutos | Fuera de lo aceptado ([ADR-006](ADR-006-busqueda.md)) |

Los umbrales son iniciales: los revisamos después de medir el sistema.

## Alternativas que pensamos

| Alternativa | A favor | En contra | Qué decidimos |
| --- | --- | --- | --- |
| **Solo logs con `correlation_id`** | Lo vimos en la materia; sin herramientas nuevas | Hay que reconstruir a mano el recorrido y no se ve dónde se fue el tiempo | Lo usamos, pero sumamos trazas |
| **Zipkin** | Más simple | Jaeger recibe OTLP directamente y su interfaz muestra mejor las dependencias | Jaeger |
| **SDK de un proveedor (Datadog, New Relic)** | Todo en uno | Ata el código a un proveedor pago | OpenTelemetry |
| **Escribir los logs en archivos dentro del contenedor** | Quedan guardados | Se pierden con el contenedor y no se ven con `docker compose logs` | Salida estándar |
| **Loki ya en la Entrega 2** | Logs centralizados con búsqueda | Una pieza más; `docker compose logs` alcanza para local | Se decide al desplegar en la nube |

**[Adicional]:** OpenTelemetry y Jaeger los agregamos nosotras. Necesitábamos trazabilidad distribuida propagando un identificador; OpenTelemetry es el estándar para hacerlo y Jaeger, la forma más simple de ver las trazas en local.

## Consecuencias

**Lo bueno**

- Desde un log se llega a la traza completa y al revés.
- Podemos mostrar en la defensa por dónde pasó una solicitud y cuánto tardó cada paso.
- No hay datos sensibles en logs ni trazas.

**Lo que aceptamos**

- Más piezas: Jaeger hoy, Prometheus y Grafana después.
- Las librerías de OpenTelemetry suman dependencias a cada servicio.
- Las solicitudes con parámetros no tienen span del gateway.

## Cómo lo validamos

- **Entrega 2 (hecho):** tests que verifican que se continúa la traza recibida, que el log tiene `trace_id`, `span_id` y `correlation_id`, que `/health` no se traza, que los spans de Solr no tienen coordenadas y que la llamada a Catálogo propaga la traza. Probado contra un Jaeger real: una reconstrucción aparece como una sola traza con spans de Búsqueda y de Catálogo.
- **Presentación final:** tablero de Grafana con los paneles de arriba, una alerta funcionando (DLQ o 5xx) y, en la jornada de fallas, encontrar la causa en menos de 10 minutos usando logs y trazas.

