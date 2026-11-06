# Gateway

**Responsable:** Guillermina Sánchez.

> **Entrega 2:** implementada la entrada de la **capacidad publicada** en [`nginx.conf`](nginx.conf): `/public/v1/` → Búsqueda `/v1/`, verificación de `X-API-Key` con `auth_request` a Búsqueda (`/internal/apikey/verificar`), 60 solicitudes por minuto por clave (429 con `Retry-After`), cuerpo máximo de 64 KiB (413), `X-Correlation-Id` recibido o generado y reenviado, logs JSON sin parámetros ni claves, `/internal/` bloqueado y errores con Problem Details (401, 413, 429, 503). Las rutas del frontend (`/api/v1`), el login con Usuarios y el balanceo de Catálogo se agregan cuando esos servicios existan.
>
> **Trazas:** la imagen es `nginx:1.27-alpine-otel`, que trae el módulo oficial de OpenTelemetry. El gateway crea el primer span de la traza, lo manda a Jaeger (OTLP por gRPC, puerto 4317) y pasa el `traceparent` a los servicios. Como el módulo guarda la URL completa en el span, solo traza las solicitudes **sin parámetros**; las demás se trazan desde Búsqueda. El log JSON incluye `trace_id`. Las rutas `/api/v1/cocinas` y `/api/v1/platos/{id}` llevan a Catálogo.
>
> **Cómo cuenta el límite:** NGINX usa un balde que se vacía a 1 solicitud por segundo (`rate=60r/m`) con `burst=59`: una clave puede hacer 60 solicitudes seguidas y después 1 por segundo. Por eso `Retry-After` es 1. NGINX no calcula `RateLimit-Remaining` en las respuestas exitosas; solo envía `RateLimit-Limit: 60` (y `RateLimit-Remaining: 0` en el 429).

El API Gateway es **NGINX** y escucha en el puerto 8080. Lo configuramos en la Entrega 2. Más detalle en [ARCHITECTURE.md](../docs/ARCHITECTURE.md#8-rutas-del-gateway) y en el [ADR-001](../docs/adr/ADR-001-limites-de-los-servicios.md).

## Qué va a hacer

- **Ser la única puerta de entrada.** Todo lo que viene del frontend (`/api/v1/...`) y del otro grupo (`/public/v1/...`) pasa por acá, y el gateway lo manda al servicio que corresponde.
- **Verificar el login.** Antes de dejar pasar algo privado, le pregunta a Usuarios si el token es válido (`auth_request`). Si el cliente manda datos de identidad inventados, el gateway los borra y pone los verdaderos. Lo que **cada usuario puede hacer** lo controla cada servicio.
- **Controlar la API key** del otro grupo y limitarlo a 60 pedidos por minuto.
- **Limitar la cantidad de pedidos por usuario**, para que nadie sature el sistema.
- **Repartir la carga** entre las 2 copias de Catálogo, por turnos. Si una copia falla, NGINX deja de mandarle pedidos un rato y después la vuelve a probar.
- **Una entrada interna** (que no se ve desde afuera) para que Pedidos, Usuarios y la reconstrucción de Búsqueda le hablen a Catálogo usando el mismo reparto.
- **Guardar en los logs a qué copia mandó cada pedido**, para mostrar en Grafana que la carga se reparte.

El gateway **no tiene reglas del negocio**: solo manda, verifica, limita y reparte.

## Rutas previstas

| Entrada pública | Destino | Autenticación |
| --- | --- | --- |
| Registro y login en `/api/v1/auth/` | Usuarios | Pública; la verificación del token es interna |
| `/api/v1/usuarios/` y `/api/v1/admin/cuentas-cocina` | Usuarios | Token; el servicio controla los permisos |
| `/api/v1/cocinas`, menú de `/api/v1/cocinas/{id}/platos` y detalle de `/api/v1/platos/{id}` | Catálogo | Pública |
| Administración de cocinas y restaurantes | Catálogo | Token de admin; localidad validada por Catálogo |
| `/api/v1/cocina/restaurantes/{restauranteId}/platos` y `/api/v1/cocina/platos/{id}` | Catálogo | Token de Cocina; pertenencia validada por Catálogo |
| `/api/v1/platos/buscar`, `/api/v1/platos/sugerencias` y `/api/v1/zonas` | Búsqueda | Pública |
| Recetas, ingredientes, reposiciones y mermas | Inventario | Token de Cocina; Inventario valida la cocina asignada y rechaza al admin |
| Carrito, pedidos, tablero de cocina y métricas de negocio | Pedidos | Token; permisos sobre sus propios datos |
| `/public/v1/` | Búsqueda | `X-API-Key` |
| `/internal/` | No se reenvía desde la entrada pública | Responde `404` |

El gateway elimina `/api/v1` al reenviar al servicio. Para la capacidad publicada elimina `/public`, conservando `/v1` y los parámetros del contrato. Las rutas específicas de recetas, ingredientes y métricas se distinguen de las rutas generales de Catálogo; `/platos/buscar` se distingue del detalle de un plato.

La entrada interna de Catálogo se expone únicamente en la red de Docker y comparte el balanceo entre sus dos instancias. Pedidos, Usuarios y Búsqueda pueden usarla sin publicar las operaciones internas en el puerto 8080. Notificaciones no tiene rutas de negocio en el gateway.

## Dependencias y fallas previstas

| Dependencia | Si falla |
| --- | --- |
| Usuarios | La verificación espera hasta 300 ms sin reintentar; se impide el acceso privado, mientras las rutas públicas siguen disponibles |
| Un servicio de destino | Falla su ruta; el gateway no presenta una lista vacía ni simula una operación exitosa |
| Una instancia de Catálogo | El balanceo deja temporalmente de usarla tras detectar fallas y conserva la otra instancia |

`POST /public/v1/proximidad` se dirige a Búsqueda conservando el cuerpo JSON y eliminando `/public`. Compara ubicacionCliente, ubicacionSede, radioKm e incluirDarkKitchen; no guarda coordenadas externas. El límite del cuerpo es 64 KiB (413 si se supera); la API key y los límites son los mismos que en GET. No se registran cuerpos con coordenadas.

El gateway no reintenta operaciones que cambian datos. Los reintentos seguros de lectura deben respetar el plazo total de la solicitud ([ADR-005](../docs/adr/ADR-005-comunicacion-entre-servicios.md)). El formato final de errores, los límites de espera y la recuperación de instancias se precisarán al configurar NGINX.

## Seguridad y configuración prevista

- Sobrescribe los headers de identidad definidos por la aplicación con los valores verificados por Usuarios; ningún servicio confía en una identidad enviada por el navegador.
- Las rutas públicas no heredan una identidad suministrada por el cliente. Los servicios se mantienen dentro de la red de Docker.
- Las claves de consumidores se configuran con `PUBLIC_API_KEYS` de [`.env.example`](../.env.example), nunca en archivos versionados con claves reales.
- Limita la capacidad publicada a 60 solicitudes por minuto por clave, con `429` y `Retry-After`; la política por usuario/IP del frontend se definirá al implementar.
- Reenvía o genera `X-Correlation-Id` y registra ruta, estado, duración e instancia de destino. Las contraseñas, tokens y API keys no aparecen en logs.
- CORS se restringe al origen del frontend. Las plantillas y variables propias de NGINX se definirán junto con su configuración.

## Pruebas previstas

- Las rutas superpuestas llegan al servicio correcto; las rutas internas no son accesibles desde la entrada pública.
- Token inválido, Usuarios caído y headers de identidad falsificados no permiten acceso privado.
- API key inválida produce `401`; superar el límite produce `429` con `Retry-After`.
- Proximidad conserva método y JSON, respeta 64 KiB y no registra coordenadas; las rutas `/api/v1/cocina/ingredientes` y de recetas llegan a Inventario.
- Reparto de solicitudes entre ambas instancias de Catálogo y continuidad de las lecturas al detener una.
- El gateway no repite escrituras; los logs permiten identificar la instancia y seguir el `X-Correlation-Id`.
