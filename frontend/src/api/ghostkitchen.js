// Capa de acceso a datos del frontend.
//
// Las pantallas solo conocen estas funciones: nunca llaman a fetch directo. Cada función le
// pide los datos al API Gateway (/api/v1) y devuelve la forma que esperan las pantallas.
// Así, si cambia una ruta o un formato del backend, se arregla acá y no en cada pantalla.
//
// Cada parte del sistema suma sus funciones en su commit (G14 la base y la sesión; G18 la
// búsqueda; V16/V17 el inventario; L17/L18 el carrito y los pedidos; G21 el admin).

import { dishPhotos } from "../data/dishPhotos";
import { ETIQUETAS, FOTOS_COCINA, categorias, photos, zonas } from "../data/visual";
import { clearAuthToken, getAuthToken, getCurrentUser, setAuthToken, setSessionProfile } from "../utils/auth";
import { estadoLabel } from "../utils/formatters";

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
    localidad: usuario.localidad || null,
    localidadId,
    // La del registro: el carrito la propone como dirección de entrega.
    direccion: usuario.direccion || "",
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

// Búsqueda devuelve como mucho 50 platos por página. Cuando hace falta TODO lo que coincide (el
// stock de un menú entero, varias marcas, las opciones de los filtros), se recorren todas las
// páginas: el tamaño de página no puede convertirse en un límite de resultados.
const POR_PAGINA = 50;
async function buscarTodo(sedeId, params) {
  const primera = await buscarEnSede(sedeId, { ...params, page: 1, pageSize: POR_PAGINA });
  const paginas = primera.totalPages || Math.ceil((primera.total || 0) / POR_PAGINA) || 1;
  const resto = await Promise.all(
    Array.from({ length: paginas - 1 }, (_, i) => buscarEnSede(sedeId, { ...params, page: i + 2, pageSize: POR_PAGINA })),
  );
  return [primera, ...resto].flatMap((r) => r.items || []);
}

// Una cocina con sus marcas y su menú. El menú sale de Catálogo (la fuente de verdad); si
// Búsqueda contesta, se le suma qué platos tienen stock. Si Búsqueda no contesta, el menú se
// muestra igual: la disponibilidad se controla de verdad al agregar al carrito.
export async function getCocinaById(id) {
  const [cocina, menu, disponibles] = await Promise.all([
    pedir("GET", `/cocinas/${encodeURIComponent(id)}`),
    pedir("GET", `/cocinas/${encodeURIComponent(id)}/platos`),
    // Si Búsqueda no contesta, no se inventa el stock: los platos quedan "sin confirmar".
    buscarTodo(id, { soloDisponibles: false }).catch(() => null),
  ]);
  const c = cocinaParaPantallas(cocina);
  const nombres = Object.fromEntries(c.marcas.map((m) => [m.id, m.nombre]));
  const stock = disponibles ? Object.fromEntries(disponibles.map((p) => [p.id, p.disponible])) : {};
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
    // No sabemos si hay stock (Búsqueda no contestó o todavía no indexó el plato): se puede
    // intentar agregar, e Inventario decide de verdad al reservar.
    stockSinConfirmar: stock[p.id] === undefined,
  }));
  return { cocina: c, marcas: c.marcas, platos, stockSinConfirmar: disponibles === null };
}

// Algunos platos con stock de la sede, para el inicio.
export async function getPlatosDestacados({ localidadId, limit = 4 } = {}) {
  if (!localidadId) return [];
  const res = await buscarEnSede(localidadId, { pageSize: limit });
  return res.items.map(platoDeBusqueda);
}

// Sugerencias mientras se escribe (como las de Google). Cada palabra escrita tiene que ser el
// principio de alguna palabra: "doble ham" sugiere "Hamburguesa doble". Devuelve:
// - categorias: categorías y etiquetas de la lista fija que coinciden (no consultan a nadie);
// - platos: platos de la sede cuyo nombre, marca o categoría coincide, sin repetir nombre y marca.
export const MINIMO_SUGERENCIAS = 2;
const palabras = (texto) => normalizar(texto).split(/[^a-z0-9ñ]+/).filter(Boolean);
const coincide = (tokens, texto) => {
  const suyas = palabras(texto);
  return tokens.every((t) => suyas.some((p) => p.startsWith(t)));
};

export async function autocompletePlatos(q, { localidadId, limite = 6 } = {}) {
  const texto = String(q || "").trim();
  const tokens = palabras(texto);
  if (texto.length < MINIMO_SUGERENCIAS || !tokens.length) return { categorias: [], platos: [] };

  const etiquetasSueltas = ETIQUETAS.filter((e) => !categorias.some((c) => c.id === e)).map((e) => ({ id: e, label: e, esEtiqueta: true }));
  const cats = [...categorias, ...etiquetasSueltas].filter((c) => coincide(tokens, `${c.label} ${c.id}`)).slice(0, 2);

  if (!localidadId) return { categorias: cats, platos: [] };
  const res = await buscarEnSede(localidadId, { q: texto, soloDisponibles: false, pageSize: 20 });
  const vistos = new Set();
  const platos = (res.items || [])
    .map(platoDeBusqueda)
    .filter((p) => coincide(tokens, `${p.nombre} ${p.restauranteNombre || ""} ${p.categoria || ""}`))
    .filter((p) => {
      const clave = `${normalizar(p.nombre)}|${p.restauranteId}`;
      if (vistos.has(clave)) return false;
      vistos.add(clave);
      return true;
    })
    // Primero los que coinciden por el nombre del plato, y entre ellos los que empiezan con lo escrito.
    .map((p) => ({ p, puntaje: (coincide(tokens, p.nombre) ? 2 : 0) + (normalizar(p.nombre).startsWith(tokens[0]) ? 1 : 0) }))
    .sort((a, b) => b.puntaje - a.puntaje)
    .slice(0, limite)
    .map(({ p }) => ({ id: p.id, nombre: p.nombre, restauranteNombre: p.restauranteNombre, foto: p.foto, sinStock: p.sinStock }));
  return { categorias: cats, platos };
}

const ORDENES = { relevancia: "relevancia", "precio-asc": "precio_asc", "precio-desc": "precio_desc" };

// Búsqueda de platos con filtros (HU04). Hace dos consultas a Búsqueda: una con los filtros
// (los resultados) y otra solo con el texto, para armar las opciones de filtro (marcas,
// categorías y precio máximo) sobre todo lo que coincide con el texto.
export async function searchPlatos(filters = {}) {
  const { q = "", localidadId, restauranteIds = [], categoria, etiqueta, precioMax, soloDisponibles = false, orden = "relevancia", page = 1, pageSize = 12 } = filters;
  if (!localidadId) return { items: [], total: 0, page: 1, pages: 1, facetas: { cocinas: [], restaurantes: [], categorias: [], precioMaximo: 0 } };
  const texto = q.trim();
  // Búsqueda filtra por una sola marca: con varias elegidas se traen TODAS las páginas con los
  // demás filtros y se filtra acá. Las opciones de los filtros (marcas, categorías, precio)
  // también salen de todos los platos que coinciden con el texto, no de la primera página.
  const unaMarca = restauranteIds.length === 1 ? restauranteIds[0] : undefined;
  const varias = restauranteIds.length > 1;
  const filtrosBusqueda = { q: texto, categoria, etiqueta, precioMax, soloDisponibles, orden: ORDENES[orden] || "relevancia" };
  const [res, base] = await Promise.all([
    varias
      ? buscarTodo(localidadId, filtrosBusqueda)
      : buscarEnSede(localidadId, { ...filtrosBusqueda, restauranteId: unaMarca, page, pageSize }),
    buscarTodo(localidadId, { q: texto, soloDisponibles: false }),
  ]);

  let items, total, paginaActual, paginas;
  if (varias) {
    const todos = res.map(platoDeBusqueda).filter((p) => restauranteIds.includes(p.restauranteId));
    total = todos.length;
    paginas = Math.max(1, Math.ceil(total / pageSize));
    paginaActual = Math.min(Math.max(1, page), paginas);
    items = todos.slice((paginaActual - 1) * pageSize, paginaActual * pageSize);
  } else {
    items = res.items.map(platoDeBusqueda);
    total = res.total;
    paginaActual = res.page || page;
    paginas = res.totalPages || Math.max(1, Math.ceil(total / pageSize));
  }

  const platos = base.map(platoDeBusqueda);
  const facetas = {
    cocinas: [...new Map(platos.map((p) => [p.cocinaId, { id: p.cocinaId, nombre: p.cocinaNombre }])).values()],
    restaurantes: [...new Map(platos.map((p) => [p.restauranteId, { id: p.restauranteId, nombre: p.restauranteNombre, cantidad: platos.filter((x) => x.restauranteId === p.restauranteId).length }])).values()],
    categorias: [...new Set(platos.map((p) => p.categoria))],
    precioMaximo: Math.max(0, ...platos.map((p) => p.precio)),
  };
  // Búsqueda no corrige errores de tipeo: "sorentinos" no encuentra "sorrentinos".
  return { items, total, page: paginaActual, pages: paginas, facetas, toleranciaAplicada: false };
}

const normalizar = (texto) => String(texto || "").normalize("NFD").replace(/[\u0300-\u036f]/g, "").toLowerCase().trim();

// Cocina más cercana a un punto: la decide Búsqueda (GET /v1/sedes/cercana), que aplica la
// cobertura de 25 km. Si no hay ninguna cocina a 25 km o menos responde 404 SIN_COBERTURA, y si
// Búsqueda no contesta, error: en los dos casos no hay cocina para mostrar (null). La pantalla
// dice "Todavía no tenemos cocinas activas cerca"; nunca se inventa una cocina.
async function cocinaCercana(lat, lng) {
  try {
    const r = await pedir("GET", `/busqueda/sedes/cercana?lat=${lat}&lng=${lng}`);
    if (!r?.sede) return null;
    return { id: r.sede.id, nombre: r.sede.nombre, localidadId: r.sede.id, distanciaKm: Math.round(r.distanciaKm * 10) / 10 };
  } catch {
    return null;
  }
}

// Zonas que coinciden con lo escrito, cada una con su cocina más cercana (HU03). La lista de
// barrios es fija y solo sirve para pasar del nombre a una coordenada; qué cocina queda más cerca
// lo calcula Búsqueda, una consulta por barrio sugerido (6 como mucho, en paralelo).
export async function buscarZonas(q, limit = 6) {
  const termino = normalizar(q);
  if (termino.length < 2) return [];
  const sugeridas = zonas
    .map((z) => ({ zona: z, nombre: normalizar(z.nombre) }))
    .filter(({ nombre }) => nombre.includes(termino))
    .sort((a, b) => Number(b.nombre.startsWith(termino)) - Number(a.nombre.startsWith(termino)) || a.nombre.localeCompare(b.nombre))
    .slice(0, limit)
    .map(({ zona }) => zona);
  const cocinas = await Promise.all(sugeridas.map((z) => cocinaCercana(z.lat, z.lng)));
  return sugeridas.map((zona, i) => ({ ...zona, cocina: cocinas[i] }));
}

// ---------------------------------------------------------------------------
// Carrito (Pedidos, L17)
// ---------------------------------------------------------------------------

// Colores de las marcas, en el orden en que la cocina las tiene (los mismos de los estilos).
const COLORES_MARCA = ["", "dark", "green", "amber"];

// Para mostrar el carrito hace falta la cocina (nombre, envío y sus marcas) y las fotos de los
// platos. Se piden una vez por cocina a Catálogo y se guardan mientras dura la página.
const cocinasVistas = new Map();
function datosDeCocina(cocinaId) {
  if (!cocinasVistas.has(cocinaId)) {
    const datos = Promise.all([pedir("GET", `/cocinas/${cocinaId}`), pedir("GET", `/cocinas/${cocinaId}/platos`)])
      .then(([cocina, menu]) => ({
        cocina: cocinaParaPantallas(cocina),
        fotos: new Map((menu?.items || []).map((p) => [p.id, fotoDe(p.id, p.fotoUrl)])),
      }))
      .catch((error) => {
        cocinasVistas.delete(cocinaId); // si falló, se vuelve a pedir la próxima vez
        throw error;
      });
    cocinasVistas.set(cocinaId, datos);
  }
  return cocinasVistas.get(cocinaId);
}

// Agrupa platos por restaurante (el carrito y el pedido muestran una "bolsa" por marca).
function porRestaurante(items, marcas, restauranteDe) {
  const ids = [...new Set(items.map(restauranteDe))];
  return ids.map((id) => {
    const posicion = marcas.findIndex((m) => m.id === id);
    return {
      restaurante: { id, nombre: marcas[posicion]?.nombre || id, color: COLORES_MARCA[Math.max(posicion, 0) % COLORES_MARCA.length] },
      items: items.filter((i) => restauranteDe(i) === id),
    };
  });
}

// Convierte el carrito de Pedidos a la forma que usan las pantallas:
//   - "ACTIVO" mientras la reserva sigue vigente (venceEn en milisegundos, para la cuenta regresiva);
//   - "VENCIDO" si pasaron los 10 minutos (Pedidos responde vencido: true) o si Inventario avisó
//     que venció y Pedidos lo vació (vencioEn, L14): ahí ya no tiene platos ni cocina.
async function carritoParaPantallas(c) {
  if (!c?.items?.length) {
    if (!c?.vencioEn) return null;
    return { estado: "VENCIDO", cocina: null, grupos: [], cantidad: 0, subtotal: 0, envio: 0, total: 0 };
  }
  const { cocina, fotos } = await datosDeCocina(c.cocinaId);
  const items = c.items.map((it) => ({
    platoId: it.platoId,
    cantidad: it.cantidad,
    subtotal: it.subtotal,
    plato: { id: it.platoId, nombre: it.nombre, precio: it.precioUnitario, foto: fotos.get(it.platoId), restauranteId: it.restauranteId },
  }));
  const envio = cocina.envio || 0;
  return {
    estado: c.vencido ? "VENCIDO" : "ACTIVO",
    venceEn: Date.parse(c.venceEn),
    cocina,
    grupos: porRestaurante(items, cocina.marcas, (i) => i.plato.restauranteId),
    cantidad: items.reduce((acc, i) => acc + i.cantidad, 0),
    subtotal: c.total,
    envio,
    total: c.total + envio,
  };
}

// Solo los clientes tienen carrito.
export async function getCart() {
  if (getCurrentUser()?.rol !== "CLIENTE") return null;
  return carritoParaPantallas(await pedir("GET", "/carrito"));
}

// Suma una unidad del plato. El primer plato reserva los ingredientes por 10 minutos.
// Cada clic lleva su Idempotency-Key: si la misma solicitud llega dos veces, Pedidos suma una sola.
export async function addToCart(platoId) {
  const clave = crypto.randomUUID();
  const carrito = await pedir("POST", "/carrito/items", {
    cuerpo: { platoId, cantidad: 1 },
    headers: { "Idempotency-Key": clave },
  });
  avisarCambio();
  return carritoParaPantallas(carrito);
}

// Suma o resta unidades de un plato que ya está en el carrito (si llega a 0, lo saca).
export async function updateCartItem(platoId, delta) {
  const actual = await pedir("GET", "/carrito");
  const item = actual?.items?.find((i) => i.platoId === platoId);
  if (!item) throw new ApiError("Ese plato no está en tu carrito.", "NO_ESTA_EN_EL_CARRITO", 404);
  const cantidad = item.cantidad + delta;
  const carrito =
    cantidad <= 0
      ? await pedir("DELETE", `/carrito/items/${platoId}`)
      : await pedir("PATCH", `/carrito/items/${platoId}`, { cuerpo: { cantidad } });
  avisarCambio();
  return carritoParaPantallas(carrito);
}

export async function removeFromCart(platoId) {
  const carrito = await pedir("DELETE", `/carrito/items/${platoId}`);
  avisarCambio();
  return carritoParaPantallas(carrito);
}

// Vacía el carrito y libera los ingredientes reservados.
export async function clearCart() {
  await pedir("DELETE", "/carrito");
  avisarCambio();
  return null;
}

const esperar = (ms) => new Promise((resolver) => window.setTimeout(resolver, ms));

// Confirma el carrito. La Idempotency-Key se arma una vez por intento de confirmar: si el
// navegador reintenta o el cliente aprieta dos veces, Pedidos devuelve el mismo pedido.
//   - 201: el pedido quedó creado.
//   - 202: "Estamos confirmando tu pedido" (Inventario no contestó a tiempo). Se consulta cómo
//     va, hasta 20 s; si todavía no terminó, se avisa y el pedido aparece en "Mis pedidos".
// direccion es adónde se lleva el pedido: viaja con el pedido y la ve la cocina.
export async function confirmCart({ direccion } = {}) {
  const clave = crypto.randomUUID();
  const respuesta = await pedir("POST", "/pedidos", {
    headers: { "Idempotency-Key": clave },
    cuerpo: { direccionEntrega: String(direccion || "").trim() },
  });
  if (respuesta?.confirmacionId) {
    for (let i = 0; i < 10; i++) {
      await esperar(2000);
      const conf = await pedir("GET", `/confirmaciones/${respuesta.confirmacionId}`);
      if (conf.estado === "COMPLETADA") {
        avisarCambio();
        return { id: conf.pedidoId, confirmando: false };
      }
      if (conf.estado === "FALLIDA") {
        avisarCambio();
        throw new ApiError("Tu reserva venció mientras confirmábamos y liberamos los ingredientes. Volvé a armar el carrito.", "CARRITO_VENCIDO", 409);
      }
    }
    avisarCambio();
    return { id: null, confirmando: true };
  }
  avisarCambio();
  return { id: respuesta.id, confirmando: false };
}

// El id del pedido es largo (UUID): para mostrarlo se usan sus primeros 8 caracteres.
export function codigoPedido(id) {
  return String(id || "").slice(0, 8).toUpperCase();
}

// ---------------------------------------------------------------------------
// Pedidos del cliente y tablero de la cocina (Pedidos, L18)
// ---------------------------------------------------------------------------

// Convierte un pedido de Pedidos a la forma que usan las pantallas. El envío es el que se cobró al
// confirmar y quedó guardado en el pedido: si la cocina cambia su tarifa, el pedido no cambia. En
// los pedidos anteriores a guardarlo, el envío viene en null y se muestra "no registrado". Los avisos por mail son uno por cambio de estado (cada
// cambio publica pedido.estado_cambiado y Notificaciones manda el mail).
function pedidoParaPantallas(p, cocina) {
  const items = (p.items || []).map((it) => ({
    platoId: it.platoId,
    nombre: it.nombre,
    precio: it.precioUnitario,
    cantidad: it.cantidad,
    restauranteId: it.restauranteId,
  }));
  const envio = p.costoEnvio ?? null;
  const codigo = codigoPedido(p.id);
  return {
    id: p.id,
    codigo,
    cocinaId: p.cocinaId,
    cocinaNombre: cocina?.nombre || p.cocinaId,
    // Vacía en los pedidos anteriores a guardar la dirección.
    direccion: p.direccionEntrega || "",
    creadoEn: Date.parse(p.creadoEn),
    estado: p.estado,
    items,
    restaurantes: porRestaurante(items, cocina?.marcas || [], (i) => i.restauranteId).map((g) => ({
      restauranteId: g.restaurante.id,
      nombre: g.restaurante.nombre,
      color: g.restaurante.color,
      items: g.items,
    })),
    cantidad: items.reduce((acc, i) => acc + i.cantidad, 0),
    envio,
    subtotal: p.total,
    total: p.totalConEnvio ?? p.total,
    envioRegistrado: envio !== null,
    comprobante: p.comprobante || null,
    notificaciones: (p.historial || [])
      .slice()
      .reverse()
      .map((h) => ({ id: `${p.id}-${h.version}`, fecha: Date.parse(h.ocurridoEn), asunto: `Tu pedido #${codigo}: ${estadoLabel(h.estadoNuevo)}` })),
  };
}

async function cocinasPorId() {
  const cocinas = await getCocinas();
  return new Map(cocinas.map((c) => [c.id, c]));
}

// Mis pedidos: los últimos 20, del más nuevo al más viejo, con su historial y su factura (por
// eso se pide el detalle de cada uno).
export async function getMyOrders() {
  const [lista, cocinas] = await Promise.all([pedir("GET", "/pedidos"), cocinasPorId()]);
  const detalles = await Promise.all((lista?.pedidos || []).map((p) => pedir("GET", `/pedidos/${p.id}`)));
  return detalles.map((p) => pedidoParaPantallas(p, cocinas.get(p.cocinaId)));
}

// Solo mientras la cocina no lo aceptó. Los ingredientes vuelven a Inventario en segundo plano.
export async function cancelOrder(pedidoId) {
  const pedido = await pedir("POST", `/pedidos/${pedidoId}/cancelar`);
  avisarCambio();
  return pedido;
}

// "Ya lo recibí": solo cuando está en camino.
export async function confirmDelivery(pedidoId) {
  const pedido = await pedir("POST", `/pedidos/${pedidoId}/entregado`);
  avisarCambio();
  return pedido;
}

// Estados que la cocina todavía tiene que mover (las columnas del tablero).
const ESTADOS_DEL_TABLERO = ["RECIBIDO", "ACEPTADO", "EN_PREPARACION", "LISTO"];

// Todos los pedidos de la cocina de hoy, de a páginas (vienen del más nuevo al más viejo).
async function pedidosDeHoy() {
  const inicioDia = new Date().setHours(0, 0, 0, 0);
  const deHoy = [];
  for (let pagina = 1; pagina <= 10; pagina++) {
    const respuesta = await pedir("GET", `/cocina/pedidos?pagina=${pagina}`);
    const lista = respuesta?.pedidos || [];
    const hoy = lista.filter((p) => Date.parse(p.creadoEn) >= inicioDia);
    deHoy.push(...hoy);
    if (hoy.length < lista.length || lista.length < respuesta.porPagina) break;
  }
  return deHoy;
}

// Tablero de la cocina: los pedidos a mover (de cualquier día), más los números de hoy.
export async function getKitchenBoard() {
  const cocinaId = getCurrentUser()?.cocinaId;
  const [cocinas, deHoy, ...porEstado] = await Promise.all([
    cocinasPorId(),
    pedidosDeHoy(),
    ...ESTADOS_DEL_TABLERO.map((estado) => pedir("GET", `/cocina/pedidos?estado=${estado}`)),
  ]);
  const cocina = cocinas.get(cocinaId) || { id: cocinaId, nombre: cocinaId, marcas: [], envio: 0 };
  const pedidos = porEstado
    .flatMap((r) => r?.pedidos || [])
    .map((p) => pedidoParaPantallas(p, cocina))
    .sort((a, b) => a.creadoEn - b.creadoEn); // el más viejo primero: es el que más espera
  const vendidos = deHoy.filter((p) => p.estado !== "CANCELADO").map((p) => pedidoParaPantallas(p, cocina));
  return {
    cocina,
    marcas: cocina.marcas,
    pedidos,
    stats: {
      pedidosHoy: vendidos.length,
      enPreparacion: pedidos.filter((p) => p.estado === "EN_PREPARACION").length,
      ventasHoy: vendidos.reduce((acc, p) => acc + p.total, 0),
    },
  };
}

// Avanza el pedido un paso: un pedido nuevo se acepta; los demás pasan al estado siguiente.
export async function advanceOrder(pedido) {
  const accion = pedido.estado === "RECIBIDO" ? "aceptar" : "avanzar";
  const actualizado = await pedir("POST", `/cocina/pedidos/${pedido.id}/${accion}`);
  avisarCambio();
  return actualizado;
}

// Rechazar cancela el pedido completo; los ingredientes vuelven a Inventario.
export async function rejectOrder(pedidoId) {
  const actualizado = await pedir("POST", `/cocina/pedidos/${pedidoId}/rechazar`);
  avisarCambio();
  return actualizado;
}

// ---------------------------------------------------------------------------
// Admin de cocina (G21): cocinas y restaurantes (Catálogo), cuentas (Usuarios) y métricas
// (Pedidos). El gateway y cada servicio controlan que sea admin y de su localidad: acá solo
// se arman los pedidos.
// ---------------------------------------------------------------------------

// Cocinas de la localidad del admin, activas o no, con todos sus restaurantes (HU14, HU16).
export async function getAdminCocinas() {
  const res = await pedir("GET", "/admin/cocinas");
  const cocinas = (res.items || []).map((c) => ({
    ...cocinaParaPantallas(c),
    tiempoMin: c.tiempoEntregaMin,
    tiempoMax: c.tiempoEntregaMax,
    restaurantes: (c.restaurantes || []).map((r, i) => ({
      id: r.id,
      nombre: r.nombre,
      activo: r.activo !== false,
      color: COLORES_MARCA[i % COLORES_MARCA.length],
    })),
  }));
  const localidad = cocinas[0]?.localidad || getCurrentUser()?.localidad || "";
  return { localidad: { nombre: localidad }, cocinas };
}

// Crea una cocina en la localidad del admin y su cuenta (HU14, HU15). Son dos servicios: si
// la cuenta falla, la cocina ya quedó creada, y se avisa para crear la cuenta de nuevo.
export async function crearCocina({ nombre, direccion, zonaId, envio, tiempoMin, tiempoMax, email, password }) {
  const zona = zonas.find((z) => z.id === zonaId);
  if (!zona) throw new ApiError("Elegí la zona donde está la cocina.", "DATOS_INVALIDOS", 400);
  const cocina = await pedir("POST", "/admin/cocinas", {
    cuerpo: {
      nombre: nombre.trim(),
      direccion: direccion.trim(),
      ubicacion: { lat: zona.lat, lng: zona.lng },
      tiempoEntregaMin: Number(tiempoMin),
      tiempoEntregaMax: Number(tiempoMax),
      costoEnvio: Number(envio) || 0,
    },
  });
  avisarCambio();
  try {
    await crearCuentaCocina({ cocinaId: cocina.id, nombre: cocina.nombre, email, password });
  } catch (error) {
    throw new ApiError(`Se creó la cocina ${cocina.nombre}, pero no su cuenta: ${error.message}`, error.code, error.status);
  }
  return cocina;
}

// Cuenta con la que la cocina entra a ver sus pedidos, su menú y su stock (HU15).
export async function crearCuentaCocina({ cocinaId, nombre, email, password }) {
  return pedir("POST", "/admin/cuentas-cocina", { cuerpo: { cocinaId, nombre, email: email.trim(), password } });
}

// Pausa o reactiva una cocina: pausada, deja de aparecer para los clientes (RN19).
export async function toggleCocina(cocina) {
  const res = await pedir("PATCH", `/admin/cocinas/${encodeURIComponent(cocina.id)}`, { cuerpo: { activa: !cocina.activa } });
  avisarCambio();
  return res;
}

// Cambia los datos de una cocina de su localidad: nombre, dirección, tiempos y costo de envío.
export async function editarCocina(cocinaId, { nombre, direccion, tiempoMin, tiempoMax, envio }) {
  const res = await pedir("PATCH", `/admin/cocinas/${encodeURIComponent(cocinaId)}`, {
    cuerpo: {
      nombre: nombre.trim(),
      direccion: direccion.trim(),
      tiempoEntregaMin: Number(tiempoMin),
      tiempoEntregaMax: Number(tiempoMax),
      costoEnvio: Number(envio) || 0,
    },
  });
  avisarCambio();
  return res;
}

// Cambia el nombre de un restaurante (Catálogo controla que no se repita en la cocina).
export async function editarRestaurante(cocinaId, restauranteId, nombre) {
  const res = await pedir("PATCH", `/admin/cocinas/${encodeURIComponent(cocinaId)}/restaurantes/${encodeURIComponent(restauranteId)}`, {
    cuerpo: { nombre: nombre.trim() },
  });
  avisarCambio();
  return res;
}

// Agrega un restaurante a una cocina, hasta 4 (HU16, RN20).
export async function crearRestaurante(cocinaId, { nombre }) {
  const res = await pedir("POST", `/admin/cocinas/${encodeURIComponent(cocinaId)}/restaurantes`, { cuerpo: { nombre: nombre.trim() } });
  avisarCambio();
  return res;
}

// Pausa o reactiva un restaurante: sus platos dejan de mostrarse (RN19).
export async function toggleRestaurante(cocinaId, restaurante) {
  const res = await pedir("PATCH", `/admin/cocinas/${encodeURIComponent(cocinaId)}/restaurantes/${encodeURIComponent(restaurante.id)}`, {
    cuerpo: { activo: !restaurante.activo },
  });
  avisarCambio();
  return res;
}

const DIAS = ["Dom", "Lun", "Mar", "Mié", "Jue", "Vie", "Sáb"];

// Métricas del negocio de una cocina de su localidad, últimos 7 días (HU22). Las calcula
// Pedidos con los pedidos guardados; no cuenta los cancelados.
// Nombre de cada restaurante de las cocinas del admin, por id.
async function nombresDeRestaurantes() {
  try {
    const res = await pedir("GET", "/admin/cocinas");
    return Object.fromEntries((res.items || []).flatMap((c) => (c.restaurantes || []).map((r) => [r.id, r.nombre])));
  } catch {
    return {};
  }
}

export async function getMetricas(cocinaId) {
  const qs = cocinaId ? `?cocinaId=${encodeURIComponent(cocinaId)}` : "";
  const res = await pedir("GET", `/admin/metricas${qs}`);
  const d = res.detalle;
  if (!d) return null;
  // Los pedidos anteriores a guardar el nombre del restaurante lo tienen vacío: se completa con
  // el nombre actual que tiene Catálogo.
  const nombres = (d.porMarca || []).some((m) => !m.nombre) ? await nombresDeRestaurantes() : {};
  return {
    pedidos: d.pedidos,
    cancelados: d.cancelados,
    facturacion: d.facturacion,
    ticketPromedio: d.ticketPromedio,
    minutosPromedio: d.minutosPromedio,
    // La fecha viene como AAAA-MM-DD (día de Córdoba): se arma a mediodía para que la zona
    // horaria del navegador no la corra al día anterior.
    dias: (d.dias || []).map((dia) => ({
      fecha: dia.fecha,
      label: DIAS[new Date(`${dia.fecha}T12:00:00`).getDay()],
      cantidad: dia.cantidad,
    })),
    topPlatos: (d.topPlatos || []).map((p) => ({
      id: p.platoId,
      nombre: p.nombre,
      restauranteNombre: p.restauranteNombre,
      vendidos: p.vendidos,
      foto: fotoDe(p.platoId),
    })),
    porMarca: (d.porMarca || []).map((m) => ({ id: m.restauranteId, nombre: m.nombre || nombres[m.restauranteId] || m.restauranteId, pedidos: m.pedidos, porcentaje: m.porcentaje })),
  };
}
