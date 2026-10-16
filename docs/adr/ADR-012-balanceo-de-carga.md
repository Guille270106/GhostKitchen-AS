# ADR-012 — Balanceo de carga

- **Estado:** Propuesta. En la Entrega 2 Catálogo corre con una copia; las 2 copias se suman junto con la caché
- **Fecha:** 2026-10-16
- **Responsable:** Guillermina Sánchez
- **Decisión del enunciado:** D12 (qué servicio se balancea, cómo se reparte el tráfico, cómo se verifica que una instancia está disponible y qué pasa si se cae una)
- **Relacionado con:** [ADR-007](ADR-007-cache.md) · [ADR-010](ADR-010-resiliencia.md) · [ADR-011](ADR-011-observabilidad.md) · [gateway](../../gateway/README.md)

## Contexto

El enunciado pide balancear la carga de al menos un servicio y mostrar qué pasa cuando se cae una instancia. Tenemos que elegir **qué** servicio, **cómo** repartir y **cómo** detectar una instancia caída con lo que ofrece NGINX gratuito.

## Decisión

### Qué servicio: Catálogo, con 2 copias

- Es el que **más lecturas** recibe: menú, cocinas y detalle de platos, y además Búsqueda lo usa para reconstruir el índice.
- **No guarda estado propio:** las dos copias leen la misma MongoDB y la misma Memcached ([ADR-007](ADR-007-cache.md)). Da igual cuál conteste, así que no hacen falta sesiones pegadas a una copia (Clase 6, 2.4).
- Que las dos copias arranquen a la vez no es un problema: la carga inicial de datos inserta sin pisar e ignora los duplicados.

No balanceamos Pedidos ni Inventario todavía: tienen procesos de fondo (vencer reservas, publicar el outbox) que con dos copias hay que coordinar. Con Catálogo mostramos el patrón en el caso más simple y más útil.

### Cómo se reparte: round-robin

NGINX manda las solicitudes por turnos (`upstream catalogo` con dos `server`). Elegimos round-robin porque las dos copias son iguales y las solicitudes cuestan parecido (lecturas cortas). *Least connections* rinde más cuando las solicitudes tienen costos muy distintos, que no es nuestro caso (Clase 6). Lo revisamos si la prueba de carga muestra lo contrario.

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

- Si una copia falla **3 veces en 10 s**, NGINX deja de mandarle tráfico durante 10 s y después vuelve a probar. Exigir varias fallas evita sacar y meter una copia por una demora pasajera (*flapping*, Clase 6).
- Si una solicitud de **lectura** falla por conexión o timeout, NGINX la manda **a la otra copia**, dentro del mismo plazo de 3 s. NGINX no reenvía solicitudes que cambian datos (POST, PATCH) salvo que se lo indiquemos, y no lo vamos a hacer: repetir una escritura puede duplicarla.
- Además, cada copia tiene `/ready` (MongoDB responde), que usa Docker y que vamos a mostrar en el tablero ([ADR-011](ADR-011-observabilidad.md)).

### Qué pasa si se cae una copia

| Momento | Qué pasa | Qué ve el usuario |
| --- | --- | --- |
| Primeras solicitudes a la copia caída | Fallan y NGINX las reintenta en la otra | Nada (alguna respuesta un poco más lenta) |
| Después de 3 fallas en 10 s | NGINX deja de mandarle tráfico | Nada |
| Cada 10 s | NGINX vuelve a probarla; si responde, la suma de nuevo | Nada |
| Se caen las dos | `503` con Problem Details | "No podemos mostrar el menú en este momento"; la búsqueda sigue andando |

### Cómo lo mostramos

- El log del gateway ya registra `upstream` (la copia que atendió). Cada copia de Catálogo también va a registrar su nombre (`INSTANCIA`).
- En el tablero: solicitudes por copia y estado de cada una. En la demo, apagamos `catalogo-1` con `docker compose stop catalogo-1` durante una prueba de carga y mostramos que no hay errores visibles.

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
- El gateway sigue siendo **una sola instancia**: si se cae, se cae todo. Lo aceptamos para el trabajo (Clase 6).

## Cómo lo vamos a validar

- **Prueba de carga** ([ADR-013](ADR-013-capacidad-y-costos.md)): menú de una cocina con 1 y con 2 copias; comparar p95 y solicitudes por segundo.
- **Caída de una copia:** durante la carga, `docker compose stop catalogo-1`; esperar 0 errores visibles (o muy pocos, en los primeros segundos) y que todo el tráfico pase a `catalogo-2`. Volver a levantarla y ver que NGINX la suma de nuevo.

## Referencias

- Clase 6: balanceo de carga, round-robin y least connections, chequeos de salud, flapping, servicios sin estado y sesiones, el gateway como punto único.
