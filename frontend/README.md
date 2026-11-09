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
├── context/              sesión, avisos (toast) y, desde L17, el carrito
├── hooks/useLiveQuery.js vuelve a pedir los datos cuando algo cambia
├── styles/<tema>.css     estilos, con las variables de variables.css
└── utils/                formato de precios y fechas, lectura del token
```

**Las pantallas nunca llaman a `fetch`.** Todo pasa por `api/ghostkitchen.js`, que le pide los datos al gateway y los devuelve con la forma que usan las pantallas. Los errores del backend llegan en Problem Details y se convierten en `ApiError` (`message`, `code` y `status`), que es lo que muestran las pantallas. Si el token venció (401), la sesión se cierra sola.

**El token** se guarda en `localStorage` y viaja en `Authorization: Bearer` en cada solicitud. El frontend lo lee solo para saber qué pantalla mostrar según el rol; los permisos los controla el backend, que verifica la firma.

**Sin CORS:** el navegador siempre le habla al mismo origen. En desarrollo, Vite reenvía `/api` al gateway (`vite.config.js`, variable `GATEWAY_URL`).

## Cómo correrlo

Con todo el sistema:

```bash
docker compose up -d --build frontend
```

Solo el frontend, con el resto del sistema en Docker:

```bash
cd frontend
npm ci
npm run dev      # http://localhost:5173, reenvía /api a http://localhost:8080
npm run lint
```

## De dónde sale cada dato (G18)

| Pantalla | Función de `ghostkitchen.js` | Backend |
| --- | --- | --- |
| Elegir la sede, encabezado | `getCocinas` | Catálogo `GET /api/v1/cocinas` |
| Sugerir la sede por zona | `buscarZonas` | Las zonas son datos fijos (`data/visual.js`); la cocina más cercana se calcula con la ubicación que da Catálogo |
| Inicio: platos destacados | `getPlatosDestacados` | Búsqueda `GET /api/v1/busqueda/sedes/{sede}/platos` |
| Buscar platos y filtros | `searchPlatos`, `autocompletePlatos` | Búsqueda (dos consultas: resultados con filtros, y opciones de filtro con solo el texto) |
| Detalle de una cocina | `getCocinaById` | Catálogo (cocina y menú) + Búsqueda (qué platos tienen stock). Si Búsqueda no contesta, el menú se muestra igual |
| Login y registro | `loginUser`, `registerUser` | Usuarios. El registro pide la dirección (HU01) |

Las fotos de los platos de ejemplo están en `data/dishPhotos.js` y `public/images/platos`; las de portada y las categorías del inicio, en `data/visual.js`. El botón "Agregar" ya está en cada plato, pero el carrito real llega con L17: hasta entonces `context/CartContext.jsx` es una versión provisoria que avisa que todavía no está disponible.

## Qué entra en cada parte

El frontend se suma por partes, cada una en el commit de quien la hace:

| Commit | Pantallas |
| --- | --- |
| G14 | Base: proyecto, contenedor, encabezado, pie, paneles, cliente del gateway y sesión |
| G18 | Login, registro, elegir la sede, inicio, buscar platos y detalle de la cocina |
| V16 · V17 | Inventario · menú, platos y recetas de la cocina |
| L17 · L18 | Carrito · mis pedidos y tablero de la cocina |
| G21 | Panel del admin: cocinas, marcas y métricas |
