# Pedidos

**Responsable:** Lucía Rucci.

> Entrega 1: diseño preliminar; todavía no hay implementación. Las carpetas, rutas, configuración y pruebas indicadas son previstas para el desarrollo desde la Entrega 2.

Maneja **el pedido de principio a fin**. Puerto **8085**. Es uno de los dos servicios donde no se puede equivocar nada. Lo programamos en la Entrega 2.

## Qué hace

- **Carrito:** guarda los platos y cantidades que eligió el cliente, todos de **una sola cocina**. Guarda una copia del nombre, el precio y el restaurante de cada plato, y el número de la reserva que hizo Inventario.
- **Confirmación:** cuando el cliente confirma, coordina los pasos con Inventario (y con el grupo proveedor, en el paso que corresponda). Solo crea el pedido si todo salió bien.
- **Estados:** controla que el pedido pase por los estados en el orden correcto y guarda el historial de cambios.
- **Cancelar y rechazar:** el pedido pasa a CANCELADO y le pide a Inventario que devuelva el stock.
- Es el **único servicio que habla con el grupo proveedor**.

## Estados del pedido

```
RECIBIDO → ACEPTADO → EN_PREPARACION → LISTO → EN_CAMINO → ENTREGADO
    └──→ CANCELADO   (solo desde RECIBIDO: cancela el cliente o rechaza la cocina)
```

La cocina lo avanza hasta EN_CAMINO y el cliente confirma que lo recibió (ENTREGADO). No existe un estado "rechazado". El pedido es **uno solo**: los platos se muestran agrupados por restaurante, pero los restaurantes no tienen estados propios.

## Qué datos guarda (MySQL)

| Tabla | Qué guarda |
| --- | --- |
| `carrito` / `carrito_item` | El carrito: cocina, reserva, cuándo vence y sus platos |
| `pedido` | Cliente, copia de su mail, cocina, estado, total y un número de **versión** |
| `pedido_item` | Cada plato con su nombre, precio, restaurante y cantidad |
| `pedido_historial` | Cada cambio de estado, cuándo y quién lo hizo |
| `saga_paso` | En qué paso va cada confirmación o devolución |
| `idempotencia` · `outbox` · `mensaje_procesado` | Para no repetir operaciones, no perder mensajes y no procesar dos veces el mismo mensaje |

## Cómo se organiza por dentro: hexagonal + DDD

Igual que Inventario, las reglas quedan **en el centro**, separadas de la base, de HTTP y de los otros servicios. Además usamos una idea de DDD: el **pedido controla sus propias reglas**. No se le puede cambiar el estado desde cualquier lado; hay que pedírselo al pedido, y él decide si el cambio es válido (por ejemplo, no se puede cancelar si ya está ACEPTADO) (Clase 8).

El número de **versión** sirve para cuando dos cosas pasan a la vez. Si el cliente cancela justo cuando la cocina acepta, se guarda solo el primer cambio y el otro se rechaza.

```
cmd/api/                      arranca el servicio
internal/domain/              el pedido y sus reglas, el carrito, el monto
internal/application/         las acciones: AgregarAlCarrito, Confirmar, Cancelar, Rechazar, Avanzar, ConfirmarEntrega
internal/ports/               lo que el centro ofrece y lo que necesita (Catálogo, Inventario, proveedor, base, mensajes)
internal/adapters/http/       rutas HTTP
internal/adapters/mysql/      acceso a MySQL
internal/adapters/rabbitmq/   envío y lectura de mensajes
internal/adapters/catalogo/   para pedirle el plato a Catálogo
internal/adapters/inventario/ para reservar, descontar y devolver en Inventario
internal/adapters/proveedor/  para hablar con el grupo proveedor
migrations/                   tablas
```

Si el grupo proveedor cambia su API, solo cambiamos su adaptador: las reglas del pedido no se tocan.

## Rutas

Salvo las rutas técnicas comunes, la tabla muestra rutas internas del servicio. El frontend entra por `/api/v1` a través del gateway; las rutas entre servicios no se publican al exterior.

| Ruta | Quién la usa | Para qué |
| --- | --- | --- |
| `GET`/`DELETE /carrito` · `POST /carrito/items` · `PATCH`/`DELETE /carrito/items/{platoId}` | cliente | Usar el carrito |
| `POST /pedidos` | cliente | Confirmar (con clave para no crear dos pedidos) |
| `GET /pedidos/mios` | cliente | Ver sus pedidos y su historial |
| `POST /pedidos/{id}/cancelar` · `/entregado` | cliente | Cancelar o confirmar que lo recibió |
| `GET /cocina/pedidos` | cocina | Ver los pedidos de su cocina |
| `POST /cocina/pedidos/{id}/aceptar` · `/rechazar` · `/avanzar` | cocina | Cambiar el estado |
| `GET /admin/cocinas/{id}/metricas` | admin | Ver métricas de su cocina |

**Permisos:** un cliente solo ve sus pedidos y solo compra en una cocina activa; una cocina solo ve y cambia sus propios pedidos.

## Con quién habla

- **Llama a** Catálogo (para ver el plato y la cocina), a Inventario (para reservar, descontar, devolver y preguntar si algo se hizo) y al grupo proveedor. Siempre con un tiempo máximo de espera, reintentos con la misma clave y circuit breaker ([ADR-005](../../docs/adr/ADR-005-comunicacion-entre-servicios.md)).
- **Manda** `pedido.estado_cambiado`, con el mail del cliente adentro.
- **Escucha** `reserva.vencida`.

## Dependencias y fallas previstas

| Dependencia | Para qué | Si falla |
| --- | --- | --- |
| MySQL (`pedidos_db`) | Carritos, pedidos, historial y pasos de recuperación | No se puede operar el carrito ni consultar o cambiar pedidos con garantías |
| Catálogo | Validar plato, cocina y precio | No se agrega el plato; máximo 500 ms por intento y un reintento |
| Inventario | Reservar, descontar y devolver | Máximo 800 ms por intento y un reintento con la misma clave; si el resultado es incierto, se consulta y se recupera |
| Grupo proveedor | Paso de confirmación pendiente de asignación | Usa el presupuesto restante; se recupera o compensa según su contrato, sin inventar el resultado |
| RabbitMQ | Publicar estados y recibir vencimientos | El outbox conserva los eventos; los avisos y la actualización de carritos vencidos pueden atrasarse |

Autenticación, llamadas, esperas entre reintentos y trabajo local comparten **3 s totales**. Si una confirmación no tiene resultado seguro dentro de ese plazo, se muestra "Estamos confirmando tu pedido" y continúa desde el paso persistido. No se responde que fracasó mientras se desconoce si el stock se descontó ([ADR-005](../../docs/adr/ADR-005-comunicacion-entre-servicios.md)).

## Procesos en segundo plano

- Publicación del outbox con confirmación del broker.
- Recuperación de confirmaciones incompletas desde su paso persistido.
- Recuperación de devoluciones pendientes: cancelar el pedido no se frena porque Inventario esté indisponible.

El cambio de estado, su historial y el evento se guardan en una sola transacción. El admin solo consulta métricas de cocinas de su localidad; Pedidos guarda la localidad de la cocina como referencia al crear el pedido.

## Configuración prevista

`PEDIDOS_DB_PASSWORD`, `PROVIDER_BASE_URL` y `PROVIDER_API_KEY`, junto con las credenciales comunes de MySQL/RabbitMQ, están anticipadas en [`.env.example`](../../.env.example). Las conexiones, el puerto y las direcciones internas de Catálogo e Inventario se definirán al implementar. El proveedor todavía no fue asignado.

## Pruebas previstas

- Transiciones inválidas, cancelación después de aceptar y acceso a pedidos de otro cliente o cocina.
- Dos confirmaciones con la misma clave crean un pedido; cancelar y aceptar a la vez permite solo una transición.
- Caída en cada paso de la confirmación y recuperación al reiniciar; devolución pendiente sin duplicar stock.
- Contrato del proveedor cuando se conozca; creación conjunta del cambio de estado, historial y outbox.
