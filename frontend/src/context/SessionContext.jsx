import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";

import { loginUser, logoutUser, registerUser, subscribe } from "../api/ghostkitchen";
import { getCurrentUser } from "../utils/auth";

const SessionContext = createContext(null);
const SEDE_KEY = "ghostkitchen-sede";
// Mientras no se elige sede, la pantalla principal de fondo muestra esta (borrosa, detras del selector).
const SEDE_DE_FONDO = "nueva-cordoba";

export function SessionProvider({ children }) {
  const [user, setUser] = useState(() => getCurrentUser());
  // Sede elegida (la localidad de una de las cocinas).
  const [sedeElegida, setSedeElegida] = useState(() => localStorage.getItem(SEDE_KEY) || "");
  const [selectorManual, setSelectorManual] = useState(false);

  // Cuando cambia la sesión (login, logout o un 401 por token vencido) se vuelve a leer el token.
  useEffect(() => subscribe(() => setUser(getCurrentUser())), []);

  const login = useCallback(async (credentials) => {
    const result = await loginUser(credentials);
    setUser(getCurrentUser());
    return result;
  }, []);

  const register = useCallback(async (form) => {
    const result = await registerUser(form);
    setUser(getCurrentUser());
    return result;
  }, []);

  const logout = useCallback(() => {
    logoutUser();
    setUser(null);
  }, []);

  const elegirSede = useCallback((localidadId) => {
    localStorage.setItem(SEDE_KEY, localidadId);
    setSedeElegida(localidadId);
    setSelectorManual(false);
  }, []);

  const abrirSelectorSede = useCallback(() => setSelectorManual(true), []);
  const cerrarSelectorSede = useCallback(() => setSelectorManual(false), []);

  // Se muestra solo la sede elegida. El cliente no tiene una sede fija: la elige (y la cambia) cuando quiere.
  const sedeActual = sedeElegida;
  const localidadId = sedeActual || SEDE_DE_FONDO;
  // El selector se abre solo hasta que haya una sede, o cuando el usuario pide cambiarla.
  const selectorSedeAbierto = selectorManual || !sedeActual;

  const value = useMemo(
    () => ({ user, login, register, logout, localidadId, tieneSede: Boolean(sedeActual), elegirSede, selectorSedeAbierto, abrirSelectorSede, cerrarSelectorSede }),
    [user, login, register, logout, localidadId, sedeActual, elegirSede, selectorSedeAbierto, abrirSelectorSede, cerrarSelectorSede]
  );

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

// eslint-disable-next-line react-refresh/only-export-components
export function useSession() {
  return useContext(SessionContext);
}
