# ADR-011 — Observabilidad: logs, trazas, métricas y alertas

- **Estado:** Primera versión. Logs y trazas implementados (Entrega 2); métricas, tablero y alertas para la presentación final
- **Fecha:** 2026-10-16
- **Responsable:** Guillermina Sánchez
- **Decisión del enunciado:** D11 (estrategia de logs, métricas, trazas y tableros, objetivo de servicio y criterios de alerta)
- **Relacionado con:** [ADR-005](ADR-005-comunicacion-entre-servicios.md) · [ADR-006](ADR-006-busqueda.md) · [ADR-007](ADR-007-cache.md) · [ADR-010](ADR-010-resiliencia.md) · [ADR-012](ADR-012-balanceo-de-carga.md) · [README](../../README.md#logs-y-trazas)

## Contexto

Una solicitud a la API publicada pasa por el gateway, Búsqueda y Solr; una reconstrucción del índice, por Búsqueda, Catálogo y MongoDB. Cuando algo falla o tarda, el error puede estar en cualquiera de esas piezas. La Clase 1 lo plantea como el costo de los microservicios: hace falta **trazabilidad distribuida**, propagando un identificador por todos los servicios, y una observabilidad centralizada.

Nuestro objetivo de calidad ([SPEC](../../SPEC.md)) es **encontrar la causa de una falla en menos de 10 minutos**. Además, no podemos guardar datos sensibles: contraseñas, tokens, API keys, ni las coordenadas que mandan otros grupos (Clase 6: no registrar lo que no es necesario para la traza).

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

**[Adicional]:** OpenTelemetry y Jaeger no son herramientas de la materia. La Clase 1 pide trazabilidad distribuida propagando un identificador; OpenTelemetry es el estándar para hacerlo y Jaeger, la forma más simple de ver las trazas en local.

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

## Referencias

- Clase 1: observabilidad centralizada y trazabilidad distribuida.
- Clase 4: identificador de correlación en los mensajes.
- Clase 6: qué registrar y qué no en el borde.
- Clase 7: señales para verificar la resiliencia.
