import process from "node:process";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Solo para trabajar en la compu (npm run dev): el frontend le pide todo al API Gateway por /api
// y Vite reenvía esas solicitudes al gateway, así el navegador ve un solo origen y no hacen falta
// reglas de CORS. En Docker no se usa Vite: NGINX sirve el build y hace este mismo reenvío
// (nginx.conf). GATEWAY_URL cambia el destino; si falta, localhost:8080.
const gateway = process.env.GATEWAY_URL || "http://localhost:8080";

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": { target: gateway, changeOrigin: true },
    },
  },
});
