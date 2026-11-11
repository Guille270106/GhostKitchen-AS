# ADR-012 — Balanceo de carga

- **Estado:** Aceptada, implementada (`catalogo-1` y `catalogo-2`, con la caché compartida) y validada con la prueba de carga y la caída de una copia (ver [Cómo lo validamos](#cómo-lo-validamos))
- **Fecha:** 2026-10-16
- **Responsable:** Guillermina Sánchez
- **Decisión del enunciado:** D12 (qué servicio se balancea, cómo se reparte el tráfico, cómo se verifica que una instancia está disponible y qué pasa si se cae una)
- **Relacionado con:** [ADR-007](ADR-007-cache.md) · [ADR-010](ADR-010-resiliencia.md) · [ADR-011](ADR-011-observabilidad.md) · [gateway](../../gateway/README.md)

## Contexto

El enunciado pide balancear la carga de al menos un servicio y mostrar qué pasa cuando se cae una instancia. Tenemos que elegir **qué** servicio, **cómo** repartir y **cómo** detectar una instancia caída con lo que ofrece NGINX gratuito.

## Decisión

### Qué servicio: Catálogo, con 2 copias

- Es el que **más lecturas** recibe: menú, cocinas y detalle de platos, y además Búsqueda lo usa para reconstruir el índice.
- **No guarda estado propio:** las dos copias leen la misma MongoDB y la misma Memcached ([ADR-007](ADR-007-cache.md)). Da igual cuál conteste, así que no hacen falta sesiones pegadas a una copia.
- Que las dos copias arranquen a la vez no es un problema: la carga inicial de datos inserta sin pisar e ignora los duplicados.

No balanceamos Pedidos ni Inventario todavía: tienen procesos de fondo (vencer reservas, publicar el outbox) que con dos copias hay que coordinar. Con Catálogo mostramos el patrón en el caso más simple y más útil.

### Cómo se reparte: round-robin

NGINX manda las solicitudes por turnos (`upstream catalogo` con dos `server`). Elegimos round-robin porque las dos copias son iguales y las solicitudes cuestan parecido (lecturas cortas). *Least connections* rinde más cuando las solicitudes tienen costos muy distintos, que no es nuestro caso. Lo revisamos si la prueba de carga muestra lo contrario.

### Cómo se detecta una copia caída

NGINX gratuito **no revisa las copias por su cuenta**: solo se entera cuando una solicitud falla (chequeo **pasivo**). Lo configuramos así:

```nginx
upstream catalogo {
    server catalogo-1:8082 max_fails=3 fail_timeout=10s;
    server catalogo-2:8082 max_fails=3 fail_timeout=10s;
    keepalive 16;
}
# en las rutas de Catálogo:
proxy_next_upstream error timeout http_502 http_503 http_504;
proxy_next_upstream_tries 2;
proxy_next_upstream_timeout 3s;
```

- Si una copia falla **3 veces en 10 s**, NGINX deja de mandarle tráfico durante 10 s y después vuelve a probar. Exigir varias fallas evita sacar y meter una copia por una demora pasajera (*flapping*).
- Si una solicitud de **lectura** falla por conexión o timeout, NGINX la manda **a la otra copia**, dentro del mismo plazo de 3 s. NGINX no reenvía solicitudes que cambian datos (POST, PATCH) salvo que se lo indiquemos, y no lo vamos a hacer: repetir una escritura puede duplicarla.
- Además, cada copia tiene `/ready` (MongoDB responde), que usa Docker y que se ve en el tablero ([ADR-011](ADR-011-observabilidad.md)).

### Qué pasa si se cae una copia

| Momento | Qué pasa | Qué ve el usuario |
| --- | --- | --- |
| Primeras solicitudes a la copia caída | Fallan y NGINX las reintenta en la otra | Nada (alguna respuesta un poco más lenta) |
| Después de 3 fallas en 10 s | NGINX deja de mandarle tráfico | Nada |
| Cada 10 s | NGINX vuelve a probarla; si responde, la suma de nuevo | Nada |
| Se caen las dos | `503` con Problem Details | "No podemos mostrar el menú en este momento"; la búsqueda sigue andando |

### Cómo lo mostramos

- El log del gateway registra `upstream` (la copia que atendió) y cada copia de Catálogo registra su nombre (`INSTANCIA`).
- En el tablero: solicitudes por copia y estado de cada una. En la demo, apagamos `catalogo-1` con `docker compose stop catalogo-1` durante una prueba de carga y mostramos que no hay errores visibles.

### Cómo quedó implementado

- **Compose:** `catalogo-1` y `catalogo-2` (la segunda con `extends` de la primera). Cada una tiene `INSTANCIA` con su nombre, que va en cada línea de log y en el header `X-Instancia` de cada respuesta; el log del gateway lo anota como `instancia`.
- **Las dos responden al nombre `catalogo`** dentro de Docker (alias de red). Búsqueda, Usuarios y Pedidos siguen usando `http://catalogo:8082`: Docker les da las dos direcciones y, si una no contesta, Go prueba con la otra. Así no hizo falta una entrada interna en el gateway.
- **Gateway:** `upstream catalogo` con las dos copias, `max_fails=3 fail_timeout=10s`, y `proxy_next_upstream error timeout http_502 http_503 http_504` con 2 intentos dentro de 3 s (en `proxy-comun.conf`). NGINX no reenvía POST ni PATCH a la otra copia.
- **El outbox lo publica una sola copia** (`PUBLICAR_OUTBOX=true` solo en `catalogo-1`): si publicaran las dos, el mismo evento saldría dos veces y en otro orden. Las dos escriben en el mismo outbox. Si `catalogo-1` se cae, los eventos esperan hasta que vuelva; no se pierden.
- **Probado** con NGINX y dos copias de prueba: por turnos (1, 2, 1, 2…); con `catalogo-1` apagada, 8 de 8 solicitudes con 200 desde `catalogo-2`, **sin ningún error**; al volver, NGINX la suma de nuevo a los 10 s.

## Alternativas que pensamos

| Alternativa | A favor | En contra | Qué decidimos |
| --- | --- | --- | --- |
| **`docker compose --scale catalogo=2`** con el DNS de Docker | Menos configuración | NGINX resuelve el nombre una sola vez al arrancar y no ve cada copia por separado; no se puede mostrar qué copia atendió | Dos servicios con nombre (`catalogo-1`, `catalogo-2`) |
| **Least connections** | Mejor con costos desparejos | Nuestras solicitudes cuestan parecido | Round-robin |
| **HAProxy o Traefik** (chequeos activos) | Revisan las copias por su cuenta | Otra pieza más; NGINX ya es nuestro gateway | NGINX con chequeo pasivo |
| **Balancear Búsqueda** | También recibe tráfico | Hay que coordinar las reconstrucciones del índice | Más adelante, si la carga lo pide |

## Consecuencias

- Catálogo sigue atendiendo si se cae una copia, sin que el usuario lo note.
- Con chequeo pasivo, las primeras solicitudes a una copia caída fallan antes de que NGINX la saque; las de lectura se reintentan en la otra.
- La caché compartida es necesaria para que las dos copias muestren lo mismo.
- El gateway sigue siendo **una sola instancia**: si se cae, se cae todo. Lo aceptamos para el trabajo.

## Cómo lo validamos

- **Prueba de carga (hecho):** con 100 usuarios durante 3 minutos, cada copia atendió **8.950 solicitudes**, la mitad exacta ([ADR-013](ADR-013-capacidad-y-costos.md)).
- **Caída de una copia (hecho):** durante la carga apagamos una copia con `docker compose stop`: **0 errores**, la otra atendió todo (8.815 respuestas) y hubo algunas solicitudes de hasta 1 s mientras NGINX la sacaba.

