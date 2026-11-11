# Gateway

**Responsable:** Guillermina Sánchez.

El API Gateway es **NGINX** y escucha en el puerto **8080**. Es la única puerta de entrada: todo lo que viene del frontend (`/api/v1/...`) y del grupo que nos consume (`/public/v1/...`) pasa por acá. Más detalle en [ARCHITECTURE.md](../docs/ARCHITECTURE.md#8-rutas-del-gateway) y en el [ADR-001](../docs/adr/ADR-001-limites-de-los-servicios.md).

## Qué hace

- **Manda cada solicitud al servicio que corresponde** y le saca `/api/v1` (o `/public`) a la ruta.
- **Verifica el login.** Antes de dejar pasar algo privado, le pregunta a Usuarios si el token sirve. Lo que **cada usuario puede hacer** lo controla cada servicio.
- **Controla la API key** del grupo que nos consume y la limita a 60 solicitudes por minuto.
- **Limita las solicitudes por IP** en el login (contra quien prueba contraseñas) y en la búsqueda del frontend.
- **Reparte la carga** entre las 2 copias de Catálogo, por turnos. Si una copia falla, deja de mandarle tráfico un rato y después la vuelve a probar.
- **Registra cada solicitud** en un log JSON, con el `X-Correlation-Id`, la copia que atendió y el `trace_id`.

El gateway **no tiene reglas del negocio**: solo manda, verifica, limita y reparte.

## Rutas

| Ruta | Destino | Login |
| --- | --- | --- |
| `/public/v1/...` | Búsqueda `/v1` | No: `X-API-Key`, 60 por minuto por clave |
| `GET /api/v1/cocinas...` y `GET /api/v1/platos/{id}` | Catálogo | No |
| `GET /api/v1/busqueda/sedes[/{id}[/platos]]` y `/api/v1/busqueda/platos/{id}` | Búsqueda `/v1` | No. 10 por segundo por IP (ráfaga de 40) |
| `POST /api/v1/auth/registro` y `POST /api/v1/auth/login` | Usuarios | No. 10 por minuto por IP (ráfaga de 5); después, `429` con `Retry-After: 6` |
| `POST /api/v1/admin/cuentas-cocina` | Usuarios | Sí (Usuarios controla que sea admin) |
| `/api/v1/admin/cocinas[/{id}[/restaurantes[/{rid}]]]` | Catálogo | Sí (Catálogo controla que sea admin y que la cocina sea de su localidad) |
| `/api/v1/cocina/platos[/{id}]` y `/api/v1/cocina/restaurantes/{rid}/platos` | Catálogo | Sí (Catálogo controla que sea la cocina del plato) |
| `/api/v1/cocina/ingredientes...`, `/api/v1/cocina/recetas...` y `/api/v1/cocina/platos/{id}/receta` | Inventario | Sí (Inventario controla que sea una cuenta Cocina y su cocina) |
| `/api/v1/carrito...`, `/api/v1/pedidos...` y `/api/v1/confirmaciones/{id}` | Pedidos | Sí (Pedidos controla que sean los datos del cliente) |
| `/api/v1/cocina/pedidos...` | Pedidos | Sí (Pedidos controla que sea la cocina del pedido) |
| `GET /api/v1/admin/metricas[?cocinaId=]` | Pedidos | Sí (Pedidos controla que sea admin y que la cocina sea de su localidad) |
| `/internal/...` y cualquier otra ruta | — | Responde `404` en JSON (`NO_ENCONTRADO`), igual que Caddy en la nube |

Notificaciones no tiene rutas: solo escucha mensajes. El frontend llama al gateway a través del servidor de Vite (`GATEWAY_URL`), así que no hace falta CORS.

## Login en el borde

1. `auth_request` le pregunta a Usuarios (`/auth/verify`) si el token sirve. Solo viajan los headers, con 300 ms de tope y sin reintento.
2. Si sirve, el gateway copia hacia el servicio la identidad que devuelve Usuarios: `X-Usuario-Id`, `X-Usuario-Email`, `X-Rol`, `X-Cocina-Id` y `X-Localidad`. Siempre los pisa: si el cliente manda `X-Rol: admin` inventado, se reemplaza por el verdadero o se borra.
3. Si no sirve, `401 NO_AUTENTICADO` con `WWW-Authenticate: Bearer`. Si Usuarios está caído o tarda más de 300 ms, `503` (**falla cerrada**): nunca se deja pasar sin verificar. Las rutas públicas siguen andando.

Esas líneas están en [`identidad.conf`](identidad.conf), que se incluye en cada ruta privada, y las comunes de proxy en [`proxy-comun.conf`](proxy-comun.conf). Para proteger una ruta nueva alcanza con `include /etc/nginx/identidad.conf;` adentro de su `location`.

## Capacidad publicada

- `/public/v1/` va a Búsqueda `/v1/`. La `X-API-Key` se verifica con `auth_request` a Búsqueda (`/internal/apikey/verificar`).
- **Límite:** NGINX usa un balde que se vacía a 1 solicitud por segundo (`rate=60r/m`) con `burst=59`: una clave puede hacer 60 solicitudes seguidas y después 1 por segundo. Por eso `Retry-After` es 1. Solo envía `RateLimit-Limit: 60` (y `RateLimit-Remaining: 0` en el `429`), porque NGINX no calcula lo que queda en las respuestas exitosas.
- Sin clave, cuenta por IP con el mismo límite, y responde `401`. No confía en un `X-Forwarded-For` mandado por el cliente.
- `POST /public/v1/proximidad` conserva el cuerpo JSON, con un máximo de 64 KiB (`413` si se pasa). No se registran cuerpos con coordenadas.
- Los errores salen con formato Problem Details (`401`, `404`, `413`, `429`, `503`).

## Dos copias de Catálogo

`upstream catalogo` reparte por turnos entre `catalogo-1` y `catalogo-2`. Si una copia falla 3 veces en 10 s, NGINX la saca 10 s y después la vuelve a probar. Una lectura que falla se reintenta una vez en la otra copia, dentro de los 3 s (`proxy-comun.conf`); las escrituras no se reintentan. Detalle en el [ADR-012](../docs/adr/ADR-012-balanceo-de-carga.md).

El gateway usa el DNS interno de Docker (`resolver 127.0.0.11`) y `resolve` en cada `upstream`, así vuelve a buscar la dirección de cada servicio cada 10 s. Si un servicio se recrea con otra IP (por ejemplo con `docker compose up --build`), el gateway la sigue sin reiniciarse.

## Headers de seguridad

Todas las respuestas llevan `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY` y `Referrer-Policy: no-referrer` ([`seguridad.conf`](seguridad.conf)). El archivo se incluye en cada location que responde, porque en NGINX una location con su propio `add_header` no hereda los del nivel `server`.

## Logs y trazas

- Una línea JSON por solicitud: ruta sin parámetros, estado, duración, `X-Correlation-Id` (recibido o generado), la copia que atendió y `trace_id`. Las contraseñas, los tokens y las API keys no aparecen en los logs.
- La imagen es `nginx:1.27-alpine-otel`, que trae el módulo oficial de OpenTelemetry. El gateway crea el primer span de la traza, lo manda a Jaeger y pasa el `traceparent` a los servicios. Como el módulo guarda la URL completa en el span, solo traza las solicitudes **sin parámetros**; las demás se trazan desde el servicio.

## Si algo falla

| Dependencia | Qué pasa |
| --- | --- |
| Usuarios | `503` a los 300 ms en las rutas privadas; las públicas siguen andando |
| Un servicio de destino | Falla su ruta con `503` y Problem Details; el gateway nunca devuelve una lista vacía ni simula un éxito |
| Una copia de Catálogo | NGINX la saca un rato y sigue con la otra |

El gateway tiene healthcheck en Docker Compose (`/health`, cada 5 s), así `docker compose ps` muestra si está sano.

## Cómo lo probamos

- Con NGINX y Usuarios reales: sin token y con token basura da `401`; con `X-Rol: admin` inventado llega `X-Rol: cliente`; la cuenta Cocina llega con su `X-Cocina-Id`; con Usuarios apagado o tardando 2 s, `503` a los 300 ms; el login da `429` al pasar el límite. Ningún token aparece en los logs.
- La capacidad publicada, con `scripts/probar-api-publica.ps1` (`401`, `413`, `429` y `503`) y con el test de contrato en cada pull request.
- Las dos copias de Catálogo, con la prueba de carga: cada copia atendió la mitad y, al apagar una, no hubo errores ([ADR-013](../docs/adr/ADR-013-capacidad-y-costos.md)).
- Recreando Búsqueda con una IP nueva: el gateway la siguió sin reiniciarse.
