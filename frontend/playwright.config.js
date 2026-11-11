// Pruebas de la pantalla con Playwright (carpeta e2e/). Abren un navegador de verdad contra el
// sistema levantado con Docker: el frontend en 5173 y el gateway en 8080.
//
//   docker compose up -d --build
//   npx playwright install chromium     (la primera vez)
//   npm run test:e2e
//
// No levantan nada por su cuenta: si el sistema no está arriba, fallan al primer paso.
import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  // Cada recorrido depende del anterior (el pedido que hace el cliente lo acepta la cocina):
  // van en orden y de a uno.
  fullyParallel: false,
  workers: 1,
  timeout: 90_000,
  expect: { timeout: 10_000 },
  forbidOnly: Boolean(process.env.CI),
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : "list",
  use: {
    baseURL: process.env.FRONTEND_URL || "http://localhost:5173",
    locale: "es-AR",
    // Si algo falla queda una captura, un video y la traza paso a paso (npx playwright show-trace).
    screenshot: "only-on-failure",
    video: "retain-on-failure",
    trace: "retain-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
