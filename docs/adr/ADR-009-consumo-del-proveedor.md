# ADR-009 — Lo que consumimos de otro grupo

- **Estado:** Propuesta. Se completa cuando la cátedra asigne el grupo proveedor y conozcamos su contrato
- **Fecha:** 2026-10-16
- **Responsable:** Lucía Rucci
- **Decisión del enunciado:** D9 (cómo incorporamos la capacidad externa, cómo nos adaptamos a su contrato y qué hacemos ante errores, cambios o caídas)
- **Relacionado con:** [ADR-005](ADR-005-comunicacion-entre-servicios.md) · [ADR-010](ADR-010-resiliencia.md) · [ADR-008](ADR-008-contrato-propio.md) · [SPEC](../../SPEC.md) (HU29)

## Contexto

El enunciado pide incorporar al flujo principal una capacidad que publica otro grupo. Todavía no sabemos cuál es: la asigna la cátedra. Sí sabemos lo que el enunciado exige de esa integración:

- un test de contrato capaz de detectar cambios incompatibles;
- tiempos de espera y tratamiento de errores definidos;
- decidir qué ve el usuario cuando el proveedor falla;
- demostrarlo en la jornada de fallas controladas.

Es un sistema que **no controlamos**: puede cambiar, ponerse lento o caerse sin avisarnos. Por eso definimos ahora **cómo** lo vamos a integrar, independientemente de qué capacidad sea, y dejamos para el ADR final los detalles del contrato concreto.

## Decisión

### 1. Quién lo llama y desde dónde

- **Pedidos** llama al proveedor, como un paso más de la confirmación del pedido (HU29). Nunca el frontend ni nuestro gateway (lo pide el enunciado y lo dice [ADR-005](ADR-005-comunicacion-entre-servicios.md)).
- Si la capacidad que nos asignan no tiene sentido en la confirmación (por ejemplo, es una consulta), la ubicamos en el paso del flujo principal donde aporte, y actualizamos este ADR.

### 2. Una capa propia entre su contrato y nuestro dominio (anticorrupción)

Pedidos es hexagonal ([ADR-002](ADR-002-arquitectura-interna.md)): el núcleo define un **puerto** con nuestras palabras (por ejemplo, `ServicioExterno.Consultar(...)`) y un **adaptador** traduce a su contrato. Así:

- un cambio en sus nombres o formatos se arregla en el adaptador, sin tocar las reglas de Pedidos;
- sus códigos de error se traducen a los nuestros: "no disponible", "rechazado por el proveedor", "resultado desconocido";
- en los tests reemplazamos el adaptador por uno falso.

### 3. Contrato y test de contrato

- Guardamos **su OpenAPI** (la versión que usamos) en `docs/contracts/proveedor/`, con la fecha y la versión.
- Un **test de contrato** compara las solicitudes que arma nuestro adaptador y las respuestas que sabe interpretar con ese OpenAPI. Si publican una versión nueva, el test corre contra ella y avisa si algo dejó de ser compatible.
- Para trabajar sin depender de ellos, levantamos **su mock con Prism**, igual que hicimos con el nuestro.
- La URL y la API key van en `PROVIDER_BASE_URL` y `PROVIDER_API_KEY` (`.env`), nunca en el repo.

### 4. Tiempos, reintentos y fallas

| Situación | Qué hacemos |
| --- | --- |
| Tiempo de espera | Lo que quede del plazo total de 3 s de la confirmación, con un máximo por intento (a definir con su latencia real, como referencia 800 ms) |
| Reintentos | Solo si la operación es segura de repetir (lectura, o escritura con clave de idempotencia que ellos acepten) y queda tiempo. Uno como máximo, con espera creciente y azar |
| Circuit breaker | Uno propio para el proveedor: si fallan la mitad de las últimas llamadas, dejamos de llamarlo 10 s ([ADR-010](ADR-010-resiliencia.md)) |
| Error del negocio de ellos (4xx) | No se reintenta; se traduce a un mensaje claro |
| Timeout | **No es un fracaso:** no sabemos si se hizo. Si la operación cambia algo, guardamos el paso como pendiente y lo resolvemos con la recuperación de la confirmación, igual que con Inventario |
| Cambio incompatible detectado | El test de contrato falla en CI; no se sube el cambio hasta adaptar el adaptador |

### 5. Qué ve el usuario

- **Nunca perdemos el pedido** por una falla del proveedor (HU29).
- Si el paso es **esencial**, la confirmación queda en "Estamos confirmando tu pedido" y se reintenta en segundo plano; si al final no se puede, se compensa (se devuelve el stock) y se avisa con un mensaje claro.
- Si el paso **no es esencial** (por ejemplo, una información extra), el pedido se confirma igual y se muestra la parte del proveedor como "no disponible por ahora".

Cuál de los dos casos aplica lo decidimos cuando conozcamos la capacidad asignada.

## Alternativas que pensamos

| Alternativa | A favor | En contra | Qué decidimos |
| --- | --- | --- | --- |
| **Llamarlo desde el frontend** | Menos código en el backend | Expone su API key y no controlamos errores ni tiempos; el enunciado no lo permite | La descartamos |
| **Usar sus tipos directo en nuestro dominio** | Menos traducción | Cualquier cambio suyo nos rompe las reglas | La descartamos: adaptador |
| **Probar solo a mano contra su servicio** | Rápido | No detecta cambios incompatibles a tiempo | La descartamos: test de contrato |
| **Hacerlo por mensajes** | Desacopla | Depende de lo que publiquen; casi seguro es HTTP | Lo decidimos con su contrato |

## Consecuencias

- Lo que no conocemos todavía queda aislado en un adaptador: el resto del diseño no espera a la asignación.
- Hay que mantener una copia de su contrato y su mock.
- La confirmación del pedido suma un paso que puede quedar en estado "pendiente".

## Cómo lo vamos a validar

- **Cuando nos asignen el proveedor:** completar este ADR con su contrato, el paso del flujo, los tiempos reales y el mensaje al usuario; agregar el test de contrato.
- **Jornada de fallas:** con el proveedor caído o lento, mostrar que el pedido no se pierde y que el usuario ve el mensaje acordado; registrar el resultado en el POSTMORTEM.

## Referencias

- Enunciado, sección 4: integración entre grupos.
- Clase 7: presupuesto de tiempo, "un timeout no es un fracaso", reintentos solo si es seguro, circuit breaker, fallback y degradación controlada.
- Clase 8: puertos y adaptadores para aislar dependencias externas.
