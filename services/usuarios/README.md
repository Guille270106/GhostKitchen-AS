# Usuarios

**Responsable:** Guillermina Sánchez.

> **Estado (G6):** el servicio arranca con MySQL, aplica sus migraciones al iniciar y ya registra clientes (`POST /auth/registro`). El login con JWT (G7), las cuentas Cocina (G8) y la verificación en el gateway (G9) vienen después.

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

Es un servicio con pocas reglas, así que usamos las capas de siempre: el **handler** recibe el pedido HTTP, el **servicio** aplica las reglas y el **repositorio** habla con la base (Clase 8).

```
cmd/api/                       arranca el servicio (conexión a MySQL, servidor HTTP, apagado ordenado)
internal/handlers/             rutas HTTP, correlation id, log por solicitud y errores Problem Details
internal/services/             reglas: mail único, hash de contraseña, token, permisos del admin (G6 a G8)
internal/repositories/         qué necesita el servicio de la base (interfaz)
internal/repositories/mysql/   la implementación con MySQL
internal/domain/               qué es un usuario, sus roles y qué datos lleva cada rol
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
| `POST /auth/registro` | cualquiera | Crear una cuenta de cliente (G6, ver abajo) |
| `POST /auth/login` | cualquiera | Iniciar sesión y recibir el token |
| `GET /auth/verify` | el gateway | Ver si un token es válido |
| `PUT /usuarios/me/cocina` | cliente | Elegir o cambiar la cocina |
| `POST /admin/cuentas-cocina` | admin | Crear la cuenta de una cocina de su localidad |

### `POST /auth/registro` (HU01)

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

El cierre de sesión descarta el token en el frontend; no implica revocación inmediata del token emitido. El gateway usa `/auth/verify` en una ubicación interna para `auth_request`, sin publicar esa operación al navegador.

**El token lleva:** quién es, su rol y su mail. Si es admin, también su localidad; si es una cuenta Cocina, su cocina. Se firma con una clave que no está en el código.

## Con quién habla

- **Le pregunta a Catálogo** por una cocina cuando el admin crea una cuenta Cocina, para controlar que sea de su localidad.
- No manda ni escucha mensajes.

## Dependencias y fallas previstas

| Dependencia | Para qué | Si falla |
| --- | --- | --- |
| MySQL (`usuarios_db`) | Cuentas, roles y cocina elegida | No se puede registrar, iniciar sesión ni verificar tokens; las rutas privadas quedan indisponibles |
| Catálogo | Validar la cocina y la localidad del admin al crear una cuenta Cocina | Se impide el alta; no se interpreta la falta de respuesta como una cocina inexistente |

La consulta a Catálogo tiene un máximo de 800 ms por intento y un reintento, dentro del plazo total de 3 s de [ADR-005](../../docs/adr/ADR-005-comunicacion-entre-servicios.md). El gateway espera como máximo 300 ms por la verificación del token y no la reintenta.

## Configuración

| Variable | Valor por defecto | Para qué |
| --- | --- | --- |
| `DB_HOST`, `DB_PORT` | `mysql`, `3306` | Dónde está MySQL |
| `DB_USER`, `DB_NAME` | `usuarios`, `usuarios_db` | Usuario y base propios: no ve las otras bases |
| `DB_PASSWORD` | (obligatoria) | `USUARIOS_DB_PASSWORD` del `.env` |
| `PORT` | `8081` | Puerto HTTP |

`JWT_SECRET` y `JWT_TTL_MINUTES` se usan desde G7.

## Pruebas

```
cd services/usuarios
go test ./...
```

Las de MySQL real se omiten salvo con `TEST_MYSQL=1` y las variables `DB_*` de una base descartable.

Previstas para G6 a G9:

- Correo único, contraseña guardada como hash bcrypt y nunca devuelta, token vencido o mal firmado.
- Alta de una cuenta Cocina rechazada si la cocina pertenece a otra localidad, está inactiva o no se puede verificar.
- Registro y login contra MySQL real; verificación del token desde el gateway sin confiar en headers de identidad del cliente.
