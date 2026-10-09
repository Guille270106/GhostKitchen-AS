# Búsqueda

**Responsable:** Guillermina Sánchez (índice y búsqueda) · Lucía Rucci (capacidad publicada).

> Entrega 1: diseño preliminar; todavía no hay implementación. Las carpetas, rutas, configuración y pruebas indicadas son previstas para el desarrollo desde la Entrega 2.

Sirve para **encontrar platos** y la **cocina más cercana**. También atiende **lo que publicamos para otro grupo**. Puerto **8083**. Lo programamos en la Entrega 2.

## Qué hace

- Busca platos de la cocina elegida por texto, con filtros (restaurante, categoría, etiqueta, precio máximo, disponibilidad), orden (relevancia o precio) y páginas.
- Sugiere la **cocina más cercana** a la zona que escribe el cliente.
- Atiende la capacidad **"Cocina más cercana y platos disponibles"** que publicamos ([contrato](../../docs/contracts/README.md)).
- Mantiene sus datos al día con los mensajes que recibe, y los puede **reconstruir de cero** si hace falta.

Lo que guarda es una **copia** armada para buscar rápido (Clase 5). Sirve para mirar, pero **no decide el stock**: que un plato figure "disponible" es orientativo, y cuando el cliente lo agrega al carrito, Inventario vuelve a controlar.

## Qué datos guarda (Solr)

| Índice | Qué guarda |
| --- | --- |
| `platos` | Nombre, descripción, categoría, etiquetas, precio, foto, restaurante, cocina, si está activo, si está disponible, si tiene receta y cuándo se actualizó |
| `cocinas` | Nombre, localidad, dirección, ubicación (para calcular distancias), tiempos estimados de entrega, costo de envío y sus restaurantes |
| `zonas` | Barrios y ciudades con su ubicación, cargados al iniciar como datos de referencia propios de Búsqueda |

Un cambio en un plato o en su stock se tiene que ver acá en **5 segundos como máximo**. Un plato sin receta no aparece como disponible.

## Cómo se organiza por dentro: capas, con dos entradas

Usamos capas, pero el servicio recibe trabajo por **dos lados**: por HTTP (las búsquedas) y por RabbitMQ (los mensajes que lo mantienen al día).

```
cmd/api/               arranca el servidor HTTP y el lector de mensajes
internal/handlers/     entrada 1: rutas HTTP
internal/indexer/      entrada 2: lee los mensajes y actualiza Solr
internal/services/     arma las búsquedas, filtros, orden y distancia
internal/repositories/ acceso a Solr
internal/domain/       cómo es un plato, una cocina y una zona en la búsqueda
internal/clients/      para pedirle datos a Catálogo e Inventario al reconstruir
solr/                  configuración de cada índice
```

## Rutas

Salvo las rutas técnicas comunes, la tabla muestra rutas internas del servicio. El frontend entra por `/api/v1` a través del gateway; las rutas entre servicios no se publican al exterior.

| Ruta | Quién la usa | Para qué |
| --- | --- | --- |
| `GET /platos/buscar` | el frontend | Buscar platos de una cocina |
| `GET /zonas?q=` | el frontend | Sugerir la cocina más cercana a una zona |
| `GET /v1/...` | el gateway, para el otro grupo (con API key) | Lo que publicamos: ver el [contrato](../../docs/contracts/v1/openapi.yaml). El gateway expone `/public/v1/...` y elimina `/public` al reenviar |
| `POST /internal/reindexar` | operación técnica autorizada en la red interna | Reconstruir la búsqueda desde cero |

## Mensajes que escucha

| Mensaje | Qué hace |
| --- | --- |
| `cocina.*` | Actualiza la cocina y sus restaurantes |
| `plato.*` | Agrega, cambia o saca un plato |
| `disponibilidad.cambiada` | Marca si el plato se puede pedir y si tiene receta |

Si un mensaje llega dos veces, no pasa nada: actualiza el mismo plato. Y si llega un mensaje viejo, lo ignora porque compara el número de versión.

La versión se compara por fuente: Catálogo actualiza datos comerciales e Inventario actualiza disponibilidad. Un evento de stock no sobrescribe el precio y uno de Catálogo no sobrescribe el stock. El ACK se realiza después de persistir la actualización idempotente en Solr. El objetivo de 5 segundos se mide en operación normal; las caídas generan atraso observable y requieren recuperación.

## Con quién habla

- Solo para **reconstruir de cero**, les pide los datos a Catálogo e Inventario **por su API**. Nunca entra a sus bases.

## Dependencias y fallas previstas

| Dependencia | Para qué | Si falla |
| --- | --- | --- |
| Solr | Índices y consultas | Responde `503`, nunca una lista vacía que simule falta de resultados; se puede navegar desde Catálogo |
| RabbitMQ | Recibir cambios | Aumenta el atraso del índice; se observa y se recupera al volver la mensajería |
| Catálogo e Inventario | Reconstrucción por API paginada | Se retoma el proceso al recuperarse; mientras tanto se conserva el índice anterior |

Las llamadas de reconstrucción admiten hasta 5 s por intento y tres reintentos: son trabajo de fondo, separado del plazo de 3 s de una solicitud de usuario ([ADR-005](../../docs/adr/ADR-005-comunicacion-entre-servicios.md)). Los índices `platos` y `cocinas` se reconstruyen desde sus fuentes; `zonas` se restaura desde su carga inicial propia. El término público `sede` no cambia el nombre interno del índice `cocinas`.

## Configuración prevista

Al implementar se definirán el puerto, las direcciones de Solr, RabbitMQ y las API internas de Catálogo e Inventario. El radio de cobertura de la API publicada es 25 km, según el [contrato v1](../../docs/contracts/v1/openapi.yaml). Las claves de consumidores se validan en el gateway mediante `PUBLIC_API_KEYS`; no se incorporan al índice.

## Pruebas previstas

- Mensajes repetidos y fuera de orden no sobrescriben datos más nuevos; las versiones de Catálogo e Inventario se comparan por separado.
- Filtros, orden, paginación e indexación contra Solr real; cocina más cercana y error `SIN_COBERTURA`.
- Reconstrucción concurrente con eventos sin perder actualizaciones ni dejar sin servicio el índice anterior.
- Respuestas de la capacidad publicada conformes a OpenAPI, incluida la traducción de fallas a `503`.
