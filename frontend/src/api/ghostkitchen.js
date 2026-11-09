// Capa de acceso a datos del frontend.
//
// Las pantallas solo conocen estas funciones: nunca llaman a fetch directo. Cada función le
// pide los datos al API Gateway (/api/v1) y devuelve la forma que esperan las pantallas.
// Así, si cambia una ruta o un formato del backend, se arregla acá y no en cada pantalla.
//
// Cada parte del sistema suma sus funciones en su commit (G14 la base y la sesión; G18 la
// búsqueda; V16/V17 el inventario; L17/L18 el carrito y los pedidos; G21 el admin).

import { dishPhotos } from "../data/dishPhotos";
import { FOTOS_COCINA, photos, zonas } from "../data/visual";
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
    foto: FOTOS_COCINA[c.id] || photos.local,
    marcas: (c.restaurantes || []).filter((r) => r.activo !== false).map((r) => ({ id: r.id, nombre: r.nombre })),
  };
}

// Cocinas activas con sus restaurantes activos. Con localidadId, solo la de esa sede.
export async function getCocinas({ localidadId } = {}) {
  const datos = await pedir("GET", "/cocinas");
  const lista = Array.isArray(datos) ? datos : datos?.items || [];
  return lista.map(cocinaParaPantallas).filter((c) => !localidadId || c.localidadId === localidadId);
}

// ---------------------------------------------------------------------------
// Detalle de una cocina, búsqueda de platos y sede más cercana (G18)
// ---------------------------------------------------------------------------

// Foto de un plato: la que cargó la cocina o, para los platos de ejemplo, la de dishPhotos.
function fotoDe(id, fotoUrl) {
  return fotoUrl || dishPhotos[id]?.foto || "";
}

// Convierte un plato de Búsqueda a la forma que usan las pantallas. "disponible" es
// orientativo (puede tener unos segundos de atraso, RN21): el control real es al reservar.
function platoDeBusqueda(p) {
  return {
    id: p.id,
    nombre: p.nombre,
    descripcion: p.descripcion || "",
    categoria: p.categoria,
    etiquetas: p.etiquetas || [],
    precio: p.precio,
    foto: fotoDe(p.id, p.fotoUrl),
    restauranteId: p.restaurante?.id,
    restauranteNombre: p.restaurante?.nombre,
    cocinaId: p.sede?.id,
    cocinaNombre: p.sede?.nombre,
    localidadId: p.sede?.id,
    sinStock: p.disponible === false,
  };
}

// Pide platos a Búsqueda (GET /api/v1/busqueda/sedes/{sede}/platos). Los parámetros son los
// del contrato de Búsqueda: q, categoria, etiqueta, restauranteId, precioMax, soloDisponibles,
// orden (relevancia, precio_asc, precio_desc), page y pageSize (hasta 50).
async function buscarEnSede(sedeId, params) {
  const qs = new URLSearchParams();
  Object.entries(params).forEach(([k, v]) => {
    if (v !== undefined && v !== null && v !== "") qs.set(k, String(v));
  });
  return pedir("GET", `/busqueda/sedes/${encodeURIComponent(sedeId)}/platos?${qs}`);
}

// Una cocina con sus marcas y su menú. El menú sale de Catálogo (la fuente de verdad); si
// Búsqueda contesta, se le suma qué platos tienen stock. Si Búsqueda no contesta, el menú se
// muestra igual: la disponibilidad se controla de verdad al agregar al carrito.
export async function getCocinaById(id) {
  const [cocina, menu, disponibles] = await Promise.all([
    pedir("GET", `/cocinas/${encodeURIComponent(id)}`),
    pedir("GET", `/cocinas/${encodeURIComponent(id)}/platos`),
    buscarEnSede(id, { soloDisponibles: false, pageSize: 50 }).catch(() => null),
  ]);
  const c = cocinaParaPantallas(cocina);
  const nombres = Object.fromEntries(c.marcas.map((m) => [m.id, m.nombre]));
  const stock = disponibles ? Object.fromEntries(disponibles.items.map((p) => [p.id, p.disponible])) : {};
  const platos = (menu.items || []).map((p) => ({
    id: p.id,
    nombre: p.nombre,
    descripcion: p.descripcion || "",
    categoria: p.categoria,
    etiquetas: p.etiquetas || [],
    precio: p.precio,
    foto: fotoDe(p.id, p.fotoUrl),
    restauranteId: p.restauranteId,
    restauranteNombre: nombres[p.restauranteId],
    cocinaId: c.id,
    cocinaNombre: c.nombre,
    localidadId: c.id,
    sinStock: stock[p.id] === false,
  }));
  return { cocina: c, marcas: c.marcas, platos };
}

// Algunos platos con stock de la sede, para el inicio.
export async function getPlatosDestacados({ localidadId, limit = 4 } = {}) {
  if (!localidadId) return [];
  const res = await buscarEnSede(localidadId, { pageSize: limit });
  return res.items.map(platoDeBusqueda);
}

// Sugerencias mientras se escribe: nombres de platos de la sede que empiezan con lo escrito.
export async function autocompletePlatos(q, { localidadId } = {}) {
  const texto = String(q || "").trim();
  if (texto.length < 3 || !localidadId) return [];
  const res = await buscarEnSede(localidadId, { q: texto, soloDisponibles: false, pageSize: 20 });
  const inicio = normalizar(texto);
  return [...new Set(res.items.map((p) => p.nombre).filter((n) => normalizar(n).startsWith(inicio)))].sort().slice(0, 5);
}

const ORDENES = { relevancia: "relevancia", "precio-asc": "precio_asc", "precio-desc": "precio_desc" };

// Búsqueda de platos con filtros (HU04). Hace dos consultas a Búsqueda: una con los filtros
// (los resultados) y otra solo con el texto, para armar las opciones de filtro (marcas,
// categorías y precio máximo) sobre todo lo que coincide con el texto.
export async function searchPlatos(filters = {}) {
  const { q = "", localidadId, restauranteIds = [], categoria, etiqueta, precioMax, soloDisponibles = false, orden = "relevancia", page = 1, pageSize = 12 } = filters;
  if (!localidadId) return { items: [], total: 0, page: 1, pages: 1, facetas: { cocinas: [], restaurantes: [], categorias: [], precioMaximo: 0 } };
  const texto = q.trim();
  // Búsqueda filtra por una sola marca: con varias elegidas se piden todas y se filtra acá.
  const unaMarca = restauranteIds.length === 1 ? restauranteIds[0] : undefined;
  const varias = restauranteIds.length > 1;
  const [res, todo] = await Promise.all([
    buscarEnSede(localidadId, {
      q: texto, categoria, etiqueta, precioMax, restauranteId: unaMarca,
      soloDisponibles, orden: ORDENES[orden] || "relevancia",
      page: varias ? 1 : page, pageSize: varias ? 50 : pageSize,
    }),
    buscarEnSede(localidadId, { q: texto, soloDisponibles: false, pageSize: 50 }),
  ]);

  let items = res.items.map(platoDeBusqueda);
  let total = res.total;
  let paginaActual = res.page || page;
  let paginas = res.totalPages || Math.max(1, Math.ceil(total / pageSize));
  if (varias) {
    const todos = items.filter((p) => restauranteIds.includes(p.restauranteId));
    total = todos.length;
    paginas = Math.max(1, Math.ceil(total / pageSize));
    paginaActual = Math.min(Math.max(1, page), paginas);
    items = todos.slice((paginaActual - 1) * pageSize, paginaActual * pageSize);
  }

  const base = todo.items.map(platoDeBusqueda);
  const facetas = {
    cocinas: [...new Map(base.map((p) => [p.cocinaId, { id: p.cocinaId, nombre: p.cocinaNombre }])).values()],
    restaurantes: [...new Map(base.map((p) => [p.restauranteId, { id: p.restauranteId, nombre: p.restauranteNombre, cantidad: base.filter((x) => x.restauranteId === p.restauranteId).length }])).values()],
    categorias: [...new Set(base.map((p) => p.categoria))],
    precioMaximo: Math.max(0, ...base.map((p) => p.precio)),
  };
  return { items, total, page: paginaActual, pages: paginas, facetas, toleranciaAplicada: false };
}

const normalizar = (texto) => String(texto || "").normalize("NFD").replace(/[\u0300-\u036f]/g, "").toLowerCase().trim();

// Distancia en km entre dos puntos (fórmula de haversine), la misma que usa Búsqueda.
function distanciaKm(a, b) {
  const rad = (g) => (g * Math.PI) / 180;
  const dLat = rad(b.lat - a.lat);
  const dLng = rad(b.lng - a.lng);
  const h = Math.sin(dLat / 2) ** 2 + Math.cos(rad(a.lat)) * Math.cos(rad(b.lat)) * Math.sin(dLng / 2) ** 2;
  return 6371 * 2 * Math.asin(Math.sqrt(Math.min(1, Math.max(0, h))));
}

// Zonas que coinciden con lo escrito, cada una con su cocina activa más cercana (HU03). Las
// zonas son datos fijos de la pantalla; las cocinas y su ubicación vienen de Catálogo.
export async function buscarZonas(q, limit = 6) {
  const termino = normalizar(q);
  if (termino.length < 2) return [];
  const cocinas = (await getCocinas()).filter((c) => c.activa && Number.isFinite(c.lat) && Number.isFinite(c.lng));
  return zonas
    .map((z) => ({ zona: z, nombre: normalizar(z.nombre) }))
    .filter(({ nombre }) => nombre.includes(termino))
    .sort((a, b) => Number(b.nombre.startsWith(termino)) - Number(a.nombre.startsWith(termino)) || a.nombre.localeCompare(b.nombre))
    .slice(0, limit)
    .map(({ zona }) => {
      const [cercana] = cocinas
        .map((c) => ({ ...c, distanciaKm: Math.round(distanciaKm(zona, c) * 10) / 10 }))
        .sort((a, b) => a.distanciaKm - b.distanciaKm);
      return { ...zona, cocina: cercana ? { id: cercana.id, nombre: cercana.nombre, localidadId: cercana.localidadId, distanciaKm: cercana.distanciaKm } : null };
    });
}
