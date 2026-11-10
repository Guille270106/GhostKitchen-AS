# Microservicios

Cada carpeta es un servicio aparte, con su propio código, su `Dockerfile` y sus tests, y se levanta por separado. Su `README.md` explica qué hace el servicio, qué datos guarda, cómo se organiza por dentro, qué rutas tiene y qué mensajes manda o escucha.

Funcionan los seis. Usuarios, Inventario y Pedidos usan MySQL; Catálogo y Notificaciones, MongoDB; Búsqueda, Solr.

| Servicio | Qué hace | Base | Cómo se organiza por dentro | Puerto |
| --- | --- | --- | --- | --- |
| [usuarios](usuarios/README.md) | Registro, login y roles; recuerda la cocina elegida | MySQL | Capas | 8081 |
| [catalogo](catalogo/README.md) | Cocinas, restaurantes y platos: qué se vende y a qué precio | MongoDB + Memcached | Capas + caché | 8082 (2 copias) |
| [busqueda](busqueda/README.md) | Buscar platos, sugerir la cocina más cercana, calcular proximidad externa y atender lo que publicamos | Solr | Capas (entra por HTTP y por mensajes) | 8083 |
| [inventario](inventario/README.md) | Ingredientes, stock, recetas y reservas de 10 minutos | MySQL | Hexagonal liviano | 8084 |
| [pedidos](pedidos/README.md) | Carrito, pedido, estados y la confirmación con Inventario y el proveedor | MySQL | Hexagonal + DDD | 8085 |
| [notificaciones](notificaciones/README.md) | Un mail por cada cambio de estado del pedido | MongoDB | Un solo paquete (entra por mensajes) | — (sin API HTTP) |

El gateway (NGINX, puerto 8080) está explicado en [`gateway/`](../gateway/README.md).

## Quién depende de quién

Las imágenes de contexto y contenedores están en `docs/diagrams/`. Las dependencias se describen a continuación.

- No hay llamadas en círculo: Pedidos llama a Catálogo e Inventario, Usuarios a Catálogo y Búsqueda a Catálogo e Inventario solo para reconstruir el índice; ninguno llama de vuelta.
- Los mensajes van por RabbitMQ. El detalle está en el [ADR-005](../docs/adr/ADR-005-comunicacion-entre-servicios.md).

## Reglas que siguen todos los servicios

- **Go + Gin** (Notificaciones no usa Gin porque no tiene API HTTP).
- Los que tienen API HTTP tienen tres rutas técnicas:
  - `/health`: el servicio está prendido (lo usa Docker para saber si reiniciarlo);
  - `/ready`: el servicio puede atender, por ejemplo porque llega a su base;
  - `/metrics`: números para Prometheus.
- Los logs se escriben en JSON y llevan un identificador para seguir una misma acción entre servicios. **Nunca** se escriben contraseñas ni tokens.
- Las claves se leen de variables de entorno (ver [Variables del `.env`](../README.md#variables-del-env)), nunca van en el código.
- Los mensajes que publican o escuchan siguen el formato común de [`docs/contracts/eventos`](../docs/contracts/eventos/README.md).

## Sin código compartido entre servicios

Cada servicio es un módulo de Go independiente y se compila solo con su carpeta, así que no hay una carpeta `pkg/` en común. Lo técnico que se repite (logs, correlation id, trazas, el sobre de los mensajes) lo tiene cada servicio en su `internal/`, con la misma forma. Es un poco de código repetido a cambio de que cada servicio se pueda compilar, probar y desplegar sin depender de los demás.
