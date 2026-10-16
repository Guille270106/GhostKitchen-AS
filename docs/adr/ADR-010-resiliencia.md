# ADR-010 — Qué pasa cuando algo falla

- **Estado:** Aceptada. Implementado lo que corresponde a Búsqueda, Catálogo y el gateway (Entrega 2); el resto se implementa con Pedidos e Inventario
- **Fecha:** 2026-10-16
- **Responsable:** Lucía Rucci
- **Decisión del enunciado:** D10 (comportamiento ante fallas y mecanismos para limitar su propagación o degradar el sistema de forma controlada)
- **Relacionado con:** [ADR-005](ADR-005-comunicacion-entre-servicios.md) · [ADR-009](ADR-009-consumo-del-proveedor.md) · [ADR-011](ADR-011-observabilidad.md) · [ARCHITECTURE](../ARCHITECTURE.md) (sección 10)

## Contexto

En un sistema distribuido siempre hay algo que falla o se pone lento: una base, Solr, otro servicio, el proveedor. La Clase 7 muestra que **la lentitud puede ser peor que un error**: si nadie corta la espera, los pedidos se acumulan, se agotan las conexiones y la falla de una pieza se propaga a todas (falla en cascada).

Tenemos dos objetivos que no se pueden negociar ([SPEC](../../SPEC.md), atributos de calidad):

- **Ninguna pantalla espera más de 3 s.**
- **Lo que toca el stock nunca se inventa:** ante la duda, no se reserva ni se confirma; se informa.

## Decisión

Combinamos cinco mecanismos y una regla de degradación. Lo marcado ✅ ya funciona en la Entrega 2.

### 1. Tiempos máximos y plazo total

| Dónde | Límite | Estado |
| --- | --- | --- |
| Gateway → servicios | Conexión 1 s, respuesta 3 s | ✅ |
| Gateway → verificación de API key | 1 s | ✅ |
| Búsqueda → Solr | 2 s por llamada | ✅ |
| Catálogo → MongoDB | 2 s para elegir servidor y por operación | ✅ |
| Búsqueda → Catálogo (reconstrucción, trabajo de fondo) | 5 s por intento | ✅ |
| Llamadas de Pedidos (Catálogo, Inventario, proveedor) | Lo que quede del plazo total de 3 s, propagado con `context.WithTimeout` ([ADR-005](ADR-005-comunicacion-entre-servicios.md)) | Entrega final |

**Cancelar la espera no deshace el efecto** (Clase 7): si una llamada que cambia algo no contesta, el resultado es *desconocido* y se consulta antes de repetir o compensar.

### 2. Reintentos: solo si es seguro y útil

- Se reintenta un error **pasajero** (conexión, timeout, 503), nunca un error del negocio (sin stock, datos inválidos) ni de permisos.
- **Reintenta uno solo**: el que llama a la dependencia. Ni el gateway ni el frontend reintentan, para no multiplicar los intentos.
- Espera creciente con un poco de azar (*jitter*), dentro del plazo que queda.
- Toda operación que cambia algo lleva **clave de idempotencia** ([ADR-005](ADR-005-comunicacion-entre-servicios.md)).
- ✅ La reconstrucción del índice reintenta 3 veces (0,5 s, 1 s, 2 s). El arranque de Búsqueda y de Catálogo reintenta hasta que Solr, Catálogo y MongoDB responden (1, 2, 4, 8 y 10 s).

### 3. Circuit breaker por operación

Si en una ventana de al menos 10 llamadas falla la mitad, el circuito se **abre** 10 s: se responde un error al instante sin llamar. Después deja pasar unas pocas de prueba (**semiabierto**) y, si funcionan, se cierra. Hay uno **por operación** (reservar, descontar, consultar, proveedor), porque cada una falla distinto. Los errores del negocio no cuentan como fallas.

Lo usamos en **Pedidos** (Entrega final), que es el que llama a las dependencias críticas. Búsqueda y Catálogo no lo necesitan todavía: sus llamadas ya tienen límite de tiempo y responden 503 enseguida.

### 4. Aislar la capacidad (bulkhead)

- Un cliente HTTP **por dependencia**, con su propio límite de conexiones: si el proveedor se pone lento, no ocupa las conexiones que necesita Inventario.
- ✅ Una sola reconstrucción del índice a la vez: si ya hay una en curso, `POST /internal/reindexar` responde `409`.
- ✅ El gateway limita la API publicada a 60 solicitudes por minuto por clave y 64 KiB de cuerpo. No es un bulkhead (Clase 7, 5.1), pero evita que un consumidor externo sature Búsqueda.

### 5. Degradar con honestidad

| Si falla… | Qué hacemos | Qué ve el usuario |
| --- | --- | --- |
| ✅ Solr | `503` con `Retry-After`, **nunca una lista vacía** | "La búsqueda no está disponible". Se puede navegar el menú desde Catálogo |
| ✅ Solr, en `POST /v1/proximidad` sin `incluirDarkKitchen` | Se responde igual: ese cálculo no usa Solr | Nada |
| ✅ Catálogo durante la reconstrucción | Se conserva el índice anterior | Nada (los datos pueden estar atrasados) |
| ✅ Búsqueda o Catálogo caído | El gateway responde `503` con Problem Details | Mensaje de servicio no disponible |
| ✅ MongoDB | Catálogo responde `503`; `/ready` falla | Mensaje de servicio no disponible |
| Inventario | No se reserva ni se confirma | "No podemos tomar pedidos en este momento" |
| Usuarios | **Se rechaza** (falla cerrada): no dejamos pasar a nadie sin verificar | No se puede iniciar sesión |
| Memcached | Se lee de MongoDB ([ADR-007](ADR-007-cache.md)) | Nada |
| RabbitMQ / correo | Outbox y reintentos ([ADR-005](ADR-005-comunicacion-entre-servicios.md)) | El mail llega más tarde |
| Proveedor | [ADR-009](ADR-009-consumo-del-proveedor.md) | Mensaje claro; el pedido no se pierde |

La tabla completa por dependencia está en la sección 10 de [ARCHITECTURE](../ARCHITECTURE.md).

### 6. Salud y reinicio

- ✅ Cada servicio tiene `/health` (el proceso está vivo) y `/ready` (sus dependencias responden). Docker Compose usa `/health` para marcar el contenedor como sano, y el gateway no arranca hasta que Búsqueda y Catálogo lo están. `/ready` sirve para saber si puede atender: un servicio sin Solr está vivo pero no listo, y reiniciarlo no arregla Solr.
- ✅ `restart: unless-stopped` en Docker Compose: si un proceso termina, Docker lo vuelve a levantar. (Docker Compose no reinicia un contenedor solo por estar "no sano"; eso lo haría un orquestador como Kubernetes.)

### Librerías

Para Pedidos usamos `sony/gobreaker` (circuit breaker) y `cenkalti/backoff` (espera creciente con azar). Son **[Adicional]**: no las vimos en clase, pero la cátedra pidió usar librerías en lugar de programar estos mecanismos a mano, y las dos son chicas y muy usadas. En Búsqueda alcanza con el reintento simple de la reconstrucción, que es trabajo de fondo.

## Alternativas que pensamos

| Alternativa | A favor | En contra | Qué decidimos |
| --- | --- | --- | --- |
| **Reintentar en todas las capas** (gateway, servicio, cliente) | "Más chances" | Multiplica los intentos y empeora una caída (Clase 7, 3.4) | Reintenta solo el que llama |
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
- **Jornada de fallas (final):** apagar Inventario, RabbitMQ, el correo y el proveedor; medir el tiempo de respuesta y el mensaje al usuario; registrar todo en el POSTMORTEM.

## Referencias

- Clase 7: fallas en cascada, plazos y cancelación, reintentos con backoff y jitter, idempotencia, circuit breaker, bulkhead, fallback y degradación, falla abierta y cerrada.
- Clase 6: chequeos de salud.
