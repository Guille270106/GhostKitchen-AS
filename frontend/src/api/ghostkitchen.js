// Capa de acceso a datos del frontend.
//
// Las pantallas solo conocen estas funciones: nunca llaman a fetch directo. Cada función le
// pide los datos al API Gateway (/api/v1) y devuelve la forma que esperan las pantallas.
// Así, si cambia una ruta o un formato del backend, se arregla acá y no en cada pantalla.
//
// Cada parte del sistema suma sus funciones en su commit (G14 la base y la sesión; G18 la
// búsqueda; V16/V17 el inventario; L17/L18 el carrito y los pedidos; G21 el admin).

import { clearAuthToken, getAuthToken, setAuthToken, setSessionProfile } from "../utils/auth";

// El frontend siempre habla con el gateway por la misma dirección: en desarrollo, Vite
// reenvía /api al gateway (vite.config.js), así no hay problemas de CORS.
const API_BASE = "/api/v1";
const CHANGE_EVENT = "ghostkitchen:db-change";
export const RESERVA_MS = 10 * 60 * 1000; // la reserva del carrito dura 10 minutos

export class ApiError extends Error {
  constructor(message, code = "ERROR", status = 0) {
    super(message);
    this.code = code;
    this.status = status || ({ UNAUTHORIZED: 401, FORBIDDEN: 403, NOT_FOUND: 404 })[code] || 400;
  }
}

// ---------------------------------------------------------------------------
// Cliente HTTP del gateway
// ---------------------------------------------------------------------------

// Mensajes para mostrar cuando el backend no manda uno propio.
const MENSAJES = {
  0: "No pudimos conectar con GhostKitchen. Revisá tu conexión y probá de nuevo.",
  401: "Necesitás iniciar sesión para realizar esta acción.",
  403: "No tenés permisos para realizar esta acción.",
  404: "No encontramos lo que buscabas.",
  429: "Hiciste muchos intentos seguidos. Esperá unos segundos y probá de nuevo.",
  503: "El servicio no está disponible en este momento. Probá de nuevo en unos segundos.",
};

// pedir hace una solicitud al gateway con el token de la sesión, si hay.
// Los errores llegan en formato Problem Details ({ code, detail, status }) y se convierten en
// ApiError, que es lo que muestran las pantallas.
export async function pedir(metodo, ruta, { cuerpo, headers = {} } = {}) {
  const token = getAuthToken();
  let respuesta;
  try {
    respuesta = await fetch(API_BASE + ruta, {
      method: metodo,
      headers: {
        Accept: "application/json",
        ...(cuerpo !== undefined ? { "Content-Type": "application/json" } : {}),
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
        ...headers,
      },
      body: cuerpo !== undefined ? JSON.stringify(cuerpo) : undefined,
    });
  } catch {
    throw new ApiError(MENSAJES[0], "NETWORK", 0);
  }

  const texto = await respuesta.text();
  let datos;
  try {
    datos = texto ? JSON.parse(texto) : null;
  } catch {
    datos = null; // por ejemplo, una página de error HTML de un proxy
  }

  if (!respuesta.ok) {
    // Un 401 con token quiere decir que la sesión venció: se cierra para que vuelva a entrar.
    if (respuesta.status === 401 && token) {
      clearAuthToken();
      avisarCambio();
    }
    const codigo = datos?.code || ({ 401: "UNAUTHORIZED", 403: "FORBIDDEN", 404: "NOT_FOUND" })[respuesta.status] || "ERROR";
    const mensaje = datos?.detail || MENSAJES[respuesta.status] || "Algo salió mal. Probá de nuevo.";
    throw new ApiError(mensaje, codigo, respuesta.status);
  }
  return datos;
}

// ---------------------------------------------------------------------------
// Avisos de cambios: cuando algo cambia (login, carrito, un pedido), las pantallas que usan
// useLiveQuery vuelven a pedir sus datos.
// ---------------------------------------------------------------------------

export function avisarCambio() {
  window.dispatchEvent(new Event(CHANGE_EVENT));
}

export function subscribe(listener) {
  window.addEventListener(CHANGE_EVENT, listener);
  return () => window.removeEventListener(CHANGE_EVENT, listener);
}

// ---------------------------------------------------------------------------
// Sesión (Usuarios)
// ---------------------------------------------------------------------------

// El rol llega en minúscula ("cocina"); las pantallas lo usan en mayúscula ("COCINA").
function usuarioParaPantallas(usuario, localidadId = null) {
  return {
    id: usuario.id,
    nombre: usuario.nombre,
    email: usuario.email,
    rol: String(usuario.rol || "").toUpperCase(),
    cocinaId: usuario.cocinaAsignadaId || null,
    localidadId,
  };
}

// El admin tiene una localidad ("Nueva Córdoba"), pero las pantallas trabajan con el id de
// la cocina de esa localidad ("nueva-cordoba"). Se busca una sola vez, al entrar.
async function localidadDelAdmin(usuario) {
  if (usuario.rol !== "admin") return null;
  try {
    const cocinas = await pedir("GET", "/cocinas");
    const lista = Array.isArray(cocinas) ? cocinas : cocinas?.items || [];
    const suya = lista.find((c) => c.localidad === usuario.localidad);
    return suya?.id || null;
  } catch {
    return null;
  }
}

export async function loginUser({ email, password }) {
  const sesion = await pedir("POST", "/auth/login", { cuerpo: { email, password } });
  const perfil = usuarioParaPantallas(sesion.usuario);
  setAuthToken(sesion.token);
  perfil.localidadId = await localidadDelAdmin(sesion.usuario);
  setSessionProfile(perfil);
  avisarCambio();
  return { usuario: perfil };
}

// HU01: nombre, correo, contraseña y dirección. Usuarios no devuelve un token al registrarse,
// así que después se inicia la sesión con los mismos datos.
export async function registerUser({ nombre, email, password, direccion }) {
  await pedir("POST", "/auth/registro", { cuerpo: { nombre, email, password, direccion } });
  return loginUser({ email, password });
}

// El cierre de sesión descarta el token (HU02): el servidor no guarda sesiones.
export function logoutUser() {
  clearAuthToken();
  avisarCambio();
}

// ---------------------------------------------------------------------------
// Catálogo
// ---------------------------------------------------------------------------

// Convierte una cocina de Catálogo a la forma que usan las pantallas. El id de la cocina es
// también el "localidadId" de la sede que elige el cliente.
export function cocinaParaPantallas(c) {
  return {
    id: c.id,
    nombre: c.nombre,
    localidadId: c.id,
    localidad: c.localidad,
    direccion: c.direccion,
    lat: c.ubicacion?.lat,
    lng: c.ubicacion?.lng,
    tiempo: c.tiempoEntregaMin ? `${c.tiempoEntregaMin}–${c.tiempoEntregaMax} min` : "",
    envio: c.costoEnvio,
    activa: c.activa !== false,
    marcas: (c.restaurantes || []).filter((r) => r.activo !== false).map((r) => ({ id: r.id, nombre: r.nombre })),
  };
}

// Cocinas activas con sus restaurantes activos. Con localidadId, solo la de esa sede.
export async function getCocinas({ localidadId } = {}) {
  const datos = await pedir("GET", "/cocinas");
  const lista = Array.isArray(datos) ? datos : datos?.items || [];
  return lista.map(cocinaParaPantallas).filter((c) => !localidadId || c.localidadId === localidadId);
}
