# Frontend en dos etapas: primero se compila con Vite y después NGINX sirve solo los archivos
# compilados (dist/). Así Docker publica la aplicación como se publica de verdad: minificada, sin
# el servidor de desarrollo ni sus mensajes de depuración. Para trabajar en la compu sigue
# estando `npm run dev` (ver el README del frontend).

# ---------- 1. Compilar ----------
FROM node:22-alpine AS build
WORKDIR /app

# Primero las dependencias: esta capa se reutiliza mientras package.json no cambie.
COPY package*.json ./
RUN npm ci

# Después el código, y el build de producción en dist/.
COPY . .
RUN npm run build

# ---------- 2. Servir ----------
FROM nginx:1.27-alpine
# Nuestra configuración en lugar de la que trae la imagen.
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html

# El mismo puerto de siempre (5173), así no cambian el compose, la documentación ni las pruebas.
EXPOSE 5173
