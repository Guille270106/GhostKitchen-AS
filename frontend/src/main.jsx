import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import App from "./App.jsx";
import { activarValidacion } from "./utils/validacion";

import "./styles/global.css";

// Mensajes de validación propios en todos los formularios, en lugar de los del navegador.
activarValidacion();

createRoot(document.getElementById("root")).render(
  <StrictMode>
    <App />
  </StrictMode>
);
