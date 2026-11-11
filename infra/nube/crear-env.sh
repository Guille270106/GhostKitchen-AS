#!/bin/sh
# Crea el .env de la máquina en la nube con claves al azar. Se corre UNA vez, en la
# máquina, desde la raíz del repositorio:
#
#   sh infra/nube/crear-env.sh 57-156-56-219.sslip.io
#
# - No pisa un .env que ya exista.
# - Las contraseñas no salen de la máquina: nadie las necesita para usar la API publicada.
# - Imprime UNA sola vez la API key del Grupo 5, para mandársela por privado.
set -eu

DOMINIO="${1:?Uso: sh infra/nube/crear-env.sh <dominio>, por ejemplo 57-156-56-219.sslip.io}"
if [ -f .env ]; then
  echo "Ya hay un .env: no lo toco. Si querés uno nuevo, borralo primero." >&2
  exit 1
fi

azar() { openssl rand -hex "$1"; }
CLAVE_GRUPO5="g5-$(azar 24)"

umask 077   # el .env queda legible solo para el usuario de la máquina
cat > .env <<FIN
# Generado por infra/nube/crear-env.sh el $(date -u +%Y-%m-%d). No se versiona.
DOMINIO_PUBLICO=$DOMINIO

# API keys de los grupos que consumen nuestra API, separadas por coma.
PUBLIC_API_KEYS=$CLAVE_GRUPO5

# Bases de datos (en la nube solo se usan las de MongoDB; el resto es para que el compose arme).
MONGO_ROOT_PASSWORD=$(azar 16)
CATALOGO_DB_PASSWORD=$(azar 16)
MYSQL_ROOT_PASSWORD=$(azar 16)
USUARIOS_DB_PASSWORD=$(azar 16)
INVENTARIO_DB_PASSWORD=$(azar 16)
PEDIDOS_DB_PASSWORD=$(azar 16)
NOTIFICACIONES_DB_PASSWORD=$(azar 16)
RABBITMQ_USER=ghostkitchen
RABBITMQ_PASSWORD=$(azar 16)
JWT_SECRET=$(azar 32)
JWT_TTL_MINUTES=120
ADMIN_PASSWORD=$(azar 16)
PROVIDER_BASE_URL=
PROVIDER_API_KEY=
FIN

echo ".env creado para $DOMINIO."
echo "API key del Grupo 5 (copiala y mandala por privado; no queda en ningún otro lado que en .env):"
echo "  $CLAVE_GRUPO5"
