// Se ejecuta UNA sola vez, cuando MongoDB arranca con el volumen vacío.
// Crea el usuario de Catálogo con permisos solo sobre su base (Database per Service, ADR-003).
// Si se cambia la contraseña en .env, hay que borrar el volumen: docker compose down -v
const base = db.getSiblingDB('catalogo_db');
base.createUser({
  user: 'catalogo',
  pwd: process.env.CATALOGO_DB_PASSWORD,
  roles: [{ role: 'readWrite', db: 'catalogo_db' }],
});
print('Usuario catalogo creado en catalogo_db');
