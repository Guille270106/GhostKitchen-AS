# GhostKitchen

GhostKitchen es una plataforma de pedidos para **4 cocinas fantasma** (*dark kitchens*) de Córdoba. Son cocinas sin salón que venden solo por delivery. Desde cada una despachan hasta 4 restaurantes, que comparten la misma cocina, el mismo personal y los mismos ingredientes.

El cliente:
1. elige la **cocina** desde la que quiere pedir (escribe su zona y el sistema le sugiere la más cercana) y puede cambiarla en otro momento;
2. arma **un solo pedido** con platos de varios restaurantes de esa cocina;
3. lo recibe en un único envío.

El objetivo del diseño es que **nunca se venda un plato si no hay ingredientes para prepararlo**, aunque muchos clientes pidan al mismo tiempo. Esa garantía se comprobará cuando se implemente Inventario.

Proyecto del Práctico Integrador de **Arquitectura de Software 2026** (UCC).

**Integrantes:** Lucía, Guillermina y Valentina.

**Repositorio público:** [GhostKitchen-AS](https://github.com/Guille270106/GhostKitchen-AS).


## Tabla de contenidos

- [Cocinas y restaurantes](#cocinas-y-restaurantes)
- [Flujo principal](#flujo-principal)
- [Arquitectura](#arquitectura)
- [Capacidad publicada para otros grupos](#capacidad-publicada-para-otros-grupos)
- [Tecnologías](#tecnologías)
- [Cómo ejecutarlo](#cómo-ejecutarlo)
- [Logs y trazas](#logs-y-trazas)
- [Estructura del repositorio](#estructura-del-repositorio)
- [Forma de trabajo](#forma-de-trabajo)
- [Documentación](#documentación)

## Cocinas y restaurantes

| Sede | Dirección | Restaurantes (máx. 4) |
| --- | --- | --- |
| Cocina Nueva Córdoba | Obispo Trejo 1050 | Mostaza · Sushi Club · Kentucky · Grido |
| Cocina Cerro | Rafael Núñez 4520 | Johnny B. Good · Il Gatto · Green Eat · Havanna |
| Cocina Villa Allende | Av. Goycoechea 680 | Big Pons · KFC · El Club de la Milanesa · Dunkin' Donuts |
| Cocina Barrio Jardín | Av. Richieri 3150 | Dean & Dennys · Sushi Pop · La Cabrera · Café Martínez |

> Los nombres son de restaurantes reales, usados como datos de ejemplo de un trabajo académico. Los platos, precios y stock son inventados.

## Flujo principal

1. **Elegir la cocina.** El cliente se registra sin localidad. Al ingresar escribe su zona ("Argüello", "Güemes", "Mendiolaza"…) y el sistema le sugiere la cocina más cercana, o elige una de las 4 a mano. Puede cambiarla en otro momento.
2. El cliente busca platos de **todos los restaurantes de esa cocina** y los suma a un mismo carrito.
3. Al agregar un plato se **reservan sus ingredientes por 10 minutos**. Si no confirma a tiempo, la reserva vence y los ingredientes se liberan.
4. Al confirmar, se descuentan esos ingredientes del stock y se crea **un único pedido**.
5. La **cocina** recibe el pedido completo, con los platos agrupados por restaurante (una bolsa por marca). Lo acepta o lo rechaza y lo avanza: aceptado → en preparación → listo → en camino. El cliente confirma la entrega. Si la cocina rechaza o el cliente cancela (solo antes de que se acepte), el pedido se cancela y vuelven los ingredientes.
6. Cada cambio de estado se avisa al cliente por mail.
7. El **admin** gestiona cocinas, restaurantes y cuentas de su localidad y consulta métricas. El **personal de cada cocina** administra sus propios platos, ingredientes, recetas, stock, reposiciones y mermas, además de los pedidos.

## Arquitectura

Diseñamos el sistema con **6 microservicios en Go** y un **API Gateway** (NGINX, puerto 8080) por donde entra todo. Cada servicio tiene su propia base de datos:

| Servicio | Qué hace | Base | Cómo se organiza por dentro | Puerto |
| --- | --- | --- | --- | --- |
| Usuarios | Registro, login y roles; recuerda la cocina elegida | MySQL | Capas | 8081 |
| Catálogo (2 copias) | Cocinas, restaurantes y platos: qué se vende y a qué precio | MongoDB + Memcached | Capas + caché | 8082 |
| Búsqueda | Buscar platos, sugerir la **cocina más cercana**, calcular proximidad externa y atender lo que publicamos | Solr | Capas | 8083 |
| **Inventario** | Ingredientes, stock, recetas y reservas de 10 minutos | MySQL | **Hexagonal** | 8084 |
| **Pedidos** | Carrito, pedido, estados y la confirmación con Inventario y el proveedor | MySQL | **Hexagonal + DDD** | 8085 |
| Notificaciones | Un mail por cada cambio de estado | MongoDB | Capas | 8086 |

Los servicios se comunican de dos formas:
- **llamadas directas (HTTP)** cuando necesitamos la respuesta en el momento, por ejemplo para reservar stock. Siempre con un tiempo máximo de espera y sin repetir operaciones si se reintenta;
- **mensajes (RabbitMQ)** para lo que puede hacerse un rato después: los mails, actualizar la búsqueda y avisar que venció una reserva.

Las imágenes de los diagramas están únicamente en **`docs/diagrams/`**. Qué datos guarda cada servicio y cómo funciona cada paso se explica en **[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)**.

## Capacidad publicada para otros grupos

**Cocinas, platos y proximidad de sedes externas:** otro grupo nos manda una ubicación y le decimos qué cocina le queda más cerca y a cuántos km. Después puede ver qué platos se pueden pedir ahora en esa cocina, con filtros, orden y páginas. La ampliación 1.1.0 agrega `POST /v1/proximidad`: ubicacionCliente, ubicacionSede y radioKm para comparar si la sede externa está cerca. incluirDarkKitchen=true consulta además nuestra cocina más cercana al cliente. No se guardan las ubicaciones ni se selecciona entre varias sedes externas.

- Contrato OpenAPI v1: [docs/contracts/v1/openapi.yaml](docs/contracts/v1/openapi.yaml)
- Guía para el consumidor (autenticación, errores, ejemplos, versionado): [docs/contracts/README.md](docs/contracts/README.md)
- Mock: `docker compose up` → http://localhost:4010
- URL pública en la nube: `https://57-156-56-219.sslip.io/public/v1` (con API key; ver [infra/nube](infra/nube/README.md)).

## Tecnologías

**Backend**

- Go + Gin para los microservicios
- NGINX como API Gateway y para repartir la carga
- MySQL, MongoDB, Solr (búsqueda), Memcached (caché) y RabbitMQ (mensajes)
- Observabilidad: logs JSON con `correlation_id` y `trace_id`, y trazas distribuidas con OpenTelemetry y Jaeger. Métricas con Prometheus (`/metrics` en cada servicio con API), tablero en Grafana y alertas.
- Pruebas: tests de Go para las reglas; integración con bases y RabbitMQ reales en Docker; test de contrato del proveedor y de nuestra API; carga con k6.

**Frontend:** React + Vite.

**Contrato y puesta en marcha:** OpenAPI (para escribir el contrato), Prism (para el mock), Docker y Docker Compose.

## Cómo ejecutarlo

Requisitos: Docker y Docker Compose, con Docker Desktop y su motor de contenedores Linux iniciados. La primera ejecución descarga las imágenes (MongoDB, Solr, NGINX, Go, Prism) y necesita conexión a Internet.

1. Crear el `.env` con claves al azar. Desde la raíz del repositorio (en Windows, desde **Git Bash**):

   ```bash
   sh infra/nube/crear-env.sh localhost
   ```

   El script completa todas las variables, no pisa un `.env` que ya exista e imprime una API key para probar la capacidad publicada. El `.env` no se sube al repositorio. Si preferís armarlo a mano, las variables están en [Variables del `.env`](#variables-del-env).
2. Desde la raíz del repositorio:

   ```bash
   docker compose up --build
   ```

   Levanta **todo**: frontend, gateway, los seis servicios (Notificaciones incluido), las bases, RabbitMQ, Mailpit y el monitoreo. Cada servicio con MySQL aplica sus migraciones al arrancar.

3. Probar la capacidad publicada, ya implementada, a través del gateway (en PowerShell usar `curl.exe`):

   ```bash
   curl -H "X-API-Key: <la clave que imprimió el script>" http://localhost:8080/public/v1/sedes
   ```

   O todas las operaciones juntas, en PowerShell: `.\scripts\probar-api-publica.ps1 -Clave <la clave>`.
4. Entrar al sistema en http://localhost:5173:

   - **Cliente:** cualquiera se registra desde "Crear cuenta".
   - **Admin:** hay uno por localidad, `admin.<sede>@ghostkitchen.dev` (`barrio-jardin`, `cerro`, `nueva-cordoba` o `villa-allende`), con la contraseña `ADMIN_PASSWORD` del `.env`.
   - **Cocina:** las 4 cocinas de los datos iniciales no traen cuenta: la crea el admin de su localidad. (Una cocina nueva, en cambio, se crea desde "Cocinas y restaurantes" junto con su cuenta.) Por ejemplo, para Nueva Córdoba, en Git Bash y desde la raíz del repositorio:

     ```bash
     CLAVE=$(grep ^ADMIN_PASSWORD= .env | cut -d= -f2)
     TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login -H "Content-Type: application/json" \
       -d "{\"email\":\"admin.nueva-cordoba@ghostkitchen.dev\",\"password\":\"$CLAVE\"}" | sed -E 's/.*"token":"([^"]+)".*/\1/')
     curl -X POST http://localhost:8080/api/v1/admin/cuentas-cocina -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
       -d '{"cocinaId":"nueva-cordoba","nombre":"Cocina Nueva Córdoba","email":"cocina.nueva-cordoba@ghostkitchen.dev","password":"cocina1234"}'
     ```

     Después la cocina entra con ese mail y esa contraseña.

| Servicio | URL |
| --- | --- |
| Frontend | http://localhost:5173 |
| Mails que manda Notificaciones (Mailpit) | http://localhost:8025 |
| Capacidad publicada | http://localhost:8080/public/v1/sedes (header `X-API-Key` con una clave de `PUBLIC_API_KEYS`) |
| Capacidad publicada en la nube | https://57-156-56-219.sslip.io/public/v1/sedes (con la API key del grupo; ver [infra/nube](infra/nube/README.md)) |
| Mock de la capacidad publicada | http://localhost:4010/v1/sedes (header `X-API-Key` con cualquier valor) |
| Lecturas de Catálogo para el frontend | http://localhost:8080/api/v1/cocinas |
| Panel de Solr (desarrollo) | http://localhost:8983 |
| MongoDB con Compass (desarrollo) | `mongodb://root:<MONGO_ROOT_PASSWORD>@localhost:27017` |
| Trazas en Jaeger | http://localhost:16686 |
| Tablero en Grafana | http://localhost:3000 (se ve sin entrar; para editar, `admin` y `GRAFANA_ADMIN_PASSWORD`) |
| Prometheus y alertas | http://localhost:9090/alerts |
| Panel de RabbitMQ (desarrollo) | http://localhost:15672 (usuario y contraseña de `RABBITMQ_USER` y `RABBITMQ_PASSWORD`) |

Para detener todo: `docker compose down`. Con `-v` se borran también los volúmenes de MongoDB y Solr: al arrancar de nuevo se cargan los datos iniciales y se reconstruye el índice.

### Variables del `.env`

Las marcadas como **obligatorias** las pide Docker Compose: si falta alguna, no arranca y dice cuál. Para usar en local alcanza con valores inventados; `infra/nube/crear-env.sh` las completa todas.

| Variable | Para qué | Obligatoria hoy |
| --- | --- | --- |
| `PUBLIC_API_KEYS` | Claves de los grupos que consumen nuestra API, separadas por coma (por ejemplo `clave-local`). Si queda vacía, `/public/v1` responde 401 a todos | No, pero sin ella no se puede probar la API |
| `MONGO_ROOT_PASSWORD` | Contraseña del usuario `root` de MongoDB | Sí |
| `CATALOGO_DB_PASSWORD` | Contraseña del usuario de Catálogo en MongoDB | Sí |
| `NOTIFICACIONES_DB_PASSWORD` | Contraseña del usuario de Notificaciones en MongoDB | Sí |
| `MYSQL_ROOT_PASSWORD` | Contraseña del usuario `root` de MySQL | Sí |
| `USUARIOS_DB_PASSWORD` | Contraseña del usuario de Usuarios en MySQL | Sí |
| `INVENTARIO_DB_PASSWORD` | Contraseña del usuario de Inventario en MySQL | Sí |
| `PEDIDOS_DB_PASSWORD` | Contraseña del usuario de Pedidos en MySQL | Sí |
| `RABBITMQ_USER` | Usuario de RabbitMQ, para el panel y los servicios (por ejemplo `ghostkitchen`) | Sí |
| `RABBITMQ_PASSWORD` | Contraseña de ese usuario | Sí |
| `JWT_SECRET` | Clave aleatoria de al menos 32 caracteres con la que Usuarios firma los tokens del login (por ejemplo, la que da `openssl rand -base64 48`) | Sí |
| `JWT_TTL_MINUTES` | Duración del token en minutos (si falta, `120`) | No |
| `ADMIN_PASSWORD` | Contraseña inicial de los 4 admins (una por localidad), de 8 caracteres o más. Si falta, los admins no se crean | No, pero sin ella no hay admins |
| `PROVIDER_BASE_URL`, `PROVIDER_API_KEY` | Dirección y clave de la API del grupo proveedor | No |
| `GRAFANA_ADMIN_PASSWORD` | Contraseña del administrador de Grafana. Si falta, `admin` (solo sirve en local: el tablero se ve sin entrar) | No |

Las contraseñas de MongoDB van dentro de una URL: usar solo letras y números. Si se cambia una contraseña de una base ya creada, hay que borrar su volumen (`docker compose down -v`).

El mock (Prism) devuelve ejemplos estáticos: no calcula distancias ni filtros. Sin Docker, con Node.js y npm:

```bash
npx --yes @stoplight/prism-cli@5.16.0 mock -h 127.0.0.1 -p 4010 docs/contracts/v1/openapi.yaml
```

Los ejemplos de uso están en [la guía del contrato](docs/contracts/README.md#ejemplos).

## Logs y trazas

**Logs estructurados.** El gateway, Búsqueda y Catálogo escriben una línea JSON por solicitud con los mismos campos: `servicio`, `correlation_id`, `trace_id`, `metodo`, `ruta`, `estado` y duración. La ruta va sin parámetros ni cuerpo, así que no quedan coordenadas, textos de búsqueda ni API keys.

**Correlación.** El gateway toma el `X-Correlation-Id` que manda el cliente (o genera uno), lo reenvía y lo devuelve en la respuesta. Búsqueda lo pasa a Catálogo cuando le pide datos. Con ese id se encuentran todas las líneas de una solicitud en los tres servicios:

```powershell
docker compose logs gateway busqueda catalogo | Select-String "mi-prueba-1"
```

**Trazas distribuidas (OpenTelemetry + Jaeger).** Cada solicitud genera una traza con un span por paso: el gateway, cada servicio con API (Usuarios, Catálogo, Búsqueda, Inventario y Pedidos), cada llamada de un servicio a otro, cada consulta a Solr y cada comando de Catálogo a MongoDB. Por ejemplo, agregar al carrito muestra `gateway → pedidos → catalogo` y `pedidos → inventario`. El `trace_id` viaja en el header estándar `traceparent` (W3C) y también aparece en los logs, así que se puede pasar de un log a su traza y al revés. Para verla:

1. Hacer una solicitud con un id conocido:
   ```powershell
   curl.exe -X POST -H "X-API-Key: clave-local" -H "X-Correlation-Id: mi-prueba-1" -H "Content-Type: application/json" --data-binary "@scripts/proximidad-ejemplo.json" http://localhost:8080/public/v1/proximidad
   ```
2. Abrir http://localhost:16686, elegir el servicio `gateway` o `busqueda` y buscar por la etiqueta `ghostkitchen.correlation_id=mi-prueba-1` (o pegar el `trace_id` del log).

Para ver la traza que cruza dos servicios y la base, pedir una reconstrucción del índice: `docker compose exec busqueda wget -qO- --post-data= http://127.0.0.1:8083/internal/reindexar`. En Jaeger aparece `busqueda → catalogo → MongoDB` (y las escrituras en Solr). La traza del arranque de Búsqueda (`preparar índice al iniciar`) muestra lo mismo.

| Qué se ve en una traza | Ejemplo |
| --- | --- |
| Gateway | `/public/v1/proximidad`, con su duración y estado |
| Búsqueda | el span de la ruta (`/v1/proximidad`) y uno por cada consulta a Solr (`solr cocinas/select`) |
| Catálogo | `/internal/platos` y uno por comando a MongoDB (`find`, `count`) |

Para no mandar datos sensibles a Jaeger, los spans llevan la ruta como plantilla (`/v1/sedes/:sedeId/platos`) y la operación de Solr, no la URL con parámetros. Por la misma razón el gateway solo crea su span cuando la solicitud no tiene parámetros (`?...`); esas solicitudes igual se trazan desde Búsqueda. En desarrollo se guardan todas las trazas.

## Estructura del repositorio

**MySQL:** una instancia con una base y un usuario por servicio (Usuarios, Inventario y Pedidos). Cada servicio aplica sus migraciones al arrancar; la [guía de MySQL y migraciones](infra/mysql/README.md) explica cómo.

```
GhostKitchen/
├── README.md
├── SPEC.md                       # Alcance, reglas de negocio y criterios de aceptación
├── docker-compose.yml            # Puesta en marcha local con un solo comando
├── docs/
│   ├── ARCHITECTURE.md           # Arquitectura: diagramas, servicios, datos, comunicación
│   ├── POSTMORTEM.md             # Informe de la jornada de fallas
│   ├── adr/                      # Decisiones arquitectónicas (D1 a D13)
│   ├── pruebas/                  # Colección de Postman: la API de punta a punta por el gateway
│   └── contracts/                # Capacidad publicada: contrato OpenAPI v1 + guía + CHANGELOG
├── services/                     # Un microservicio por carpeta, cada uno con su README
│   └── usuarios/  catalogo/  busqueda/  inventario/  pedidos/  notificaciones/
├── frontend/                     # React + Vite
├── gateway/                      # API Gateway NGINX
├── infra/                        # MySQL, MongoDB, RabbitMQ, Prometheus, Grafana y la nube
├── scripts/                      # Prueba del contrato, de la API publicada y jornada de fallas
└── tests/carga/                  # Pruebas de carga con k6
```

## Forma de trabajo

- **Metodología:** Scrum, con la cátedra como Product Owner.
- **Aplicación al trabajo:** las HU y sus criterios se toman de SPEC; cada tarea referencia una HU, una RN o un ADR, tiene responsable y estado pendiente/en curso/en revisión/terminada. El equipo planifica contra los hitos 9/10 y 23/10, revisa los avances en cada reunión y aplica la Definition of Done de SPEC antes de integrar. La aprobación del dominio y la evaluación de la cátedra se registran cuando ocurren.
- **Ramas (Git Flow):**
  - `main`: solo lo que entregamos. Cada entrega pasa de `develop` a `main` y se marca con un tag (`entrega-1`, `entrega-2`, `final`).
  - `develop`: donde juntamos el trabajo de todos.
  - `feature/...`, `docs/...`, `fix/...`: una rama por tarea. Sale de `develop` y vuelve a `develop` con un **pull request que revisa otro integrante**.
- **Revisión automática:** desde la Entrega 2, GitHub Actions corre los tests en cada pull request. Para la entrega final se sumaron el test de contrato de la API publicada (`contrato.yml`) y la prueba de punta a punta con todo el sistema levantado (`e2e.yml`).
- **Decisiones:** cada decisión importante va en un ADR. Si cambiamos de idea, hacemos un ADR nuevo y no borramos el viejo.
- **Contraseñas y claves:** nunca en el repositorio. Van en el `.env` de cada integrante, que está en `.gitignore`; los nombres de las variables están en [Variables del `.env`](#variables-del-env).

## Documentación

| Documento | Contenido |
| --- | --- |
| [SPEC.md](SPEC.md) | Qué tiene que hacer el sistema: roles, funcionalidades, reglas y cómo sabemos que algo está terminado |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Cómo está armado: diagramas, servicios, datos, comunicación y cómo funciona cada paso |
| [docs/adr/](docs/adr/README.md) | Las decisiones que tomamos y por qué (D1 a D13) |
| [docs/POSTMORTEM.md](docs/POSTMORTEM.md) | La jornada de fallas: qué apagamos, qué vio el usuario y qué aprendimos |
| [docs/contracts/](docs/contracts/README.md) | El contrato de lo que publicamos, con ejemplos y errores |
| [docs/pruebas/](docs/pruebas/README.md) | La colección de Postman que prueba la API de punta a punta (100 pruebas) y cómo correrla |
| [services/](services/README.md) | Qué hace cada microservicio y de quién depende |
