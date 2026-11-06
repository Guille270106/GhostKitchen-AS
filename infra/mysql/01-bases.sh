#!/bin/bash
# Funciona tanto ejecutado como cargado por la imagen oficial en el primer arranque.
set -e

# Convertir secretos en literales hexadecimales evita interpolarlos como código SQL.
# No se imprime SQL ni se pasan contraseñas en argumentos de procesos.
crear_base() {
    local base="$1" usuario="$2" password="$3" password_hex
    test -n "$password" || { echo "Falta contraseña de $usuario" >&2; return 1; }
    password_hex="$(printf '%s' "$password" | od -An -v -tx1 | tr -d ' \n')"
    MYSQL_PWD="${MYSQL_ROOT_PASSWORD:?Falta MYSQL_ROOT_PASSWORD}" mysql --protocol=socket -uroot <<SQL
CREATE DATABASE IF NOT EXISTS \`$base\` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
SET @clave = CONVERT(X'$password_hex' USING utf8mb4);
SET @crear = CONCAT('CREATE USER IF NOT EXISTS ''$usuario''@''%'' IDENTIFIED BY ', QUOTE(@clave));
PREPARE crear_usuario FROM @crear;
EXECUTE crear_usuario;
DEALLOCATE PREPARE crear_usuario;
GRANT ALL PRIVILEGES ON \`$base\`.* TO '$usuario'@'%';
SQL
}

crear_base usuarios_db usuarios "${USUARIOS_DB_PASSWORD:?Falta USUARIOS_DB_PASSWORD}"
crear_base inventario_db inventario "${INVENTARIO_DB_PASSWORD:?Falta INVENTARIO_DB_PASSWORD}"
crear_base pedidos_db pedidos "${PEDIDOS_DB_PASSWORD:?Falta PEDIDOS_DB_PASSWORD}"
