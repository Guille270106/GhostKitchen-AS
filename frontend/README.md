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

## Qué entra en cada parte

El frontend se suma por partes, cada una en el commit de quien la hace:

| Commit | Pantallas |
| --- | --- |
| G14 | Base: proyecto, contenedor, encabezado, pie, paneles, cliente del gateway y sesión |
| G18 | Login, registro, elegir la sede, inicio, buscar platos y detalle de la cocina |
| V16 · V17 | Inventario · menú, platos y recetas de la cocina |
| L17 · L18 | Carrito · mis pedidos y tablero de la cocina |
| G21 | Panel del admin: cocinas, marcas y métricas |
