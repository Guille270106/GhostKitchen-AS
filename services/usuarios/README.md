# Usuarios

**Responsable:** Guillermina Sánchez.

> Entrega 1: diseño preliminar; todavía no hay implementación. Las carpetas, rutas, configuración y pruebas indicadas son previstas para el desarrollo desde la Entrega 2.

Se encarga de las cuentas y del login. Puerto **8081**. Lo programamos en la Entrega 2.

## Qué hace

- Registra al cliente con nombre, correo, contraseña y dirección. **No pide localidad**, porque el cliente puede pedir a distintas cocinas en distintos momentos.
- Hace el login y entrega un **token** (JWT). Cuando alguien quiere hacer algo privado, el gateway le pregunta a este servicio si el token es válido.
- Maneja los tres roles: cliente, cocina y admin.
- Recuerda **qué cocina eligió el cliente**, para mostrársela la próxima vez. El cliente la puede cambiar cuando quiera.
- Crea la cuenta Cocina cuando el admin da de alta una cocina (solo si la cocina es de su localidad).
- Las cuentas de admin se cargan al iniciar el sistema: no se pueden registrar solas.

## Qué datos guarda (MySQL)

Una tabla `usuario` con: nombre, mail (no se puede repetir), hash de contraseña con bcrypt, dirección, rol, cocina elegida (si es cliente), cocina asignada (si es una cuenta Cocina) y localidad (si es admin).

La contraseña **nunca** se guarda tal cual ni se devuelve.

## Cómo se organiza por dentro: capas

Es un servicio con pocas reglas, así que usamos las capas de siempre: el **handler** recibe el pedido HTTP, el **servicio** aplica las reglas y el **repositorio** habla con la base (Clase 8).

```
cmd/api/              arranca el servicio
internal/handlers/    rutas HTTP
internal/services/    reglas: mail único, calcular el hash de contraseña, armar el token, permisos del admin
internal/repositories/ acceso a MySQL
internal/domain/      qué es un usuario y sus roles
internal/clients/     para pedirle datos a Catálogo
migrations/           tablas y carga de los admins
```

## Rutas

Salvo las rutas técnicas comunes, la tabla muestra rutas internas del servicio. El frontend entra por `/api/v1` a través del gateway; las rutas entre servicios no se publican al exterior.

| Ruta | Quién la usa | Para qué |
| --- | --- | --- |
| `POST /auth/registro` | cualquiera | Crear una cuenta de cliente |
| `POST /auth/login` | cualquiera | Iniciar sesión y recibir el token |
| `GET /auth/verify` | el gateway | Ver si un token es válido |
| `PUT /usuarios/me/cocina` | cliente | Elegir o cambiar la cocina |
| `POST /admin/cuentas-cocina` | admin | Crear la cuenta de una cocina de su localidad |

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

## Configuración prevista

`JWT_SECRET`, `JWT_TTL_MINUTES` y `USUARIOS_DB_PASSWORD` están anticipadas en [`.env.example`](../../.env.example). La conexión a MySQL, el puerto y la dirección interna de Catálogo se definirán al implementar el servicio. Cada servicio accederá solo a su propia base.

## Pruebas previstas

- Correo único, contraseña guardada como hash bcrypt y nunca devuelta, token vencido o mal firmado.
- Alta de una cuenta Cocina rechazada si la cocina pertenece a otra localidad, está inactiva o no se puede verificar.
- Registro y login contra MySQL real; verificación del token desde el gateway sin confiar en headers de identidad del cliente.
