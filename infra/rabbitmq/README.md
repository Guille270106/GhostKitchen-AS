# RabbitMQ

Mensajería entre servicios (ADR-005). Se levanta con el resto del sistema:

```bash
docker compose up -d --build
```

- **`rabbitmq`**: el broker. Crea el usuario de `RABBITMQ_USER` y `RABBITMQ_PASSWORD` del `.env`.
- **`rabbitmq-topologia`**: un trabajo de una sola vez que, cuando RabbitMQ está listo, carga [`definiciones.json`](definiciones.json) por la API de administración y termina. Se puede repetir sin problema: importar las mismas definiciones no duplica nada.

Las definiciones no se cargan al arrancar el broker porque, en ese caso, RabbitMQ no crea el usuario del `.env`.

## Panel de administración

http://localhost:15672, con el usuario y la contraseña de `RABBITMQ_USER` y `RABBITMQ_PASSWORD`. Ahí se ven las colas, cuántos mensajes tienen, los que fueron a la DLQ y un gráfico de mensajes por segundo.

Desde los servicios, la dirección es `amqp://<RABBITMQ_USER>:<RABBITMQ_PASSWORD>@rabbitmq:5672/`. El puerto 5672 no se publica fuera de Docker.

## Topología

Un exchange `topic`, `ghostkitchen.events`, donde se publica cada evento con su `tipo` como clave de ruteo. Cada servicio que escucha tiene **su propia cola**:

| Cola | Escucha | Quién la consume | DLQ |
| --- | --- | --- | --- |
| `busqueda.indexador` | `cocina.*`, `plato.*`, `disponibilidad.cambiada` | Búsqueda | `busqueda.indexador.dlq` |
| `inventario.referencias` | `cocina.*`, `plato.*` | Inventario | `inventario.referencias.dlq` |
| `pedidos.reservas` | `reserva.vencida` | Pedidos | `pedidos.reservas.dlq` |
| `notificaciones.mailer` | `pedido.estado_cambiado` | Notificaciones | `notificaciones.mailer.dlq` |

- Las colas son **quorum** y **durables**: sobreviven a un reinicio de RabbitMQ.
- Un mensaje **rechazado sin reencolar**, o **entregado 5 veces sin confirmar**, va al exchange `ghostkitchen.dlx` y de ahí a la DLQ de su cola.
- Los datos quedan en el volumen `rabbitmq-data`. Con `docker compose down -v` se borran, y al volver a levantar se carga la topología de nuevo.

El formato de los mensajes está en [`docs/contracts/eventos`](../../docs/contracts/eventos/README.md).

## Agregar una cola

1. Sumar la cola, su DLQ y sus bindings a `definiciones.json`, copiando una existente.
2. Volver a correr el trabajo:

   ```bash
   docker compose up rabbitmq-topologia
   ```
