# MySQL y migraciones — ronda 1

Una instancia MySQL 8.4, tres bases y tres usuarios. Cada usuario tiene permisos únicamente sobre su base; los servicios no reciben la contraseña root. El puerto 3306 permanece dentro de Docker y `mysql-data` conserva los datos.

| Servicio | Base | Usuario | Contraseña en `.env` |
| --- | --- | --- | --- |
| Usuarios | usuarios_db | usuarios | USUARIOS_DB_PASSWORD |
| Inventario | inventario_db | inventario | INVENTARIO_DB_PASSWORD |
| Pedidos | pedidos_db | pedidos | PEDIDOS_DB_PASSWORD |

Copiar `.env.example` como `.env` si todavía no existe y completar las cuatro contraseñas MySQL, además de las de MongoDB que ya requiere el proyecto. No sobrescribir un `.env` existente ni versionarlo.

```powershell
docker compose up -d mysql
docker compose up --build pedidos-migrations
docker compose up -d --build usuarios inventario
docker compose logs mysql usuarios inventario pedidos-migrations
```

Pedidos conserva un trabajo que termina con código 0 tras aplicar sus migraciones. Desde V5 (Inventario) y G5 (Usuarios), cada servicio ejecuta el runner en su propio entrypoint y después queda atendiendo HTTP; reemplazan a los trabajos `inventario-migrations` y `usuarios-migrations`. Repetir el arranque comprueba las versiones sin aplicar nuevamente las ya ejecutadas. El `docker compose up --build` general también los incluye.

## Inicialización y persistencia

`01-bases.sh` solo se ejecuta cuando MySQL inicializa un volumen vacío. Crea bases y usuarios, no tablas del negocio. Los secretos se convierten a literales hexadecimales antes de construir SQL; no se concatenan como código ni se pasan por argumentos de línea de comandos.

Cambiar contraseñas en `.env` no cambia usuarios de un volumen ya inicializado: se debe realizar una rotación explícita en MySQL. No borrar el volumen para actualizar esquemas. Las actualizaciones se hacen mediante migraciones; la eliminación de volúmenes pierde datos.

## Plantilla de migración para los tres servicios

Cada servicio tiene su propio directorio `services/<servicio>/migrations` y su tabla `schema_migrations`. La versión 000001 crea únicamente una fila de identificación para comprobar la plantilla. Las tablas de Usuarios, Inventario y Pedidos empiezan desde 000002 en la ronda 2.

Usamos `golang-migrate` v4.18.3, con su bloqueo MySQL y marca `dirty`. Se aplica en orden ascendente; sin cambios es éxito. Si una migración falla, el proceso termina con error y no arranca el comando del servicio. MySQL puede confirmar DDL parcialmente: el estado dirty exige inspección y reparación manual antes de ajustar la versión; no se fuerza ni se reintenta a ciegas.

Para una nueva migración, agregar ambos archivos a la carpeta de su servicio:

```text
000002_descripcion.up.sql
000002_descripcion.down.sql
```

No editar una migración ya aplicada: agregar otra versión. Los archivos down permiten describir una reversión manual revisada; el arranque solo ejecuta up, nunca borra datos automáticamente. El formato y el bloqueo siguen la [documentación de golang-migrate](https://github.com/golang-migrate/migrate/tree/v4.18.3).

## Conexión al arranque real en la ronda 2

`tools/mysql-startup` genera un ejecutable reutilizable. Primero aplica las migraciones y después reemplaza su proceso por el comando recibido, manteniendo señales y código de salida en Linux. No conecta un servicio que aún no está implementado.

En el Dockerfile de cada servicio, con contexto de build en la raíz del repositorio, agregar una etapa que construya `tools/mysql-startup`, copiar el ejecutable y el directorio de migraciones a la imagen final:

```dockerfile
# Etapa adicional; la etapa de compilación del servicio se define en la ronda 2.
FROM golang:1.24-alpine AS migraciones
WORKDIR /src
COPY tools/mysql-startup/go.mod tools/mysql-startup/go.sum ./
RUN go mod download
COPY tools/mysql-startup/ ./
RUN CGO_ENABLED=0 go build -o /mysql-startup .

# En la imagen final del servicio, después de su propio FROM:
# COPY --from=migraciones /mysql-startup /usr/local/bin/mysql-startup
# COPY services/inventario/migrations /migrations
# ENTRYPOINT ["/usr/local/bin/mysql-startup"]
# CMD ["/app/inventario"]
```

Reemplazar inventario por usuarios o pedidos. Proveer `DB_HOST=mysql`, `DB_PORT=3306`, `DB_USER`, `DB_PASSWORD` y `DB_NAME` propios; `MIGRATIONS_DIR` vale `/migrations` por defecto. Depender de `mysql: service_healthy`. Una vez integrado el entrypoint, se pueden retirar los trabajos separados de ese servicio; no depender únicamente de un trabajo que terminó en un arranque antiguo.

El runner espera conexión hasta 30 s, limita sentencias a 20 s y espera el bloqueo de migración hasta 10 s. No registra contraseñas, DSN ni SQL del driver. Si falla, revisar conectividad, nombres de archivos y `schema_migrations` con las credenciales de desarrollo. Una marca dirty requiere revisar el SQL aplicado antes de cualquier reparación.

## Comprobaciones con Docker encendido

La verificación automática usa Python estándar y Docker en un proyecto separado, `gk-ronda1-check`, con contraseñas de prueba y un volumen descartable. Lo elimina al terminar y no usa `.env`. No ejecutar dos copias al mismo tiempo:

```powershell
python tools/mysql-startup/testdata/verify.py
```

Incluye primer arranque, contraseña con comilla y barra invertida, migraciones repetidas, arranques concurrentes, permisos cruzados, persistencia, SQL inválido y estado dirty, bloqueo del comando si falla la migración, y respuestas 401/429 por IP y 200/429 por clave del gateway. El backend HTTP de esa prueba es un doble de Búsqueda: no prueba el negocio de búsqueda ni Solr.

1. Ejecutar las migraciones dos veces: los trabajos de Usuarios y Pedidos terminan en 0; Inventario queda listo después de migrar.
2. Consultar con cada usuario sus tablas `servicio_metadata` y `schema_migrations`: nombre correcto, versión 1 para Usuarios/Pedidos y 4 para Inventario (V7), sin dirty.
3. Con el usuario de Usuarios, intentar `SELECT * FROM inventario_db.servicio_metadata`: debe denegarse. Repetir las otras combinaciones.
4. Reiniciar MySQL sin eliminar volúmenes: se conservan versiones y metadatos.
5. En una base **descartable**, ejecutar la prueba de integración del runner con `MYSQL_STARTUP_INTEGRATION=1` y sus variables DB. Verifica repetición y bloqueo tras SQL inválido. Deja la base dirty deliberadamente; nunca usar una base del proyecto para esa prueba.

Para entrar al cliente sin exponer la contraseña en argumentos:

```powershell
docker compose exec mysql mysql -uusuarios -p usuarios_db
```

El cliente pide la contraseña. Dentro se puede ejecutar `SELECT * FROM schema_migrations;` y `SELECT * FROM servicio_metadata;`.
