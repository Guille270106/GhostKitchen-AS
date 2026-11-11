# Usuarios

**Responsable:** Guillermina Sánchez.

Registra clientes, hace el login de los tres roles con JWT, verifica los tokens para el gateway, carga los admins iniciales y deja que cada admin cree las cuentas Cocina de su localidad.

Se encarga de las cuentas y del login. Puerto **8081**, solo dentro de Docker.

## Qué hace

- Registra al cliente con nombre, correo, contraseña y dirección. **No pide localidad**, porque el cliente puede pedir a distintas cocinas en distintos momentos.
- Hace el login y entrega un **token** (JWT). Cuando alguien quiere hacer algo privado, el gateway le pregunta a este servicio si el token es válido.
- Maneja los tres roles: cliente, cocina y admin. Admin administra cocinas, restaurantes y cuentas y consulta métricas; Cocina administra sus propios platos, pedidos e inventario, incluidas recetas, reposiciones y mermas. Conservamos la cocina asignada en el token, sin agregar otro rol.
- Recuerda **qué cocina eligió el cliente**, para mostrársela la próxima vez. El cliente la puede cambiar cuando quiera.
- Crea la cuenta Cocina cuando el admin da de alta una cocina (solo si la cocina es de su localidad).
- Las cuentas de admin se cargan al iniciar el sistema: no se pueden registrar solas.

## Qué datos guarda (MySQL)

Una tabla `usuario` con: nombre, mail (no se puede repetir), hash de contraseña con bcrypt, dirección, rol, cocina elegida (si es cliente), cocina asignada (si es una cuenta Cocina) y localidad (si es admin).

La contraseña **nunca** se guarda tal cual ni se devuelve.

## Cómo se organiza por dentro: capas

Es un servicio con pocas reglas, así que usamos las capas de siempre: el **handler** recibe el pedido HTTP, el **servicio** aplica las reglas y el **repositorio** habla con la base.

```
cmd/api/                       arranca el servicio (conexión a MySQL, servidor HTTP, apagado ordenado)
internal/handlers/             rutas HTTP, correlation id, log por solicitud y errores Problem Details
internal/services/             reglas: mail único, hash de contraseña, token, permisos del admin
internal/repositories/         qué necesita el servicio de la base (interfaz)
internal/repositories/mysql/   la implementación con MySQL
internal/domain/               qué es un usuario, sus roles y qué datos lleva cada rol
internal/clients/              cliente HTTP de Catálogo
internal/seed/                 los admins iniciales
migrations/                    000001 plantilla, 000002 tabla usuario
```

En Docker, el `ENTRYPOINT` es `mysql-startup`: primero aplica las migraciones y después arranca el servicio. Si una migración falla, el servicio no arranca ([guía de migraciones](../../infra/mysql/README.md)).

## La tabla `usuario`

| Columna | Para qué |
| --- | --- |
| `id` | UUID que genera el servicio |
| `nombre`, `direccion` | Datos de la persona |
| `email` | En minúsculas y único (`uq_usuario_email`) |
| `password_hash` | Hash bcrypt; la contraseña nunca se guarda |
| `rol` | `cliente`, `cocina` o `admin` |
| `cocina_elegida_id` | Solo cliente: la última cocina que eligió |
| `cocina_asignada_id` | Solo cuenta Cocina: la cocina que administra (obligatoria) |
| `localidad` | Solo admin: la localidad que administra (obligatoria) |
| `activo`, `creado_en`, `actualizado_en` | Estado y fechas |

Los `CHECK` de la tabla repiten las reglas por rol, así una cuenta mal armada no entra ni por error del código. El dominio las valida antes, para responder un 400 claro.

## Rutas

Salvo las rutas técnicas comunes, la tabla muestra rutas internas del servicio. El frontend entra por `/api/v1` a través del gateway; las rutas entre servicios no se publican al exterior.

| Ruta | Quién la usa | Para qué |
| --- | --- | --- |
| `GET /health` | Docker | El proceso está vivo (no mira MySQL) |
| `GET /ready` | Docker y el gateway | MySQL responde y la tabla existe; si no, 503 |
| `POST /auth/registro` | cualquiera | Crear una cuenta de cliente (ver abajo) |
| `POST /auth/login` | cualquiera | Iniciar sesión y recibir el token |
| `GET /auth/verify` | el gateway | Ver si un token es válido |
| `PUT /usuarios/me/cocina` | cliente | Elegir o cambiar la cocina |
| `POST /admin/cuentas-cocina` | admin | Crear la cuenta de una cocina de su localidad |

### `POST /auth/registro`

```json
{ "nombre": "Ana", "email": "ana@mail.com", "password": "clave-secreta-1", "direccion": "Obispo Trejo 323" }
```

| Respuesta | Cuándo |
| --- | --- |
| `201` con la cuenta (sin contraseña ni hash) y `Location: /usuarios/{id}` | Se creó la cuenta, siempre con rol cliente |
| `400 DATOS_INVALIDOS` | Falta un dato, el correo no es válido, la contraseña no tiene entre 8 caracteres y 72 bytes, o vino un campo de más (por ejemplo `rol` o `localidad`) |
| `409 EMAIL_REPETIDO` | Ya existe una cuenta con ese correo, sin importar mayúsculas. No se muestra nada de la otra cuenta |
| `503 SERVICIO_NO_DISPONIBLE` | MySQL no respondió |

El correo se guarda en minúsculas. La contraseña se guarda solo como hash bcrypt (costo 10) y no aparece en la respuesta ni en los logs. El tope de 72 bytes es el de bcrypt: una contraseña más larga se rechaza en vez de recortarse sin avisar.

### `POST /auth/login`

```json
{ "email": "ana@mail.com", "password": "clave-secreta-1" }
```

Con datos correctos, `200` (con `Cache-Control: no-store`):

```json
{ "token": "eyJ...", "tipo": "Bearer", "venceEn": "2026-11-13T12:00:00Z", "usuario": { "id": "...", "rol": "cliente", ... } }
```

Con el correo inexistente, la contraseña mal o la cuenta desactivada, siempre la misma respuesta: `401 CREDENCIALES_INVALIDAS` "Correo o contraseña incorrectos". Cuando el correo no existe, igual se compara contra un hash falso, así la respuesta tarda lo mismo y por el tiempo no se puede saber qué correos tienen cuenta.

### El token (JWT)

Firmado con HS256 y `JWT_SECRET`. Dura `JWT_TTL_MINUTES` (120 por defecto). Lleva solo lo necesario para autorizar:

| Campo | Contenido |
| --- | --- |
| `sub` | id del usuario |
| `email`, `rol` | correo y rol (`cliente`, `cocina` o `admin`) |
| `cocina` | solo cuenta Cocina: su cocina |
| `localidad` | solo admin: su localidad |
| `iss`, `iat`, `exp` | `ghostkitchen-usuarios`, cuándo se emitió y cuándo vence |

Nunca lleva la contraseña, el hash ni la dirección. Al verificar solo se acepta HS256 (un token con `alg: none` se rechaza), el emisor tiene que ser el nuestro, el vencimiento es obligatorio y los datos tienen que corresponder al rol.

### `GET /auth/verify` (para el gateway)

Recibe `Authorization: Bearer <token>`. Si el token sirve, responde `204` sin cuerpo y con la identidad en headers, que el gateway copia hacia los servicios:

| Header | Cuándo |
| --- | --- |
| `X-Usuario-Id` | siempre |
| `X-Usuario-Email` | siempre |
| `X-Rol` | siempre: `cliente`, `cocina` o `admin` |
| `X-Cocina-Id` | solo cuenta Cocina (Inventario y Pedidos lo usan para ver solo lo suyo) |
| `X-Localidad` | solo admin, codificada para URL porque un header solo admite ASCII: `Nueva%20C%C3%B3rdoba` |

Si falta el token o no sirve, `401 NO_AUTENTICADO` con `WWW-Authenticate: Bearer`, sin decir por qué. No consulta MySQL: el token ya trae todo, así verificar tarda menos de 1 ms y sigue andando aunque la base esté lenta. La contra es que un token emitido sigue sirviendo hasta que vence, aunque se desactive la cuenta; por eso dura poco.

### Admins iniciales

Los admins no se registran: se cargan al arrancar el servicio, uno por localidad, las mismas de las cocinas de Catálogo. Están en `internal/seed/admins.json`:

| Correo | Localidad |
| --- | --- |
| `admin.barrio-jardin@ghostkitchen.dev` | Barrio Jardín |
| `admin.cerro@ghostkitchen.dev` | Cerro de las Rosas |
| `admin.nueva-cordoba@ghostkitchen.dev` | Nueva Córdoba |
| `admin.villa-allende@ghostkitchen.dev` | Villa Allende |

La contraseña inicial de los cuatro es la de `ADMIN_PASSWORD` en el `.env`, así no queda escrita en el repositorio. Sin esa variable no se crea ninguno y el log lo avisa. La carga se puede repetir en cada arranque: si el correo ya existe, no lo toca ni le cambia la contraseña.

### `POST /admin/cuentas-cocina`

Con `Authorization: Bearer <token de admin>`:

```json
{ "nombre": "Cocina Nueva Córdoba", "email": "cocina.nc@ghostkitchen.dev", "password": "clave-cocina-1", "cocinaId": "nueva-cordoba" }
```

Usuarios verifica el token él mismo: no confía en headers de identidad. Después valida los datos y recién ahí le pregunta a Catálogo por la cocina (`GET /cocinas/:id`, 800 ms y un reintento solo si no hubo respuesta o fue 5xx).

| Respuesta | Cuándo |
| --- | --- |
| `201` con la cuenta (rol `cocina` y su `cocinaAsignadaId`) | La cocina existe, está activa y es de la localidad del admin |
| `401 NO_AUTENTICADO` | Falta el token o no sirve |
| `403 SIN_PERMISO` | Quien llama no es admin |
| `403 OTRA_LOCALIDAD` | La cocina es de otra localidad |
| `422 COCINA_INVALIDA` | Catálogo responde 404: no existe o está inactiva |
| `409 EMAIL_REPETIDO` / `400 DATOS_INVALIDOS` | Igual que en el registro |
| `503 CATALOGO_NO_DISPONIBLE` con `Retry-After: 5` | Catálogo no respondió: no se crea la cuenta, y no se toma como "no existe" |

El cierre de sesión descarta el token en el frontend; no implica revocación inmediata del token emitido. El gateway usa `/auth/verify` en una ubicación interna para `auth_request`, sin publicar esa operación al navegador.

**El token lleva:** quién es, su rol y su mail. Si es admin, también su localidad; si es una cuenta Cocina, su cocina. Se firma con una clave que no está en el código.

## Con quién habla

- **Le pregunta a Catálogo** por una cocina cuando el admin crea una cuenta Cocina, para controlar que sea de su localidad.
- No manda ni escucha mensajes.

## Dependencias y fallas

| Dependencia | Para qué | Si falla |
| --- | --- | --- |
| MySQL (`usuarios_db`) | Cuentas, roles y cocina elegida | No se puede registrar ni iniciar sesión. Los tokens ya emitidos se siguen verificando, porque verificar no usa la base |
| Catálogo | Validar la cocina y la localidad del admin al crear una cuenta Cocina | Se impide el alta; no se interpreta la falta de respuesta como una cocina inexistente |

La consulta a Catálogo tiene un máximo de 800 ms por intento y un reintento, dentro del plazo total de 3 s de [ADR-005](../../docs/adr/ADR-005-comunicacion-entre-servicios.md). El gateway espera como máximo 300 ms por la verificación del token y no la reintenta.

## Configuración

| Variable | Valor por defecto | Para qué |
| --- | --- | --- |
| `DB_HOST`, `DB_PORT` | `mysql`, `3306` | Dónde está MySQL |
| `DB_USER`, `DB_NAME` | `usuarios`, `usuarios_db` | Usuario y base propios: no ve las otras bases |
| `DB_PASSWORD` | (obligatoria) | `USUARIOS_DB_PASSWORD` del `.env` |
| `PORT` | `8081` | Puerto HTTP |
| `JWT_SECRET` | (obligatoria, al menos 32 bytes) | Clave para firmar los tokens. Sin ella el servicio no arranca |
| `JWT_TTL_MINUTES` | `120` | Cuánto dura un token |
| `CATALOGO_URL` | `http://catalogo:8082` | Dónde está Catálogo |
| `ADMIN_PASSWORD` | (vacía) | Contraseña inicial de los admins; sin ella no se cargan |

## Pruebas

```
cd services/usuarios
go test ./...
```

Las de MySQL real se omiten salvo con `TEST_MYSQL=1` y las variables `DB_*` de una base descartable.

Qué cubren:

- Correo único, contraseña guardada como hash bcrypt y nunca devuelta, token vencido o mal firmado.
- Alta de una cuenta Cocina rechazada si la cocina pertenece a otra localidad, está inactiva o no se puede verificar.
- Registro y login contra MySQL real; verificación del token desde el gateway sin confiar en headers de identidad del cliente.
