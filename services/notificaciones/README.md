# Notificaciones

**Responsable:** Valentina Rey.

Manda un **mail al cliente cada vez que cambia el estado de su pedido**. Puerto **8086**, pero solo para `/health`, `/ready` y `/metrics`. Se implementa en la ronda 4.

## Qué hace

Es el único servicio que funciona **solo con mensajes**: no recibe pedidos del frontend, solo escucha `pedido.estado_cambiado`. Muestra la ventaja de usar mensajes (Clase 4): si se cae el correo o este mismo servicio, los pedidos siguen funcionando y los mensajes esperan en la cola hasta que vuelva.

- **No le pregunta nada a Usuarios:** el mail del cliente ya viene adentro del mensaje (Pedidos lo guardó al crear el pedido).
- Deduplica eventos ya enviados. Según RN17/HU13, solo se admite un correo repetido si el servicio se cae después de enviarlo y antes de persistir el estado ENVIADO; los reintentos habituales no deben duplicarlo.

## Qué datos guarda (MongoDB)

Una colección `notificaciones` con un registro por mail: el ID del mensaje (no se puede repetir), a quién se manda, de qué pedido, qué estado avisa, si ya se mandó (PENDIENTE o ENVIADO) y cuántas veces se intentó.

## Cómo se organiza por dentro: capas

Usamos capas, pero en vez de entrar por HTTP, **entra por RabbitMQ**. El envío del mail pasa por una **interfaz `Mailer`** que tiene tres versiones: Mailpit para probar en local, un SMTP real para la versión desplegada y una falsa para los tests.

```
cmd/worker/            arranca el lector de mensajes y las rutas de health y metrics
internal/consumer/     lee los mensajes de RabbitMQ
internal/services/     reglas: no repetir, elegir el texto del mail
internal/repositories/ acceso a MongoDB
internal/mailer/       la interfaz Mailer y sus versiones
internal/templates/    el texto de cada mail según el estado
```

## Cómo procesa cada mensaje

1. Anota el mensaje como **PENDIENTE** (el ID no se puede repetir).
2. Manda el mail.
3. Lo marca como **ENVIADO**.
4. **Recién ahí** le confirma a RabbitMQ que lo procesó.

Si el mismo mensaje vuelve a llegar:
- si ya está ENVIADO, lo ignora;
- si está PENDIENTE, lo intenta de nuevo.

Puede pasar que llegue un mail repetido si el servicio se cae justo después de mandarlo y antes de marcarlo. Es la excepción prevista en RN17/HU13: se prioriza recuperar el envío antes que perder el aviso (ver [ADR-005](../../docs/adr/ADR-005-comunicacion-entre-servicios.md)).

## Si algo falla

- Si el correo falla, reintenta esperando cada vez más. Si no puede, el mensaje va a la cola de errores `notificaciones.dlq` para revisarlo.
- Si el mensaje viene roto, va directo a esa cola.
- Que falle el mail **no frena el pedido**: la cocina lo sigue preparando.
- En local usamos **Mailpit**, que tiene una página web para ver los mails sin mandarlos de verdad.

## Dependencias y fallas previstas

| Dependencia | Para qué | Si falla |
| --- | --- | --- |
| RabbitMQ | Recibir cambios de estado | Los eventos publicados quedan en la cola durable; los aún no publicados permanecen en el outbox de Pedidos |
| MongoDB (`notificaciones_db`) | Estado de envío y deduplicación | No se hace ACK sin persistir el resultado; el evento queda disponible para recuperación |
| Servidor SMTP | Enviar correos | Se reintenta según ADR-005; si se agotan los intentos va a `notificaciones.dlq` |

## Configuración prevista

`NOTIFICACIONES_DB_PASSWORD`, `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER` y `SMTP_PASSWORD`, junto con las credenciales comunes de MongoDB/RabbitMQ, están en las [variables del `.env`](../../README.md#variables-del-env). El remitente, las conexiones, el puerto técnico y los límites de reintento se definirán al implementar.

## Pruebas previstas

- Reentrega de un evento ya marcado ENVIADO no manda otro correo; errores transitorios reintentan y mensajes inválidos van a la DLQ.
- Evento real desde RabbitMQ produce un correo visible en Mailpit.
- Caída después del envío y antes de persistir: comprobar la recuperación y la excepción de posible duplicado prevista en RN17/HU13, sin perder el aviso.

## V13 · Validar la recepción

El paquete interno valida el sobre de L13 y registra cada evento en MongoDB con el UUID como clave única. Si el mismo evento vuelve, conserva el registro existente y sus intentos.

Para probarlo con instancias descartables, configurar MONGO_URI y RABBITMQ_URL, enlazar la cola notificaciones.v13.prueba a ghostkitchen.events con la clave pedido.estado_cambiado y publicar evento-prueba.json. Ejecutar go run ./cmd/prueba dos veces. El lector vuelve a poner el mensaje en cola para simular la reentrega. Debe quedar un solo documento PENDIENTE en notificaciones_v13_prueba.notificaciones.

El worker continuo, el correo y el ACK después de guardar ENVIADO corresponden a V14.
