import { useEffect, useState } from "react";

import { getAdminCocinas } from "../api/ghostkitchen";
import useLiveQuery from "./useLiveQuery";

const KEY = "ghostkitchen-admin-cocina";

// La cocina elegida se recuerda en el navegador; si no se puede (modo privado), se elige la
// primera cada vez.
function leer() {
  try {
    return localStorage.getItem(KEY) || "";
  } catch {
    return "";
  }
}

function guardar(id) {
  try {
    localStorage.setItem(KEY, id);
  } catch {
    // sin almacenamiento: no pasa nada
  }
}

// Cocinas de la localidad del admin y cuál está seleccionada (se recuerda entre pantallas).
export default function useAdminCocina() {
  const { data, loading, error } = useLiveQuery(getAdminCocinas, []);
  const [cocinaId, setCocinaIdState] = useState(() => leer());

  const cocinas = data?.cocinas || [];
  const valida = cocinas.some((c) => c.id === cocinaId);
  const selectedId = valida ? cocinaId : cocinas[0]?.id || "";

  useEffect(() => {
    if (selectedId) guardar(selectedId);
  }, [selectedId]);

  return {
    loading,
    error,
    localidad: data?.localidad,
    cocinas,
    cocinaId: selectedId,
    cocina: cocinas.find((c) => c.id === selectedId),
    setCocinaId: setCocinaIdState,
  };
}
