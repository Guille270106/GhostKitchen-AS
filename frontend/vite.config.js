import process from "node:process";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// El frontend le pide todo al API Gateway por /api. Vite reenvía esas solicitudes al gateway,
// así el navegador ve un solo origen y no hacen falta reglas de CORS.
// GATEWAY_URL: en Docker es http://gateway:8080; corriendo Vite en la compu, localhost:8080.
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
