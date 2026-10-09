# ADR-008 — Lo que publicamos para otro grupo

- **Estado:** Aceptada
- **Fecha:** 2026-10-09
- **Responsable:** Lucía Rucci
- **Decisión del enunciado:** D8 (diseño, publicación, compatibilidad y versionado de la capacidad que ofrecemos a otro grupo)
- **Relacionado con:** [contrato](../contracts/v1/openapi.yaml) · [guía para el otro grupo](../contracts/README.md) · [CHANGELOG](../contracts/CHANGELOG.md) · [ADR-001](ADR-001-limites-de-los-servicios.md) · [ARCHITECTURE](../ARCHITECTURE.md) (sección 9)

## Contexto

Cada grupo tiene que publicar algo de su sistema para que otro grupo lo use **sin tener que preguntarnos nada** (enunciado, sección 4). Para eso necesitamos:
- un contrato formal y con versión, guardado en `docs/contracts/`;
- que esté en una URL pública hasta el final de la materia;
- que al otro grupo le sirva para algo importante de su sistema.

Todavía no sabemos qué grupo nos va a usar ni de qué es su sistema. Por eso buscamos algo que:

- **sirva para muchos tipos de sistemas** (eventos, turismo, alojamiento, etc.). Casi todos manejan una **ubicación**: una dirección, un lugar o un evento;
- **sea seguro:** que nadie de afuera pueda tocar nuestro stock ni nuestros pedidos;
- **sea barato** de tener en la nube todo el cuatrimestre.

## Decisión

### Qué publicamos: "Cocina más cercana y platos disponibles"

Con una ubicación (latitud y longitud), el otro sistema puede saber **qué cocina nuestra le queda más cerca y a cuántos km**, y después ver **qué platos se pueden pedir ahora** en esa cocina, con filtros y orden.

Ejemplos de cómo lo podría usar:

- una app de eventos muestra al lado de cada evento "pedí comida en GhostKitchen Cerro (a 2 km)" con sus platos;
- una app de alojamiento sugiere platos veganos de la cocina más cercana;
- un sistema de logística calcula cuántos pedidos podría haber por zona.

### Operaciones (versión 1)

| Operación | Ruta | Para qué |
| --- | --- | --- |
| `listarSedes` | `GET /v1/sedes` | Las 4 cocinas con su ubicación y sus restaurantes |
| `buscarSedeCercana` | `GET /v1/sedes/cercana?lat=&lng=` | La cocina activa más cercana a una ubicación y a cuántos km está |
| `buscarPlatos` | `GET /v1/sedes/{sedeId}/platos` | Los platos de una cocina, con filtros, orden y páginas |
| `obtenerPlato` | `GET /v1/platos/{platoId}` | El detalle de un plato y si está disponible |

### Quién la atiende: Búsqueda

- Búsqueda ya tiene en su índice **la ubicación de las sedes y los platos con su disponibilidad**: no hace falta un servicio nuevo ni consultar a varios servicios en cada solicitud.
- Así, las consultas de otro grupo **no compiten** con las reservas y los pedidos, que es la parte que no puede fallar (Clase 5: la búsqueda no debe cargar a la base que atiende las operaciones críticas).
- En la nube alcanza con subir **Búsqueda y Solr** con los datos de prueba, en lugar de todo el sistema.

### Cómo es el contrato

- **Formato:** **OpenAPI 3.0** en YAML (`docs/contracts/v1/openapi.yaml`). Es un estándar: con ese archivo se pueden generar clientes, un mock y tests.
- **Solo lectura:** todas las operaciones son `GET`, así que el otro grupo puede reintentar sin miedo (Clase 7). **No publicamos reservas ni pedidos**, para que nadie de afuera pueda tocar el stock.
- **Cocina más cercana:** la calcula Solr con la distancia en línea recta. Si ninguna cocina está a menos de 25 km, responde `404 SIN_COBERTURA` (RN24), para no sugerir una cocina que no llega.
- **Autenticación:** cada grupo recibe **su propia API key** por un canal privado y la manda en el header `X-API-Key`. Así podemos saber quién la usa, limitarla o darla de baja. La controla el gateway (Clase 6).
- **Límite de uso:** 60 pedidos por minuto por clave. Si se pasa, responde `429` y le dice cuánto esperar (Clase 6).
- **Errores:** siempre en el mismo formato (*Problem Details*, RFC 9457), con un código fijo:

  | HTTP | Código |
  | --- | --- |
  | 400 | `PARAMETRO_INVALIDO` |
  | 401 | `API_KEY_INVALIDA` |
  | 404 | `SEDE_NO_ENCONTRADA`, `PLATO_NO_ENCONTRADO`, `SIN_COBERTURA` |
  | 429 | `LIMITE_EXCEDIDO` |
  | 503 | `SERVICIO_NO_DISPONIBLE` |

- **Páginas:** `page` (desde 1) y `pageSize` (de 1 a 50, por defecto 20). La respuesta dice el total y cuántas páginas hay.
- **Somos honestos con el dato:** cada plato dice si está `disponible` y cuándo se actualizó. Aclaramos que **puede tener hasta 5 segundos de atraso y que no reserva nada**. Es la misma regla que usamos adentro: la búsqueda sirve para mirar, y el que decide el stock es Inventario (Clase 5).
- **Seguimiento:** si el otro grupo nos manda un `X-Correlation-Id`, se lo devolvemos y queda en nuestros logs, así podemos rastrear un problema juntos.
- **Precios:** en pesos, como número entero (`precio: 12900`) y con `moneda: "ARS"`.

### Cómo lo publicamos

- **Desde hoy (Entrega 1), un mock:** el contrato trae ejemplos de cada respuesta. Con `docker compose up` se levanta **Prism**, que responde esos ejemplos en `http://localhost:4010`. Así el otro grupo puede empezar a integrarse sin esperar a que tengamos el sistema.
- **La versión real (Entrega 2):** la atiende el servicio **Búsqueda**, detrás del gateway NGINX, en `/public/v1`. El gateway controla la API key y el límite.
- **En la nube:** subimos Búsqueda con Solr a un servicio gratuito o barato (Render o Railway; lo decidimos en el ADR-013). La URL pública la agregamos al contrato y a la guía.
- **Disponibilidad:** hacemos lo posible para que esté siempre andando, desde la Entrega 2 hasta el final de la materia. Si tenemos que cortarlo, avisamos al otro grupo con 48 h de anticipación.

### Cómo manejamos los cambios (versionado)

- **La versión va en la ruta** (`/v1`) y además tiene un número (`1.0.0`, `1.1.0`…).
- **Cambios que no rompen nada** (sube el segundo número, la ruta sigue siendo `/v1`): agregar campos opcionales, filtros opcionales, nuevos órdenes, nuevas operaciones o nuevas cocinas. El otro grupo se compromete a **ignorar los campos que no conoce**.
- **Cambios que rompen** (necesitan `/v2`): sacar o renombrar un campo, cambiar su tipo o lo que significa, agregar un parámetro obligatorio o cambiar los códigos de error.
- **Si hay una `v2`, la `v1` sigue andando** hasta el final de la materia, y lo avisamos en el `CHANGELOG` (Clase 6: una versión nueva convive con la anterior hasta su retiro).
- **Historial:** `docs/contracts/CHANGELOG.md` anota cada versión con su fecha. Las versiones viejas no se borran.
- **Para no romper el contrato sin darnos cuenta:** el OpenAPI se valida en cada pull request, y desde la Entrega 2 los tests de Búsqueda comparan sus respuestas con los esquemas del contrato.

## Alternativas que pensamos

| Alternativa | A favor | En contra | Qué decidimos |
| --- | --- | --- | --- |
| **Solo "platos por localidad"**, sin distancia | Más simple | El otro grupo tendría que conocer nuestras localidades; casi todos los sistemas tienen coordenadas | La descartamos y agregamos la cocina más cercana |
| **Dejar reservar o crear pedidos desde afuera** | Más completo | Un sistema que no controlamos podría tocar nuestro stock. Además dependería de que Pedidos, Inventario y Catálogo funcionen juntos para la Entrega 2 | La descartamos para la v1 |
| **Atenderla desde Pedidos** | Pedidos es el centro de la compra | Los datos que publicamos están en Búsqueda; desde Pedidos habría que consultar a Catálogo e Inventario en cada solicitud y competiría con las confirmaciones | La descartamos |
| **Publicar mensajes** en vez de una API | Avisos al instante | El otro grupo tendría que conectarse a nuestro RabbitMQ en la nube | La descartamos |
| **GraphQL** | El otro grupo elige los campos | Más difícil de limitar, cachear y testear | La descartamos |
| **Versión en un header** | URLs más limpias | Es fácil olvidarse de mandarlo | La descartamos |
| **Sin API key** | Más fácil | No podríamos limitar ni saber quién la usa | La descartamos |

## Consecuencias

**Lo bueno**

- El otro grupo puede empezar hoy con el mock, sin preguntarnos nada que no esté en el contrato.
- Con solo una ubicación, cualquier sistema puede sumar "dónde pedir comida cerca".
- Como todo es `GET`, el otro grupo puede reintentar y cachear sin riesgo.
- Publicamos la búsqueda y no las bases, así protegemos el stock y no cargamos la parte que vende.

**Lo que aceptamos**

- La disponibilidad puede tener hasta 5 s de atraso: el otro grupo la tiene que mostrar como orientativa.
- La distancia es en línea recta, no por calles.
- El otro grupo no puede hacer pedidos con nuestra API: solo descubrir qué hay y dónde.
- Hay que mantener un servicio en la nube todo el cuatrimestre.
- Nos comprometemos a no romper la `v1`.

## Cómo lo vamos a validar

- **Entrega 1:** contrato validado con `swagger-cli` 4.0.4 y mock probado con Prism 5.16.0. Las cuatro operaciones y los errores documentados (404, 429, 503, latitud inválida y falta de API key) responden; cómo probarlo está en la [guía del contrato](../contracts/README.md#probar-sin-esperar-al-backend-mock). Que Prism arranque, por sí solo, no sustituye la validación formal de OpenAPI.
- **Entrega 2:** la versión real pasa un test que la compara con el contrato y está en una URL pública.
- **Durante la integración:** si el otro grupo nos pide un cambio, lo anotamos acá con la versión nueva.

## Referencias

- Enunciado, sección 4: integración entre grupos.
- Clase 5: el índice de búsqueda como copia derivada, su atraso aceptable y por qué no cargar la base de las operaciones críticas.
- Clase 6: API Gateway, autenticación, límite de solicitudes y versionado de contratos.
- Clase 7: reintentos seguros e idempotencia.
