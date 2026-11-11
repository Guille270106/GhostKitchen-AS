# ADR-002 — Cómo se organiza cada servicio por dentro

- **Estado:** Aceptada e implementada en los seis servicios (ver [Cómo lo validamos](#cómo-lo-validamos))
- **Fecha:** 2026-10-16
- **Responsable:** Valentina Rey
- **Decisión del enunciado:** D2 (estilos o patrones de cada servicio, dependencias internas y por qué cada alternativa se adecua al problema del servicio)
- **Relacionado con:** [ADR-001](ADR-001-limites-de-los-servicios.md) · [ADR-003](ADR-003-persistencia.md) · [ARCHITECTURE](../ARCHITECTURE.md) (sección 4)

## Contexto

Los seis servicios no tienen la misma cantidad de reglas. Algunos casi solo guardan y leen datos (Usuarios, Catálogo); otros tienen reglas que no se pueden romper y varias formas de entrar (Inventario, Pedidos). Por eso no usamos el mismo estilo en todos: más estructura donde hay reglas que cuidar y menos donde solo agregaría pasos.

Además, el enunciado pide tests unitarios de las reglas y tests de integración contra una dependencia real. Para poder hacer los dos, las reglas no pueden depender directamente de MongoDB, Solr o HTTP.

## Decisión

**Elegimos el estilo según cuánta lógica de negocio tiene cada servicio.** Todos comparten una misma regla: las capas de afuera dependen de las de adentro, y el acceso a datos se usa a través de una **interfaz**, así los tests la pueden reemplazar.

| Servicio | Estilo | Por qué |
| --- | --- | --- |
| Usuarios | Capas | Registro, login y roles: guardar y consultar, sin reglas complejas |
| Catálogo | Capas + caché como *decorator* del repositorio | Lecturas y escrituras simples; la caché envuelve al repositorio sin que el servicio lo sepa (ADR-007) |
| Búsqueda | Capas, con dos entradas (HTTP y reconstrucción/mensajes) | No decide nada del negocio: refleja datos de otros y responde consultas |
| Notificaciones | Un solo paquete, con el lector de RabbitMQ como entrada | Recibe un mensaje y manda un mail. Es tan chico que separarlo en capas solo agregaba archivos: el lector (`cmd/worker`), la deduplicación en MongoDB y el envío a Mailpit están en `internal/notificaciones` |
| Inventario | **Hexagonal liviano** | Reglas que no se pueden romper (todo o nada, la merma nunca baja de lo reservado) y tres entradas: HTTP, mensajes y el proceso que vence reservas |
| Pedidos | **Hexagonal + DDD** (agregado `Pedido`) | Máquina de estados, saga de confirmación con compensaciones y control por versión |

### Capas (Usuarios, Catálogo y Búsqueda)

```
handlers  →  services  →  repositories (interfaz)
   │             │               ├── solr / mongo   (implementación real)
   │             │               └── memoria        (solo para tests)
   └── middleware (correlación, logs, trazas) y router
```

- **handlers:** traducen HTTP (parámetros, validación y errores Problem Details) y no tienen reglas.
- **services:** tienen las reglas. Por ejemplo, en Búsqueda: cobertura de 25 km, cocina más cercana con desempate por id y comparación de proximidad; en Catálogo: solo restaurantes activos, platos visibles solo si el plato, su restaurante y su cocina están activos.
- **repositories:** una interfaz (`Lector` e `Indexador` en Búsqueda, `Repositorio` en Catálogo) y sus implementaciones. Las reglas no importan el driver de Solr ni el de MongoDB.
- **Entradas adicionales:** en Búsqueda, `indexer` (reconstrucción desde Catálogo y, más adelante, el lector de mensajes) usa el mismo `Indexador` que la carga inicial, igual que el lector de mensajes.
- **clients:** cada llamada a otro servicio tiene su propio paquete (`clients/catalogo`), con sus tiempos y reintentos.
- `cmd/api/main.go` es el único lugar que arma todo (crea el repositorio real y se lo pasa al servicio).

### Hexagonal (Inventario y Pedidos)

El núcleo (dominio y casos de uso) define **puertos** (interfaces) y no conoce MySQL, HTTP ni RabbitMQ. Afuera, los **adaptadores** los implementan. En Inventario, el adaptador de MySQL es el que hace la transacción explícita del "todo o nada". En Pedidos, el agregado `Pedido` es el único que puede cambiar su estado y valida cada transición.

## Alternativas que pensamos

| Alternativa | A favor | En contra | Qué decidimos |
| --- | --- | --- | --- |
| **Hexagonal en todos los servicios** | Un solo estilo para todo | En Usuarios, Catálogo y Búsqueda agrega puertos y adaptadores sin reglas que proteger | La descartamos |
| **Capas en todos, también Inventario y Pedidos** | Más simple | Las reglas del stock y de los estados quedarían mezcladas con SQL y HTTP, difíciles de probar solas | La descartamos |
| **DDD completo en Inventario** | Agregados también para el stock | El modelo es chico y la garantía de no vender de más la da la base (`UPDATE ... WHERE total - reservado >= ?`) | La descartamos: hexagonal liviano |
| **Handlers que hablan directo con la base** | Menos archivos | No se puede probar una regla sin levantar la base | La descartamos |

## Consecuencias

**Lo bueno**

- Las reglas se prueban sin levantar nada: Búsqueda y Catálogo tienen tests de servicios y handlers con un repositorio en memoria, y aparte tests del repositorio real (contra un Solr simulado y contra MongoDB real).
- Cambiar de motor o sumar una caché no toca las reglas: la caché de Catálogo es otra implementación de la misma interfaz.
- Cada servicio se lee igual: quien conoce uno encuentra las cosas en el otro.

**Lo que aceptamos**

- Hay dos estilos en el sistema. Lo explicamos servicio por servicio en sus README.
- El código de middleware (correlación, logs y trazas) se repite en cada servicio. Lo aceptamos porque los servicios no comparten librerías propias: cada uno se despliega solo.
- El repositorio en memoria hay que mantenerlo junto con la interfaz.

## Cómo lo validamos

- **Entrega 2 (hecho):** Búsqueda y Catálogo siguen la estructura de capas. Sus tests cubren reglas y handlers sin dependencias reales; la reconstrucción del índice se probó contra un Catálogo simulado.
- **Entrega final (hecho):** Usuarios sigue las mismas capas. Inventario y Pedidos tienen puertos (`internal/ports`) y adaptadores (`internal/adapters`: MySQL, HTTP y RabbitMQ). Hay tests del agregado `Pedido` (transiciones válidas e inválidas, permisos) sin levantar nada, y de la reserva todo o nada contra MySQL real ([ADR-004](ADR-004-consistencia-y-concurrencia.md)).

