# MySQL y migraciones

Usamos **una sola instancia de MySQL 8.4** con tres bases, una por servicio, y un usuario por base. Cada usuario solo puede entrar a la suya, y ningún servicio conoce la contraseña de root. El puerto 3306 queda adentro de Docker y los datos se guardan en el volumen `mysql-data`.

| Servicio | Base | Usuario | Contraseña en el `.env` |
| --- | --- | --- | --- |
| Usuarios | `usuarios_db` | `usuarios` | `USUARIOS_DB_PASSWORD` |
| Inventario | `inventario_db` | `inventario` | `INVENTARIO_DB_PASSWORD` |
| Pedidos | `pedidos_db` | `pedidos` | `PEDIDOS_DB_PASSWORD` |

Las contraseñas (y la de root, `MYSQL_ROOT_PASSWORD`) van en el `.env`; `infra/nube/crear-env.sh` las genera al azar (ver [Variables del `.env`](../../README.md#variables-del-env)).

```bash
docker compose up -d mysql
docker compose up -d --build usuarios inventario pedidos
docker compose logs mysql usuarios inventario pedidos
```

El `docker compose up --build` general también los levanta.

## Cómo se crean las bases

`01-bases.sh` corre **solo la primera vez**, cuando el volumen de MySQL está vacío. Crea las bases y los usuarios, no las tablas. Las contraseñas se pasan a MySQL como texto hexadecimal, así una contraseña con comillas no rompe el SQL.

Cambiar una contraseña en el `.env` no cambia la del usuario en un volumen que ya existe: hay que cambiarla también en MySQL. Para cambiar las tablas no se borra el volumen (se pierden los datos): se agrega una migración.

## Migraciones

Cada servicio tiene sus migraciones en `services/<servicio>/migrations` y su propia tabla `schema_migrations`:

| Servicio | Migraciones |
| --- | --- |
| Usuarios | 000001 a 000002 |
| Inventario | 000001 a 000008 |
| Pedidos | 000001 a 000008 |

Usamos `golang-migrate` v4.18.3. Cada migración tiene un archivo `up` y uno `down`:

```text
000009_descripcion.up.sql
000009_descripcion.down.sql
```

- Una migración ya aplicada **no se edita**: se agrega otra.
- Al arrancar solo se aplican las `up`; las `down` sirven para revertir a mano, después de revisarlas.
- Si una migración falla, el servicio no arranca. MySQL puede dejar la migración a medias (estado `dirty`): hay que revisar qué se aplicó antes de arreglarla. Nunca se fuerza ni se reintenta a ciegas.

## Cómo se aplican al arrancar

`tools/mysql-startup` es un programa chico que **primero aplica las migraciones y después arranca el servicio**. Cada Dockerfile (Usuarios, Inventario y Pedidos) lo compila en una etapa aparte, copia el programa y la carpeta `migrations` a la imagen final, y lo usa como `ENTRYPOINT`:

```dockerfile
FROM golang:1.24-alpine AS migraciones
WORKDIR /src
COPY tools/mysql-startup/go.mod tools/mysql-startup/go.sum ./
RUN go mod download
COPY tools/mysql-startup/ ./
RUN CGO_ENABLED=0 go build -trimpath -o /mysql-startup .

# En la imagen final del servicio:
COPY --from=migraciones /mysql-startup /usr/local/bin/mysql-startup
COPY services/pedidos/migrations /migrations
ENTRYPOINT ["/usr/local/bin/mysql-startup"]
CMD ["/app/pedidos"]
```

Lee `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD` y `DB_NAME`; `MIGRATIONS_DIR` vale `/migrations` si no se dice otra cosa. Espera la conexión hasta 30 s, corta cada sentencia a los 20 s y espera el bloqueo de migraciones hasta 10 s. No escribe contraseñas ni SQL en los logs. Si se arranca dos veces, no vuelve a aplicar lo que ya está.

## Cómo lo probamos

`tools/mysql-startup/testdata/verify.py` levanta un MySQL descartable en un proyecto de Docker aparte (`gk-ronda1-check`), con contraseñas de prueba, y lo borra al terminar. No usa el `.env`. No hay que correr dos a la vez:

```bash
python tools/mysql-startup/testdata/verify.py
```

Prueba el primer arranque, una contraseña con comilla y barra invertida, migraciones repetidas y al mismo tiempo, que un usuario no pueda leer la base de otro, que los datos sigan después de reiniciar, una migración con SQL inválido (estado `dirty`) y que el servicio no arranque si la migración falla. También prueba los límites del gateway (`401`/`429` por IP y `200`/`429` por clave), con un servicio falso en lugar de Búsqueda.

El test de integración del propio runner (`MYSQL_STARTUP_INTEGRATION=1` y sus variables `DB_*`) deja la base en `dirty` a propósito: correrlo **solo en una base descartable**, nunca en la del proyecto.

Para entrar a una base sin poner la contraseña en el comando:

```bash
docker compose exec mysql mysql -uusuarios -p usuarios_db
```

El cliente pide la contraseña. Adentro, `SELECT * FROM schema_migrations;` muestra la versión aplicada.
