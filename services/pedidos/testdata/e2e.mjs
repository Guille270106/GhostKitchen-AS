// Prueba de punta a punta de Pedidos con todo el sistema levantado (entra por el gateway, en 8080,
// como lo haría el frontend). Recorre: el admin crea una cuenta Cocina; la cocina crea un plato,
// un ingrediente con stock 10 y la receta; un cliente compra, confirma (Inventario descuenta),
// cancela (Inventario devuelve), y en otro pedido la cocina lo lleva hasta EN_CAMINO y el cliente
// confirma que lo recibió. Al final revisa que pedido.estado_cambiado haya llegado a RabbitMQ.
//
// Uso, desde la raíz del repositorio y con el sistema levantado (docker compose up -d --build):
//   node services/pedidos/testdata/e2e.mjs
// Necesita Node 18 o más nuevo. Lee ADMIN_PASSWORD y RABBITMQ_USER/RABBITMQ_PASSWORD del .env
// (no los imprime). Cada corrida crea datos nuevos con un sufijo distinto, así se puede repetir.
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

// El .env está en la raíz del repositorio, tres carpetas arriba de este archivo.
const repo = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..");
const env = Object.fromEntries(readFileSync(join(repo, ".env"), "utf8").split(/\r?\n/)
  .filter((l) => /^[A-Z_]+=/.test(l)).map((l) => [l.slice(0, l.indexOf("=")), l.slice(l.indexOf("=") + 1).trim()]));
const API = "http://localhost:8080/api/v1";
const marca = Date.now().toString(36);
let fallas = 0;

async function pedir(metodo, ruta, { token, cuerpo, headers = {} } = {}) {
  const h = { ...headers };
  if (token) h.Authorization = "Bearer " + token;
  if (cuerpo !== undefined) h["Content-Type"] = "application/json";
  const r = await fetch(API + ruta, { method: metodo, headers: h, body: cuerpo === undefined ? undefined : JSON.stringify(cuerpo) });
  const texto = await r.text();
  let json = null;
  try { json = JSON.parse(texto); } catch { /* sin cuerpo */ }
  return { estado: r.status, json, texto };
}
function revisar(ok, que, detalle = "") {
  console.log(`${ok ? "OK   " : "FALLA"} ${que}${ok ? "" : "  -> " + detalle}`);
  if (!ok) fallas++;
  return ok;
}
function exigir(ok, que, detalle) {
  if (!revisar(ok, que, detalle)) { console.log("\nNo se puede seguir."); process.exit(1); }
}
const esperar = (ms) => new Promise((r) => setTimeout(r, ms));

async function colaMailer() {
  const auth = "Basic " + Buffer.from(env.RABBITMQ_USER + ":" + env.RABBITMQ_PASSWORD).toString("base64");
  const r = await fetch("http://localhost:15672/api/queues/%2F/notificaciones.mailer", { headers: { Authorization: auth } });
  const q = await r.json();
  return (q.messages ?? 0) + (q.message_stats?.ack ?? 0);
}

// ---------- 1. Preparación: admin, cuenta Cocina, plato nuevo con receta y stock ----------
console.log("\n== Preparación ==");
let r = await pedir("POST", "/auth/login", { cuerpo: { email: "admin.cerro@ghostkitchen.dev", password: env.ADMIN_PASSWORD } });
exigir(r.estado === 200, "login del admin de Cerro", r.texto);
const admin = r.json.token;

const cocinaEmail = `cocina.e2e.${marca}@ghostkitchen.dev`;
r = await pedir("POST", "/admin/cuentas-cocina", { token: admin, cuerpo: { nombre: "Cocina e2e", email: cocinaEmail, password: "clave-cocina-e2e", cocinaId: "cerro" } });
exigir(r.estado === 201, "el admin crea una cuenta Cocina para cerro", r.estado + " " + r.texto);
r = await pedir("POST", "/auth/login", { cuerpo: { email: cocinaEmail, password: "clave-cocina-e2e" } });
exigir(r.estado === 200, "login de la cocina", r.texto);
const cocina = r.json.token;

r = await pedir("GET", "/cocina/platos", { token: cocina });
exigir(r.estado === 200, "la cocina ve sus platos", r.estado + " " + r.texto);
const lista = r.json.items ?? r.json.platos ?? r.json;
const modelo = lista.find((p) => p.restauranteId) ?? lista[0];
r = await pedir("POST", `/cocina/restaurantes/${modelo.restauranteId}/platos`, { token: cocina,
  cuerpo: { nombre: `Plato e2e ${marca}`, descripcion: "prueba de punta a punta", categoria: modelo.categoria, precio: 5000, etiquetas: [] } });
exigir(r.estado === 201, "la cocina crea un plato nuevo (nace desactivado)", r.estado + " " + r.texto);
const plato = r.json;

r = await pedir("POST", "/cocina/ingredientes", { token: cocina, cuerpo: { nombre: `Ingrediente e2e ${marca}`, unidad: "unidades", stockInicial: 10 } });
exigir(r.estado === 201, "la cocina crea un ingrediente con stock 10", r.estado + " " + r.texto);
const ingrediente = r.json;

// Inventario conoce el plato por el mensaje plato.creado de Catálogo (outbox): puede tardar un poco.
for (let i = 0; i < 20; i++) {
  r = await pedir("PUT", `/cocina/platos/${plato.id}/receta`, { token: cocina, cuerpo: { items: [{ ingredienteId: ingrediente.id, cantidad: 2 }] } });
  if (r.estado === 200) break;
  await esperar(1000);
}
exigir(r.estado === 200, "la receta: 2 unidades del ingrediente por plato", r.estado + " " + r.texto);
r = await pedir("PATCH", `/cocina/platos/${plato.id}`, { token: cocina, cuerpo: { activo: true } });
exigir(r.estado === 200, "la cocina activa el plato (ya tiene receta)", r.estado + " " + r.texto);

async function stock() {
  const s = await pedir("GET", `/cocina/ingredientes/${ingrediente.id}`, { token: cocina });
  return s.json;
}

// ---------- 2. Cliente: carrito, confirmar (descuento real), cancelar (devolución real) ----------
console.log("\n== Cliente: confirmar y cancelar ==");
const clienteEmail = `cliente.e2e.${marca}@mail.com`;
r = await pedir("POST", "/auth/registro", { cuerpo: { nombre: "Cliente e2e", email: clienteEmail, password: "clave-cliente-e2e", direccion: "Obispo Trejo 323" } });
exigir(r.estado === 201, "registro del cliente", r.estado + " " + r.texto);
r = await pedir("POST", "/auth/login", { cuerpo: { email: clienteEmail, password: "clave-cliente-e2e" } });
exigir(r.estado === 200, "login del cliente", r.texto);
const cliente = r.json.token;

const mensajesAntes = await colaMailer();

// Inventario se entera de que el plato se activó por un mensaje de Catálogo: puede tardar.
let espera = 0;
for (; espera < 15; espera++) {
  r = await pedir("POST", "/carrito/items", { token: cliente, cuerpo: { platoId: plato.id, cantidad: 2 } });
  if (r.estado === 200) break;
  await esperar(1000);
}
exigir(r.estado === 200, `agrega 2 platos al carrito (reserva 4 unidades; esperó ${espera} s a que Inventario supiera del plato activo)`, r.estado + " " + r.texto);
let s = await stock();
revisar(JSON.stringify(s).includes("4"), "Inventario: 4 unidades reservadas", JSON.stringify(s));

r = await pedir("POST", "/pedidos", { token: cliente, headers: { "Idempotency-Key": `e2e-${marca}-1` } });
exigir(r.estado === 201 && r.json.estado === "RECIBIDO", "confirma: 201 y el pedido en RECIBIDO", r.estado + " " + r.texto);
const pedido1 = r.json;
const otraVez = await pedir("POST", "/pedidos", { token: cliente, headers: { "Idempotency-Key": `e2e-${marca}-1` } });
revisar(otraVez.estado === 201 && otraVez.json.id === pedido1.id, "doble clic con la misma clave: el mismo pedido", otraVez.estado + " " + otraVez.texto);
s = await stock();
const despuesDeConfirmar = JSON.stringify(s);
console.log("      stock después de confirmar:", despuesDeConfirmar);

r = await pedir("GET", "/pedidos?estado=RECIBIDO", { token: cliente });
revisar(r.estado === 200 && r.json.pedidos.some((p) => p.id === pedido1.id), "mis pedidos con ?estado=RECIBIDO lo muestra (el gateway reenvía el filtro)", r.estado + " " + r.texto);
r = await pedir("GET", "/pedidos?estado=ENTREGADO", { token: cliente });
revisar(r.estado === 200 && !r.json.pedidos.some((p) => p.id === pedido1.id), "con ?estado=ENTREGADO no aparece", r.texto);
r = await pedir("GET", "/cocina/pedidos?estado=RECIBIDO", { token: cocina });
revisar(r.estado === 200 && r.json.pedidos.some((p) => p.id === pedido1.id), "la pantalla de la cocina lo muestra", r.estado + " " + r.texto);
r = await pedir("GET", "/cocina/pedidos", { token: cliente });
revisar(r.estado === 403, "un cliente no entra a la pantalla de la cocina (403)", r.estado + "");

r = await pedir("POST", `/pedidos/${pedido1.id}/cancelar`, { token: cliente });
revisar(r.estado === 200 && r.json.estado === "CANCELADO", "el cliente lo cancela", r.estado + " " + r.texto);
let devuelto = false;
for (let i = 0; i < 15 && !devuelto; i++) {
  await esperar(1000);
  s = await stock();
  devuelto = JSON.stringify(s) !== despuesDeConfirmar;
}
console.log("      stock después de cancelar:  ", JSON.stringify(s));
revisar(devuelto, "Inventario devolvió los ingredientes (en segundo plano, menos de 15 s)", JSON.stringify(s));

// ---------- 3. Cocina: aceptar y avanzar; cliente: confirmar que lo recibió ----------
console.log("\n== Cocina y entrega ==");
r = await pedir("POST", "/carrito/items", { token: cliente, cuerpo: { platoId: plato.id, cantidad: 1 } });
exigir(r.estado === 200, "otro carrito con 1 plato", r.estado + " " + r.texto);
r = await pedir("POST", "/pedidos", { token: cliente, headers: { "Idempotency-Key": `e2e-${marca}-2` } });
exigir(r.estado === 201, "confirma el segundo pedido", r.estado + " " + r.texto);
const pedido2 = r.json;
r = await pedir("POST", `/pedidos/${pedido2.id}/entregado`, { token: cliente });
revisar(r.estado === 409, "todavía no puede confirmar la entrega (409)", r.estado + "");
for (const [accion, esperado] of [["aceptar", "ACEPTADO"], ["avanzar", "EN_PREPARACION"], ["avanzar", "LISTO"], ["avanzar", "EN_CAMINO"]]) {
  r = await pedir("POST", `/cocina/pedidos/${pedido2.id}/${accion}`, { token: cocina });
  revisar(r.estado === 200 && r.json.estado === esperado, `la cocina: ${accion} -> ${esperado}`, r.estado + " " + r.texto);
}
r = await pedir("POST", `/pedidos/${pedido2.id}/cancelar`, { token: cliente });
revisar(r.estado === 409, "ya no se puede cancelar (409)", r.estado + "");
r = await pedir("POST", `/pedidos/${pedido2.id}/entregado`, { token: cliente });
revisar(r.estado === 200 && r.json.estado === "ENTREGADO", "el cliente confirma que lo recibió", r.estado + " " + r.texto);
r = await pedir("GET", `/pedidos/${pedido2.id}`, { token: cliente });
revisar(r.estado === 200 && r.json.historial?.length === 6, "el historial tiene los 6 estados", r.texto);

// ---------- 4. Mensajes en RabbitMQ ----------
console.log("\n== Mensajes ==");
// Las estadísticas de RabbitMQ se actualizan cada 5 s: esperamos hasta 20 s a que lleguen los 8.
let nuevos = 0;
for (let i = 0; i < 20 && nuevos < 8; i++) {
  await esperar(1000);
  nuevos = (await colaMailer()) - mensajesAntes;
}
revisar(nuevos >= 8, `pedido.estado_cambiado llegó a notificaciones.mailer (${nuevos} nuevos; se esperan 8: 2 del primero y 6 del segundo)`, String(nuevos));

console.log(fallas === 0 ? "\nTODO OK" : `\n${fallas} FALLA(S)`);
process.exit(fallas === 0 ? 0 : 1);
