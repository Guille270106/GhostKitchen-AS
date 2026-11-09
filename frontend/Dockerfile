# Imagen base liviana de Node
FROM node:22-alpine

# Carpeta de trabajo dentro del contenedor
WORKDIR /app

# Copiamos primero las dependencias (se cachea si package.json no cambia)
COPY package*.json ./
RUN npm ci

# Copiamos el resto del codigo
COPY . .

# Puerto del servidor de desarrollo de Vite
EXPOSE 5173

# --host hace que Vite escuche en 0.0.0.0 para ser accesible desde fuera del contenedor
CMD ["npm", "run", "dev", "--", "--host"]
