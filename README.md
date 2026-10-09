# GhostKitchen

GhostKitchen es una plataforma de pedidos para **4 cocinas fantasma** (*dark kitchens*) de Córdoba. Son cocinas sin salón que venden solo por delivery. Desde cada una despachan hasta 4 restaurantes, que comparten la misma cocina, el mismo personal y los mismos ingredientes.

El cliente:
1. elige la **cocina** desde la que quiere pedir (escribe su zona y el sistema le sugiere la más cercana) y puede cambiarla en otro momento;
2. arma **un solo pedido** con platos de varios restaurantes de esa cocina;
3. lo recibe en un único envío.

El objetivo del diseño es que **nunca se venda un plato si no hay ingredientes para prepararlo**, aunque muchos clientes pidan al mismo tiempo. Esa garantía se comprobará cuando se implemente Inventario.

Proyecto del Práctico Integrador de **Arquitectura de Software 2026** (UCC).

**Integrantes:** Lucía, Guillermina y Valentina. **Repositorio público:** URL pendiente de informar. La carpeta de trabajo revisada todavía no tiene metadatos Git; verificar el acceso de la cátedra antes de entregar.

> **Estado (Entrega 1 — 9/10/2026): diseño y contrato con mock.**
> -  Alcance, reglas de negocio y criterios de aceptación ([SPEC.md](SPEC.md)).
> -  Arquitectura: diagramas de contexto y contenedores, límites de los servicios y propiedad de los datos ([docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)).
> -  Capacidad seleccionada, con contrato OpenAPI versionado y mock configurado con Prism ([docs/contracts](docs/contracts/README.md)).
> -  Decisiones D1 y D8, y versiones iniciales de D3 y D5 ([docs/adr](docs/adr/README.md)).
> -  Estructura inicial de los servicios y sus dependencias ([services/](services/README.md)).
> -  Implementación de los microservicios, gateway, bases de datos, mensajería y frontend: desde la Entrega 2.

## Tabla de contenidos

- [Cocinas y restaurantes](#cocinas-y-restaurantes)
- [Flujo principal](#flujo-principal)
- [Arquitectura](#arquitectura)
- [Capacidad publicada para otros grupos](#capacidad-publicada-para-otros-grupos)
- [Tecnologías](#tecnologías)
- [Cómo ejecutarlo](#cómo-ejecutarlo)
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
7. El **admin** gestiona las cocinas de su localidad, sus restaurantes, los platos con su receta y el stock de ingredientes.

## Arquitectura

Diseñamos el sistema con **6 microservicios en Go** y un **API Gateway** (NGINX, puerto 8080) por donde entra todo. Cada servicio tiene su propia base de datos:

| Servicio | Qué hace | Base | Cómo se organiza por dentro | Puerto |
| --- | --- | --- | --- | --- |
| Usuarios | Registro, login y roles; recuerda la cocina elegida | MySQL | Capas | 8081 |
| Catálogo (2 copias) | Cocinas, restaurantes y platos: qué se vende y a qué precio | MongoDB + Memcached | Capas + caché | 8082 |
| Búsqueda | Buscar platos, sugerir la **cocina más cercana** y atender lo que publicamos | Solr | Capas | 8083 |
| **Inventario** | Ingredientes, stock, recetas y reservas de 10 minutos | MySQL | **Hexagonal** | 8084 |
| **Pedidos** | Carrito, pedido, estados y la confirmación con Inventario y el proveedor | MySQL | **Hexagonal + DDD** | 8085 |
| Notificaciones | Un mail por cada cambio de estado | MongoDB | Capas | 8086 |

Los servicios se comunican de dos formas:
- **llamadas directas (HTTP)** cuando necesitamos la respuesta en el momento, por ejemplo para reservar stock. Siempre con un tiempo máximo de espera y sin repetir operaciones si se reintenta;
- **mensajes (RabbitMQ)** para lo que puede hacerse un rato después: los mails, actualizar la búsqueda y avisar que venció una reserva.

Los diagramas, qué datos guarda cada servicio y cómo funciona cada paso están en **[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)**.

## Capacidad publicada para otros grupos

**Cocina más cercana y platos disponibles:** otro grupo nos manda una ubicación y le decimos qué cocina le queda más cerca y a cuántos km. Después puede ver qué platos se pueden pedir ahora en esa cocina, con filtros, orden y páginas.

- Contrato OpenAPI v1: [docs/contracts/v1/openapi.yaml](docs/contracts/v1/openapi.yaml)
- Guía para el consumidor (autenticación, errores, ejemplos, versionado): [docs/contracts/README.md](docs/contracts/README.md)
- Mock: `docker compose up` → http://localhost:4010
- URL pública en la nube: se publica en la Entrega 2.

## Tecnologías

**Backend (lo programamos desde la Entrega 2)**

- Go + Gin para los microservicios
- NGINX como API Gateway y para repartir la carga
- MySQL, MongoDB, Solr (búsqueda), Memcached (caché) y RabbitMQ (mensajes)
- Monitoreo: Prometheus, Grafana, Loki y Jaeger
- Tests: los de Go, testcontainers (tests contra bases reales) y k6 (tests de carga)

**Frontend (Entrega 2):** React + Vite.

**Contrato y puesta en marcha:** OpenAPI (para escribir el contrato), Prism (para el mock), Docker y Docker Compose.

## Cómo ejecutarlo

Requisitos: Docker y Docker Compose, con Docker Desktop y su motor de contenedores Linux iniciados. La primera ejecución descarga la imagen de Prism y necesita conexión a Internet.

Desde la raíz del repositorio:

```bash
docker compose up
```

El comando levanta únicamente el mock de esta entrega. Para iniciarlo en segundo plano y detenerlo:

```bash
docker compose up -d contract-mock
docker compose down
```

Prueba de una respuesta:

```bash
curl -H "X-API-Key: demo" http://localhost:4010/v1/sedes
```

En PowerShell usar `curl.exe`. Sin Docker, con Node.js y npm:

```bash
npx --yes @stoplight/prism-cli@5.16.0 mock -h 127.0.0.1 -p 4010 docs/contracts/v1/openapi.yaml
```

Prism devuelve ejemplos estáticos. No calcula distancias ni filtros, no reserva ingredientes y no implementa el límite real de solicitudes. Los ejemplos de uso están en [la guía del contrato](docs/contracts/README.md#ejemplos).

| Servicio | URL |
| --- | --- |
| Mock de la capacidad publicada | http://localhost:4010/v1/sedes (header `X-API-Key` con cualquier valor) |

En esta entrega no hace falta ninguna contraseña. Cuando sumemos los servicios, hay que copiar `.env.example` como `.env` y completarlo. El `.env` no se sube al repositorio.

## Estructura del repositorio

```
GhostKitchen/
├── README.md
├── SPEC.md                       # Alcance, reglas de negocio y criterios de aceptación
├── docker-compose.yml            # Puesta en marcha local con un solo comando
├── .env.example                  # Variables necesarias (sin secretos reales)
├── docs/
│   ├── ARCHITECTURE.md           # Arquitectura: diagramas, servicios, datos, comunicación
│   ├── adr/                      # Decisiones arquitectónicas (D1, D3, D5 y D8 en esta entrega)
│   └── contracts/                # Capacidad publicada: contrato OpenAPI v1 + guía + CHANGELOG
├── services/                     # Un microservicio por carpeta, cada uno con su README
│   └── usuarios/  catalogo/  busqueda/  inventario/  pedidos/  notificaciones/
└── gateway/                      # API Gateway NGINX (la configuración llega en la Entrega 2)
```

## Forma de trabajo

- **Metodología:** Scrum, con la cátedra como Product Owner.
- **Aplicación al trabajo:** las HU y sus criterios se toman de SPEC; cada tarea referencia una HU, una RN o un ADR, tiene responsable y estado pendiente/en curso/en revisión/terminada. El equipo planifica contra los hitos 9/10 y 23/10, revisa los avances en cada reunión y aplica la Definition of Done de SPEC antes de integrar. La aprobación del dominio y la evaluación de la cátedra se registran cuando ocurren.
- **Ramas (Git Flow):**
  - `main`: solo lo que entregamos. Cada entrega pasa de `develop` a `main` y se marca con un tag (`entrega-1`, `entrega-2`, `final`).
  - `develop`: donde juntamos el trabajo de todos.
  - `feature/...`, `docs/...`, `fix/...`: una rama por tarea. Sale de `develop` y vuelve a `develop` con un **pull request que revisa otro integrante**.
- **Revisión automática:** desde la Entrega 2, GitHub Actions corre los tests en cada pull request.
- **Decisiones:** cada decisión importante va en un ADR. Si cambiamos de idea, hacemos un ADR nuevo y no borramos el viejo.
- **Contraseñas y claves:** nunca en el repositorio; solo subimos `.env.example` con los nombres de las variables.

## Documentación

| Documento | Contenido |
| --- | --- |
| [SPEC.md](SPEC.md) | Qué tiene que hacer el sistema: roles, funcionalidades, reglas y cómo sabemos que algo está terminado |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Cómo está armado: diagramas, servicios, datos, comunicación y cómo funciona cada paso |
| [docs/adr/](docs/adr/README.md) | Las decisiones que tomamos y por qué (D1, D3, D5 y D8 en esta entrega) |
| [docs/contracts/](docs/contracts/README.md) | El contrato de lo que publicamos, con ejemplos y errores |
| [services/](services/README.md) | Qué hace cada microservicio y de quién depende |
