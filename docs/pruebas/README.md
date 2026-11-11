# Pruebas de la API con Postman

[`GhostKitchen-pruebas.postman_collection.json`](GhostKitchen-pruebas.postman_collection.json) prueba la API de punta a punta **a través del gateway**, como la usa el frontend. Tiene 73 requests y 100 pruebas automáticas, y se corre entera con *Run collection*. La última corrida completa (11/11/2026) pasó las 100.

Es una prueba de integración del sistema levantado. Los tests de cada servicio (unitarios, de integración con su base y de contrato) están en su carpeta y corren en GitHub Actions, igual que la prueba de punta a punta [`services/pedidos/testdata/e2e.mjs`](../../services/pedidos/testdata/e2e.mjs), que recorre el flujo principal con todo el sistema levantado.

## Qué prueba

| Grupo | Qué cubre |
| --- | --- |
| 1. Usuarios y gateway | Registro, mail repetido (409), registro sin dirección (400), login de cliente, admin y cocina, clave mala (401), cuenta de cocina de otra localidad (403), sin token (401) |
| 2. Catálogo | Cocinas y menú, detalle de plato, cocina nueva del admin con un restaurante, plato nuevo de la cocina, activar sin receta (409, RN18), cambiar precio |
| 3. Búsqueda | Búsqueda con palabra completa y a medias, filtros y orden, solo con stock, API publicada con y sin API key |
| 4. Inventario | Ingredientes (alta, datos inválidos), recetas, reposición, merma, merma mayor al stock, admin en una ruta de cocina (403) |
| 5. Pedidos (cliente) | Carrito, plato de otra cocina (409), sin stock (409), confirmar con dirección de entrega, misma `Idempotency-Key` (no crea otro), sin clave y sin dirección (400) |
| 6. Pedidos (cocina y admin) | Aceptar, rechazar un pedido aceptado (409), avanzar hasta *En camino*, el cliente confirma la entrega (*Entregado*), métricas del admin y de otra localidad (403) |
| 7. Errores y casos borde | Ruta inexistente y ruta `/internal` (404 en JSON con headers de seguridad), health del gateway, sede más cercana y sin cobertura, JSON roto (400), método equivocado, rol equivocado (403), token inventado (401) |
| 8. Cancelar un pedido | Un segundo pedido que el cliente cancela (*Cancelado*) y no se puede volver a cancelar (409) |

## Cómo correrla

1. Levantar el sistema (`docker compose up -d`, ver el [README principal](../../README.md#cómo-ejecutarlo)) y esperar a que todo esté `healthy`.
2. En Postman: **Import** → este archivo.
3. En la colección → pestaña **Variables**, completar:
   - `adminPassword`: la `ADMIN_PASSWORD` del `.env`;
   - `apiKey`: una de las claves de `PUBLIC_API_KEYS` del `.env`.

   Las claves no se guardan en el repo: se completan en cada compu.
4. Clic derecho en la colección → **Run**:
   - **Add a delay of `2000` ms**: el gateway deja 10 logins o registros por minuto por IP, y el grupo 1 hace 7 seguidos;
   - **Stop run if an error occurs** destildado, para ver todas las pruebas.
5. **Start run.**

**Antes de cada corrida nueva**, cambiar el número de `emailCliente`, `emailCocina` y `claveConfirmacion` (`ana.prueba1@test.com` → `ana.prueba2@test.com`, y así). La colección crea ese cliente, esa cuenta de cocina y ese pedido; si ya existen, el registro da 409 y la confirmación devuelve el pedido anterior.

Las demás variables (tokens, ids de pedido, plato, ingrediente y cocina nueva) las completan solas las requests.

## Datos que usa

Usa los datos de ejemplo que cargan los servicios al arrancar: la cocina `nueva-cordoba`, el plato `p1` (Mega doble) y la Veggie burger `p3`, cuyo medallón de lentejas arranca con 3 unidades para poder probar "sin stock". Crea la cocina `alberdi` la primera vez; en las corridas siguientes sigue con la misma.
