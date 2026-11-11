export function formatPrice(value) {
  return `$ ${Math.round(Number(value) || 0).toLocaleString("es-AR")}`;
}

export function formatCountdown(ms) {
  const totalSeconds = Math.max(0, Math.ceil(ms / 1000));
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}:${String(seconds).padStart(2, "0")}`;
}

export function formatRelative(timestamp) {
  const diff = Date.now() - timestamp;
  const minutes = Math.floor(diff / 60000);
  if (minutes < 1) return "recién";
  if (minutes < 60) return `hace ${minutes} min`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `hace ${hours} h`;
  return formatDate(timestamp);
}

export function formatDate(timestamp) {
  return new Date(timestamp).toLocaleDateString("es-AR", { day: "numeric", month: "long" });
}

export function formatTime(timestamp) {
  return new Date(timestamp).toLocaleTimeString("es-AR", { hour: "2-digit", minute: "2-digit", hour12: false });
}

export function formatToday() {
  const text = new Date().toLocaleDateString("es-AR", { weekday: "long", day: "numeric", month: "long" });
  return text.charAt(0).toUpperCase() + text.slice(1);
}

export function initials(name) {
  return String(name || "")
    .split(" ")
    .filter(Boolean)
    .slice(0, 2)
    .map((word) => word[0].toUpperCase())
    .join("");
}

export const ESTADOS_PEDIDO = ["RECIBIDO", "ACEPTADO", "EN_PREPARACION", "LISTO", "EN_CAMINO", "ENTREGADO"];

const ESTADO_LABELS = {
  RECIBIDO: "Recibido",
  ACEPTADO: "Aceptado",
  EN_PREPARACION: "En preparación",
  LISTO: "Listo",
  EN_CAMINO: "En camino",
  ENTREGADO: "Entregado",
  CANCELADO: "Cancelado",
  PENDIENTE: "Pendiente",
  ACEPTADA: "Aceptada",
  LISTA: "Lista",
  DESPACHADA: "Despachada",
  RECHAZADA: "Rechazada",
};

export function estadoLabel(estado) {
  return ESTADO_LABELS[estado] || estado;
}
