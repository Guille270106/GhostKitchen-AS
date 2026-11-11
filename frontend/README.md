# Frontend

| | |
| --- | --- |
| **Tecnología** | React 19 + Vite + React Router |
| **Puerto** | 5173 (`http://localhost:5173`) |
| **Habla con** | Solo con el API Gateway, por `/api/v1` |

## Cómo está armado

```
src/
├── api/ghostkitchen.js   única capa de datos: las pantallas solo llaman a sus funciones
├── pages/                una pantalla por ruta
├── components/<tema>/    piezas de cada pantalla (common y layout las usan todas)
├── context/              sesión, avisos (toast) y el carrito
├── hooks/useLiveQuery.js vuelve a pedir los datos cuando algo cambia
├── styles/<tema>.css     estilos, con las variables de variables.css
└── utils/                formato de precios y fechas, lectura del token
```

**Las pantallas nunca llaman a `fetch`.** Todo pasa por `api/ghostkitchen.js`, que le pide los datos al gateway y los devuelve con la forma que usan las pantallas. Los errores del backend llegan en Problem Details y se convierten en `ApiError` (`message`, `code` y `status`), que es lo que muestran las pantallas. Si el token venció (401), la sesión se cierra sola.

**El token** se guarda en `localStorage` y viaja en `Authorization: Bearer` en cada solicitud. El frontend lo lee solo para saber qué pantalla mostrar según el rol; los permisos los controla el backend, que verifica la firma.

**Sin CORS:** el navegador siempre le habla al mismo origen. En desarrollo, Vite reenvía `/api` al gateway (`vite.config.js`, variable `GATEWAY_URL`).

## Cómo correrlo

**En Docker (versión de producción).** La imagen compila con `npm run build` y **NGINX** sirve los archivos de `dist/` (`Dockerfile` en dos etapas y `nginx.conf`). NGINX le pasa `/api` al gateway, guarda en caché los archivos de `/assets/` (llevan un hash en el nombre) y no guarda `index.html`. Si se recarga una página interna (por ejemplo `/cocina/inventario`), devuelve `index.html` y la ruta la resuelve React.

```bash
docker compose build frontend
docker compose up -d frontend
```

**Para trabajar en la compu (desarrollo), sin Docker para el frontend.** Con el resto del sistema en Docker, el servidor de Vite recarga los cambios al instante. Es el mismo de siempre:

```bash
cd frontend
npm ci
npm run dev      # http://localhost:5173, reenvía /api a http://localhost:8080
npm run lint
```

## Prueba de la pantalla (Playwright)

`e2e/pedido.spec.js` abre un navegador de verdad y recorre el flujo principal contra el sistema levantado, sin respuestas simuladas:

1. Un cliente nuevo se registra, elige Cocina Cerro y busca un plato.
2. Lo agrega al carrito y confirma el pedido con **otra** dirección de entrega.
3. Lo ve en *Mis pedidos* (Recibido, con esa dirección).
4. La cocina, en otro navegador, lo ve en su tablero con la dirección y lo acepta.
5. El cliente ve que la cocina lo aceptó y que ya no lo puede cancelar.

Antes, `e2e/preparar.js` crea por la API una cuenta Cocina y un plato propio con stock (como `services/pedidos/testdata/e2e.mjs`), así no depende del stock de los datos de ejemplo y se puede repetir. Los botones y campos se buscan por lo que ve el usuario (su texto o su rótulo) y no por clases de CSS: si cambia el diseño, la prueba sigue andando; si cambia un texto de esos, hay que actualizarla.

Corre en GitHub Actions en cada pull request, en el mismo job que la prueba de la API (`.github/workflows/e2e.yml`). Si falla, el job guarda el artefacto `playwright` con la captura, el video y la traza paso a paso. Para correrla en la compu, con el sistema levantado (`docker compose up -d --build`) y desde `frontend/`:

```bash
npm ci
npx playwright install chromium   # la primera vez
npm run test:e2e                  # usa ADMIN_PASSWORD del .env, sin imprimirla
npx playwright show-trace test-results/<carpeta>/trace.zip   # si falló: qué vio el navegador
```

El gateway deja 10 logins o registros por minuto por IP: si se corre varias veces seguidas, esperar un minuto entre corridas.

## De dónde sale cada dato

| Pantalla | Función de `ghostkitchen.js` | Backend |
| --- | --- | --- |
| Elegir la sede, encabezado | `getCocinas` | Catálogo `GET /api/v1/cocinas` |
| Sugerir la sede por zona | `buscarZonas` | Los barrios son una lista fija (`data/visual.js`) que solo da la coordenada; la cocina más cercana la calcula Búsqueda (`GET /api/v1/busqueda/sedes/cercana`), con la cobertura de 25 km. Sin cobertura o con Búsqueda caída, el barrio queda sin cocina |
| Inicio: platos destacados | `getPlatosDestacados` | Búsqueda `GET /api/v1/busqueda/sedes/{sede}/platos` |
| Buscar platos y filtros | `searchPlatos`, `autocompletePlatos` | Búsqueda (dos consultas: resultados con filtros, y opciones de filtro con solo el texto) |
| Detalle de una cocina | `getCocinaById` | Catálogo (cocina y menú) + Búsqueda (qué platos tienen stock). Si Búsqueda no contesta, el menú se muestra igual |
| Login y registro | `loginUser`, `registerUser` | Usuarios. El registro pide la dirección |

Las fotos de los platos de ejemplo están en `data/dishPhotos.js` y `public/images/platos`; las de portada y las categorías del inicio, en `data/visual.js`. El botón "Agregar" de cada plato usa el carrito de `context/CartContext.jsx`, que le pide todo a Pedidos.

## Panel del admin

| Pantalla | Función de `ghostkitchen.js` | Backend |
| --- | --- | --- |
| Cocinas de la localidad, con sus restaurantes | `getAdminCocinas` | Catálogo `GET /api/v1/admin/cocinas` (solo las de su localidad) |
| Agregar cocina | `crearCocina` | Catálogo `POST /api/v1/admin/cocinas` y Usuarios `POST /api/v1/admin/cuentas-cocina`. La ubicación sale de la zona elegida (`data/visual.js`). Si la cuenta falla, la cocina ya quedó creada y se avisa |
| Pausar o reactivar una cocina o un restaurante | `toggleCocina`, `toggleRestaurante` | Catálogo `PATCH` (lo pausado deja de mostrarse) |
| Nuevo restaurante (hasta 4) | `crearRestaurante` | Catálogo `POST .../restaurantes` (hasta 4, sin repetir el nombre) |
| Métricas de los últimos 7 días | `getMetricas` | Pedidos `GET /api/v1/admin/metricas?cocinaId=` |

Las rutas `/admin/...` van dentro de `ProtectedRoute` (solo rol `ADMIN`) y `WorkspaceLayout`. La cocina elegida en las métricas se recuerda en el navegador. Quien decide los permisos es el backend: la pantalla solo evita mostrar lo que no corresponde.

## Pantallas

| Ruta | Quién la ve | Pantalla |
| --- | --- | --- |
| `/` | Todos | Inicio: elegir la sede y platos destacados |
| `/buscar` | Todos | Buscar platos con filtros |
| `/cocinas/:id` | Todos | Menú de una cocina |
| `/login` · `/registro` | Todos | Iniciar sesión y crear una cuenta de cliente |
| `/carrito` | Cliente | Carrito con el tiempo que le queda a la reserva, y confirmar el pedido |
| `/pedidos` | Cliente | Mis pedidos: seguir el estado, cancelar y confirmar que llegó |
| `/cocina` | Cocina | Pedidos de la cocina en columnas: aceptar, rechazar y avanzar |
| `/cocina/inventario` | Cocina | Stock de ingredientes, reposiciones y mermas |
| `/cocina/platos` | Cocina | Platos y recetas |
| `/admin/cocinas` | Admin | Cocinas y restaurantes de su localidad |
| `/admin/metricas` | Admin | Métricas del negocio |
