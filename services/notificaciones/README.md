# Notificaciones

**Responsable:** Valentina Rey.

Manda un **mail al cliente cada vez que cambia el estado de su pedido**. No atiende solicitudes del sistema: solo escucha mensajes de RabbitMQ. Tiene un puerto técnico, **8086**, solo dentro de Docker, para `/health`, `/ready` y `/metrics`.

## Qué hace

Es el único servicio que funciona **solo con mensajes**: no recibe solicitudes del frontend, solo escucha `pedido.estado_cambiado`. Si se cae el correo o este mismo servicio, los pedidos siguen funcionando y los mensajes esperan en la cola hasta que vuelva.

- **No le pregunta nada a Usuarios:** el mail del cliente ya viene adentro del mensaje (Pedidos lo guardó al crear el pedido).
- **No manda dos veces el mismo aviso.** Solo puede llegar un mail repetido si el servicio se cae justo después de mandarlo y antes de marcarlo como ENVIADO (ver más abajo).

## Qué datos guarda (MongoDB)

Una colección `notificaciones` con un documento por mail: el ID del mensaje como `_id` (no se puede repetir), a quién se manda, de qué pedido, qué estado avisa, si ya se mandó (PENDIENTE o ENVIADO) y cuántas veces se intentó.

## Cómo se organiza por dentro

Es un servicio chico, así que no lo separamos en capas ([ADR-002](../../docs/adr/ADR-002-arquitectura-interna.md)):

```
cmd/worker/                  arranca el lector de RabbitMQ, reintenta, manda a la DLQ y atiende /health, /ready y /metrics
cmd/prueba/                  lee un solo mensaje, para probar la deduplicación a mano
internal/notificaciones/
  evento.go                  lee y valida el mensaje (el sobre común de los eventos)
  mongo.go                   registra el aviso, lo "reclama" para enviarlo y lo marca como enviado
  mailpit.go                 manda el mail por SMTP
internal/metricas/           cuenta los mensajes según cómo terminaron y mide el envío
```

## Cómo procesa cada mensaje

1. Anota el aviso como **PENDIENTE** (el ID no se puede repetir).
2. Lo **reclama** por 30 s, para que no lo mande otro intento al mismo tiempo.
3. Manda el mail.
4. Lo marca como **ENVIADO**.
5. **Recién ahí** le confirma a RabbitMQ que lo procesó (ACK).

Si el mismo mensaje vuelve a llegar: si ya está ENVIADO, lo ignora; si está PENDIENTE, lo intenta de nuevo.

Puede pasar que llegue un mail repetido si el servicio se cae justo después de mandarlo y antes de marcarlo. Es la excepción que aceptamos en el [SPEC](../../SPEC.md): preferimos un mail de más antes que perder un aviso (ver [ADR-005](../../docs/adr/ADR-005-comunicacion-entre-servicios.md)).

## Cómo se lo observa

- **Logs JSON**, una línea por mensaje, con `evento_id`, `pedido_id`, `correlation_id` (el mismo del pedido que causó el aviso) y `resultado`.
- **Métricas** en `/metrics` (Prometheus las lee): `notificaciones_mensajes_total` por resultado (`enviado`, `duplicado`, `reintento`, `dlq`, `invalido`), `notificaciones_envio_duration_seconds` y `notificaciones_rabbitmq_conectado`. Se ven en el tablero de Grafana, en "Notificaciones: mails".
- **`/ready`** responde 200 solo si llega a MongoDB y está escuchando la cola. Así se distingue un worker caído (Prometheus no lo lee y salta `ServicioCaido`) de uno sin trabajo (está listo, pero no cuenta mensajes).

## Si algo falla

| Dependencia | Para qué | Si falla |
| --- | --- | --- |
| RabbitMQ | Recibir los cambios de estado | Los mensajes esperan en la cola; los que Pedidos todavía no publicó quedan en su outbox. Si se corta la conexión, el lector vuelve a conectarse a los 5 s |
| MongoDB (`notificaciones_db`) | Estado del envío y deduplicación | No se hace ACK sin guardar el resultado: el mensaje vuelve a la cola |
| Servidor de correo | Mandar los mails | Reintenta a los 5 s, 30 s y 2 min. Si no puede, el mensaje va a `notificaciones.mailer.dlq` |

Un mensaje roto va directo a la DLQ. Que falle el mail **no frena el pedido**: la cocina lo sigue preparando.

## Cómo usarlo

Con `NOTIFICACIONES_DB_PASSWORD` en el `.env`:

```bash
docker compose up -d --build notificaciones
```

El trabajo `notificaciones-mongo` crea el usuario de la base. Los mails se ven en **Mailpit**: http://localhost:8025.

| Variable | Para qué |
| --- | --- |
| `NOTIFICACIONES_DB_PASSWORD` | Contraseña del usuario `notificaciones` en MongoDB |
| `RABBITMQ_USER`, `RABBITMQ_PASSWORD` | Conexión con RabbitMQ |
| `SMTP_ADDR` | Servidor de correo (en Docker Compose, Mailpit) |
| `MONGO_URI`, `RABBITMQ_URL` | Opcionales: reemplazan la conexión armada con las variables de arriba (sirven para las pruebas) |

## Cómo lo probamos

- **Tests de Go** de la lectura y validación del mensaje (`internal/notificaciones/evento_test.go`).
- **Integración con MongoDB real** (`mongo_integracion_test.go`, con `MONGO_TEST_URI`): el mismo evento dos veces es un solo documento, mientras un intento lo tiene reclamado nadie más lo manda, y una vez ENVIADO no se reenvía. Sin la variable, se saltea.
- **Deduplicación con RabbitMQ, MongoDB y Mailpit reales.** Desde la raíz del repositorio, en PowerShell:

  ```powershell
  & ./services/notificaciones/prueba-mensaje-repetido.ps1
  ```

  Requiere Go, Docker y las imágenes `mongo:7` y `rabbitmq:3-management`. Crea contenedores descartables, publica dos veces el mismo evento y verifica que haya **un solo mail** en Mailpit y un solo registro ENVIADO con un intento en MongoDB. También comprueba que un evento inválido llegue a la DLQ. Al terminar, borra los contenedores.
- **A mano, un mensaje por vez:** con `MONGO_URI` y `RABBITMQ_URL` configuradas, enlazar una cola de prueba a `ghostkitchen.events` con la clave `pedido.estado_cambiado`, publicar `evento-prueba.json` y correr `go run ./cmd/prueba` dos veces. El lector vuelve a poner el mensaje en la cola para simular que llega de nuevo; tiene que quedar un solo documento.
- **En la jornada de fallas:** con un corte corto del correo, el mail llegó 24 s después de que volvió; con un corte largo, terminó en la DLQ ([POSTMORTEM](../../docs/POSTMORTEM.md)).
