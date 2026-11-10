# ADR-010 — Qué pasa cuando algo falla

- **Estado:** Aceptada e implementada en todos los servicios. Búsqueda, Catálogo y el gateway desde la Entrega 2; Pedidos, Inventario y Notificaciones en la entrega final. Validada en la jornada de fallas del 2026-11-10 ([POSTMORTEM](../../POSTMORTEM.md))
- **Fecha:** 2026-10-16
- **Responsable:** Lucía Rucci
- **Decisión del enunciado:** D10 (comportamiento ante fallas y mecanismos para limitar su propagación o degradar el sistema de forma controlada)
- **Relacionado con:** [ADR-005](ADR-005-comunicacion-entre-servicios.md) · [ADR-009](ADR-009-consumo-del-proveedor.md) · [ADR-011](ADR-011-observabilidad.md) · [ARCHITECTURE](../ARCHITECTURE.md) (sección 10)

## Contexto

En un sistema distribuido siempre hay algo que falla o se pone lento: una base, Solr, otro servicio, el proveedor. **La lentitud puede ser peor que un error**: si nadie corta la espera, los pedidos se acumulan, se agotan las conexiones y la falla de una pieza se propaga a todas (falla en cascada).

Tenemos dos objetivos que no se pueden negociar ([SPEC](../../SPEC.md), atributos de calidad):

- **Ninguna pantalla espera más de 3 s.**
- **Lo que toca el stock nunca se inventa:** ante la duda, no se reserva ni se confirma; se informa.

## Decisión

Combinamos cinco mecanismos y una regla de degradación. Lo marcado ✅ está implementado y probado.

### 1. Tiempos máximos y plazo total

| Dónde | Límite | Estado |
| --- | --- | --- |
| Gateway → servicios | Conexión 1 s, respuesta 3 s | ✅ |
| Gateway → verificación de API key | 1 s | ✅ |
| Búsqueda → Solr | 2 s por llamada | ✅ |
| Catálogo → MongoDB | 2 s para elegir servidor y por operación | ✅ |
| Búsqueda → Catálogo (reconstrucción, trabajo de fondo) | 5 s por intento | ✅ |
| Pedidos: plazo total de cada solicitud | 3 s, propagado a todas sus llamadas con `context.WithTimeout` ([ADR-005](ADR-005-comunicacion-entre-servicios.md)) | ✅ |
| Pedidos → Catálogo | 500 ms por intento | ✅ |
| Pedidos → Inventario | 800 ms por intento; el descuento al confirmar, como mucho 2 s con sus reintentos (deja 1 s para guardar el pedido) | ✅ |
| Pedidos → proveedor (factura) | 5 s por intento, en segundo plano: fuera del plazo del usuario ([ADR-009](ADR-009-consumo-del-proveedor.md)) | ✅ |

**Cancelar la espera no deshace el efecto**: si una llamada que cambia algo no contesta, el resultado es *desconocido* y se consulta antes de repetir o compensar.

### 2. Reintentos: solo si es seguro y útil

- Se reintenta un error **pasajero** (conexión, timeout, 503), nunca un error del negocio (sin stock, datos inválidos) ni de permisos.
- **Reintenta uno solo**: el que llama a la dependencia. Ni el gateway ni el frontend reintentan, para no multiplicar los intentos.
- Espera creciente con un poco de azar (*jitter*), dentro del plazo que queda.
- Toda operación que cambia algo lleva **clave de idempotencia** ([ADR-005](ADR-005-comunicacion-entre-servicios.md)).
- ✅ La reconstrucción del índice reintenta 3 veces (0,5 s, 1 s, 2 s), y no reintenta un `4xx` de Catálogo, que es definitivo. El arranque de Búsqueda y de Catálogo reintenta hasta que Solr, Catálogo y MongoDB responden (1, 2, 4, 8 y 10 s).
- ✅ Pedidos: descontar en Inventario reintenta con `cenkalti/backoff` (100 ms, 200 ms, 400 ms… con ±50 % de azar, hasta 2 s) y siempre la misma `Idempotency-Key`. Reservar, ajustar y liberar reintentan una sola vez con la misma clave. Lo que queda pendiente (una confirmación, una devolución, un aviso, una factura) lo retoma un proceso en segundo plano.

### 3. Circuit breaker por operación

Si en una ventana de al menos 10 llamadas falla la mitad, el circuito se **abre** 10 s: se responde un error al instante sin llamar. Después deja pasar unas pocas de prueba (**semiabierto**) y, si funcionan, se cierra. Hay uno **por operación**, porque cada una falla distinto. Los errores del negocio (una reserva vencida, una factura rechazada) no cuentan como fallas.

✅ Está en **Pedidos**, que es el que llama a las dependencias críticas, con `sony/gobreaker`, en las dos operaciones que se repiten mucho cuando la dependencia está caída: **descontar en Inventario** (`inventario-consumir`) y **la factura del proveedor** (`proveedor-comprobantes`). Reservar y ajustar no lo necesitaron: las pide el usuario de a una, con un solo reintento, y si Inventario no contesta reciben `503` en menos de 2 s. Búsqueda y Catálogo tampoco: sus llamadas ya tienen límite de tiempo y responden 503 enseguida.

### 4. Aislar la capacidad (bulkhead)

- ✅ Un cliente HTTP **por dependencia** en Pedidos (Catálogo, Inventario y el proveedor), cada uno con su propio tiempo máximo: si el proveedor se pone lento, no frena las llamadas a Inventario. Además la factura va en segundo plano, así que su lentitud nunca llega al usuario.
- ✅ Una sola reconstrucción del índice a la vez: si ya hay una en curso, `POST /internal/reindexar` responde `409`.
- ✅ El gateway limita la API publicada a 60 solicitudes por minuto por clave y 64 KiB de cuerpo. No es un bulkhead, pero evita que un consumidor externo sature Búsqueda.

### 5. Degradar con honestidad

| Si falla… | Qué hacemos | Qué ve el usuario |
| --- | --- | --- |
| ✅ Solr | `503` con `Retry-After`, **nunca una lista vacía** | "La búsqueda no está disponible". Se puede navegar el menú desde Catálogo |
| ✅ Solr, en `POST /v1/proximidad` sin `incluirDarkKitchen` | Se responde igual: ese cálculo no usa Solr | Nada |
| ✅ Catálogo durante la reconstrucción | Se conserva el índice anterior | Nada (los datos pueden estar atrasados) |
| ✅ Búsqueda o Catálogo caído | El gateway responde `503` con Problem Details | Mensaje de servicio no disponible |
| ✅ MongoDB | Catálogo responde `503`; `/ready` falla | Mensaje de servicio no disponible |
| ✅ Inventario | No se reserva a ciegas: agregar al carrito responde `503`. Si cae durante la confirmación, no se dice que falló: queda pendiente y se completa sola cuando vuelve. Cancelar funciona igual y la devolución queda anotada | "El servicio no está disponible" al armar el carrito; "Estamos confirmando tu pedido" al confirmar |
| ✅ Usuarios | **Se rechaza** (falla cerrada): el gateway responde `503` a los 300 ms y no deja pasar a nadie sin verificar | No se puede iniciar sesión; las rutas públicas siguen andando |
| ✅ Memcached | Se lee de MongoDB ([ADR-007](ADR-007-cache.md)) | Nada |
| ✅ RabbitMQ / correo | Outbox, reintentos (5 s, 30 s, 2 min) y DLQ ([ADR-005](ADR-005-comunicacion-entre-servicios.md)) | Nada en el pedido; el mail llega más tarde |
| ✅ Proveedor | La factura queda pendiente y sale sola cuando vuelve ([ADR-009](ADR-009-consumo-del-proveedor.md)) | El pedido se confirma igual; la factura figura "pendiente" |

La tabla completa por dependencia está en la sección 10 de [ARCHITECTURE](../ARCHITECTURE.md).

### 6. Salud y reinicio

- ✅ Cada servicio con API tiene `/health` (el proceso está vivo) y `/ready` (sus dependencias responden). Notificaciones no tiene API: si se cae, se nota porque los mensajes se acumulan en su cola. Docker Compose usa `/health` para marcar el contenedor como sano, y el gateway no arranca hasta que Búsqueda y Catálogo lo están. `/ready` sirve para saber si puede atender: un servicio sin Solr está vivo pero no listo, y reiniciarlo no arregla Solr.
- ✅ `restart: unless-stopped` en Docker Compose: si un proceso termina, Docker lo vuelve a levantar. (Docker Compose no reinicia un contenedor solo por estar "no sano"; eso lo haría un orquestador como Kubernetes.)

### Librerías

Para Pedidos usamos `sony/gobreaker` (circuit breaker) y `cenkalti/backoff` (espera creciente con azar). Son **[Adicional]**: preferimos usar librerías en lugar de programar estos mecanismos a mano, y las dos son chicas y muy usadas. En Búsqueda alcanza con el reintento simple de la reconstrucción, que es trabajo de fondo.

## Alternativas que pensamos

| Alternativa | A favor | En contra | Qué decidimos |
| --- | --- | --- | --- |
| **Reintentar en todas las capas** (gateway, servicio, cliente) | "Más chances" | Multiplica los intentos y empeora una caída | Reintenta solo el que llama |
| **Un circuit breaker por servicio** | Menos configuración | Una operación rota abriría el circuito de las que andan | Uno por operación |
| **Devolver lista vacía si Solr falla** | El frontend no se rompe | Miente: parece que no hay platos | `503` |
| **Dejar pasar si Usuarios no responde** (falla abierta) | No se corta el servicio | Cualquiera entraría sin login | Falla cerrada |
| **Service mesh (Istio, Linkerd)** | Resiliencia sin código | Demasiada infraestructura para el trabajo | La descartamos |

## Consecuencias

- Una falla queda en su dependencia y el usuario recibe una respuesta clara en menos de 3 s.
- Hay más estados para mostrar ("estamos confirmando", "no disponible por ahora") y más casos para probar.
- Los valores (ventanas, porcentajes, tiempos) son iniciales: los ajustamos con lo que medimos ([ADR-011](ADR-011-observabilidad.md), [ADR-013](ADR-013-capacidad-y-costos.md)).

## Cómo lo validamos

- **Entrega 2 (hecho):** tests de Búsqueda con Solr caído (503 con `Retry-After`, nunca lista vacía), proximidad sin Solr, Catálogo caído en la mitad de la reconstrucción y Catálogo con MongoDB caído. Con el gateway: servicio caído → 503 con Problem Details.
- **Entrega final (hecho):** tests de Pedidos con Inventario y el proveedor caídos o lentos (reintentos, circuit breaker y respuesta en menos de 3 s), y la integración con MySQL real.
- **Jornada de fallas (hecho, 2026-11-10):** con `scripts/fallas/jornada.mjs` apagamos Inventario, RabbitMQ, el correo y el proveedor. Con Inventario caído, agregar al carrito respondió `503` en 1,6 s y confirmar respondió "Estamos confirmando tu pedido" en 2,1 s; el pedido se completó solo 13 s después de que volvió. Con RabbitMQ caído los pedidos siguieron andando y el outbox se vació 36 s después de que volvió. El correo llegó tarde (corte corto) o terminó en la DLQ (corte largo), y la factura salió 7 s después de que volvió el proveedor. Ninguna caída produjo errores 5xx. El detalle está en el [POSTMORTEM](../../POSTMORTEM.md).

