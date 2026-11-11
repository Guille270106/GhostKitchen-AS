const TOKEN_KEY = "ghostkitchen-token";
// El token no trae el nombre ni la sede del admin: se guardan aparte al iniciar sesión.
const PERFIL_KEY = "ghostkitchen-perfil";

function decodeBase64Url(value) {
  const base64 = value.replace(/-/g, "+").replace(/_/g, "/");
  const padded = base64.padEnd(base64.length + ((4 - (base64.length % 4)) % 4), "=");
  return decodeURIComponent(escape(window.atob(padded)));
}

export function getAuthToken() {
  return localStorage.getItem(TOKEN_KEY);
}

export function setAuthToken(token) {
  localStorage.setItem(TOKEN_KEY, token);
}

export function clearAuthToken() {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(PERFIL_KEY);
}

export function setSessionProfile(perfil) {
  localStorage.setItem(PERFIL_KEY, JSON.stringify(perfil));
}

function getSessionProfile() {
  try {
    return JSON.parse(localStorage.getItem(PERFIL_KEY) || "{}");
  } catch {
    return {};
  }
}

export function getCurrentUser() {
  const token = getAuthToken();
  if (!token) return null;

  try {
    const payload = JSON.parse(decodeBase64Url(token.split(".")[1] || ""));
    const expiresAt = payload.exp ? payload.exp * 1000 : null;

    if (expiresAt && expiresAt <= Date.now()) {
      clearAuthToken();
      return null;
    }

    // El JWT de Usuarios trae sub (id), rol, y según el rol la cocina o la localidad. Solo
    // se lee para mostrar la pantalla que corresponde: quien decide los permisos es el
    // backend, que verifica la firma en cada solicitud.
    const perfil = getSessionProfile();
    return {
      id: payload.sub,
      nombre: perfil.nombre || payload.email,
      email: payload.email,
      rol: String(payload.rol || "").toUpperCase(),
      localidadId: perfil.localidadId || null,
      localidad: perfil.localidad || null, // la del admin ("Nueva Córdoba")
      direccion: perfil.direccion || "", // la del cliente, cargada al registrarse
      cocinaId: payload.cocina || null,
      expiresAt,
      token,
    };
  } catch {
    clearAuthToken();
    return null;
  }
}

export function homeForRole(rol) {
  if (rol === "COCINA") return "/cocina";
  if (rol === "ADMIN") return "/admin/cocinas";
  return "/";
}
