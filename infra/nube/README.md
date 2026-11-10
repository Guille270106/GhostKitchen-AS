# Despliegue en la nube

La **capacidad publicada** (`/public/v1`, la API que usa el Grupo 5) corre en una máquina virtual de Azure, con HTTPS:

| | |
| --- | --- |
| **URL pública** | `https://57-156-56-219.sslip.io/public/v1` |
| Máquina | Azure for Students · `ghostkitchen-vm` · Chile Central · Standard B2als v2 (2 vCPU, 4 GiB) + 2 GiB de swap · Ubuntu 24.04 |
| IP pública | `57.156.56.219` (estática: se conserva aunque la máquina se detenga) |
| Puertos abiertos | 22 (SSH, solo con clave), 80 y 443 (Caddy) |
| Costo | ~0,053 USD por hora prendida (~38 USD por mes), del crédito de Azure for Students. Detenida, solo el disco y la IP |

## Qué corre y qué no

```
Internet ──HTTPS──▶ Caddy (80/443) ──▶ gateway (API key, 60/min por clave) ──▶ Búsqueda ──▶ Solr
                                                                                   └──▶ Catálogo ×2 ──▶ MongoDB · Memcached
```

- **Sí:** Caddy, gateway, Búsqueda, Solr, las 2 copias de Catálogo, MongoDB, Memcached y Jaeger. Es todo lo que necesita `/public/v1`.
- **No:** Usuarios, Pedidos, Inventario, Notificaciones, RabbitMQ y el frontend. La API publicada no los usa, y así la máquina es más chica y barata y hay menos piezas que puedan fallar.
- Sin Inventario, Búsqueda arma el índice al arrancar con la **disponibilidad de los datos de ejemplo**.
- Es el **mismo código y la misma configuración** que en local (`docker-compose.yml`). `docker-compose.nube.yml` solo agrega Caddy, cierra los demás puertos y apaga lo que en la nube no está.

### Por qué así

| Decisión | Por qué |
| --- | --- |
| Azure for Students | Gratis para estudiantes (100 USD de crédito) y sin tarjeta |
| Una máquina virtual con Docker Compose (IaaS) | Corre lo mismo que en local: lo que probamos es lo que se publica. Una PaaS como Vercel o Render no corre Solr, MongoDB y varios servicios juntos |
| Chile Central | Es una de las regiones que permite la cuenta de estudiante, la más cercana con máquinas disponibles |
| Caddy + sslip.io | HTTPS sin comprar un dominio: `57-156-56-219.sslip.io` apunta a la IP, y Caddy saca y renueva el certificado de Let's Encrypt solo |
| Solo `/public` y `/health` hacia afuera | Lo demás del gateway no se expone en internet (Caddy responde 404) |

## Cómo se despliega (la primera vez)

1. **Prender la máquina:** portal de Azure → `ghostkitchen-vm` → **Iniciar**.
2. **Entrar por SSH**, desde la compu que tiene la clave:
   ```bash
   ssh -i ~/.ssh/ghostkitchen-vm_key.pem ghostkitchen@57.156.56.219
   ```
3. **Bajar el repositorio:**
   ```bash
   git clone https://github.com/Guille270106/GhostKitchen-AS.git
   cd GhostKitchen-AS
   ```
4. **Crear el `.env`** con claves al azar. Imprime **una sola vez** la API key del Grupo 5:
   ```bash
   sh infra/nube/crear-env.sh 57-156-56-219.sslip.io
   ```
5. **Levantar:**
   ```bash
   docker compose -f docker-compose.yml -f docker-compose.nube.yml up -d --build caddy memcached
   ```
6. **Probar desde cualquier compu:**
   ```bash
   curl -H "X-API-Key: <clave>" "https://57-156-56-219.sslip.io/public/v1/sedes/cercana?lat=-31.333&lng=-64.255"
   ```
   Sin la clave responde `401`. Con la clave, la sede más cercana.

## Actualizar a una versión nueva

```bash
cd ~/GhostKitchen-AS
git pull
docker compose -f docker-compose.yml -f docker-compose.nube.yml up -d --build caddy memcached
```

## Operación

| Qué | Cómo |
| --- | --- |
| Ver si todo está arriba | `docker compose -f docker-compose.yml -f docker-compose.nube.yml ps` |
| Ver los logs del gateway | `docker logs -f ghostkitchen-gateway` |
| Dar una clave a otro grupo | Agregarla en `PUBLIC_API_KEYS` del `.env` (separadas por coma) y volver a correr el `up -d` de arriba |
| Ahorrar crédito | Portal de Azure → **Detener**. Al iniciarla, Docker levanta todo solo (`restart: unless-stopped`) |
| Borrar todo al terminar la materia | Portal de Azure → grupo de recursos `ghostkitchen` → **Eliminar** |

## Seguridad

- A la máquina se entra solo con la **clave SSH** (`.pem`), que no se comparte ni se sube al repo. Para que otra integrante entre, se le agrega **su propia** clave pública en `~/.ssh/authorized_keys`.
- El `.env` de la máquina tiene claves al azar, solo lo puede leer el usuario de la máquina y nunca va al repo.
- La API key de cada grupo se manda **por privado**.
- Caddy no registra cada solicitud, así no quedan coordenadas en sus logs. El gateway registra la ruta sin parámetros (ADR-011).

## Limitaciones conocidas

- **El límite por IP de las solicitudes sin clave** cuenta a todos como una sola IP: la de Caddy. El límite que importa, 60 por minuto **por clave**, funciona igual.
- **Los datos son los de ejemplo.** Lo que se carga en local (platos nuevos, stock) no está en la nube, porque no se publican Catálogo ni Inventario para escribir.
- **Una sola máquina:** si se cae, la API publicada no responde hasta que vuelva. Lo aceptamos para el trabajo práctico.
