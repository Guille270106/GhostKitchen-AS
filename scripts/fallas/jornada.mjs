// Jornada de fallas controladas (ADR-010). Apaga a propósito una dependencia por vez,
// mide qué ve el usuario y cuánto tarda el sistema en recuperarse cuando vuelve:
//
//   1. Inventario caído    (reservar, confirmar y cancelar)
//   2. RabbitMQ caído      (pedidos y avisos)
//   3. Correo caído        (Mailpit: corto, y largo hasta la DLQ)
//   4. Proveedor caído     (la factura del Grupo 4)
//
// Uso, desde la raíz del repositorio, con todo levantado:
//   docker compose up -d --build
//   node scripts/fallas/jornada.mjs
// Tarda unos 10 minutos. Al final deja todo prendido de nuevo. Lee ADMIN_PASSWORD, la clave de
// MySQL de Pedidos y el usuario de RabbitMQ del .env (no los imprime).
//
// Toda la jornada usa el mock del proveedor. Si el .env apunta al servicio real del Grupo 4
// (PROVIDER_BASE_URL completa), Pedidos se reinicia apuntando al mock antes de empezar y vuelve a
// quedar como dice el .env al terminar. Así los pedidos de prueba no generan facturas en el
// sistema del Grupo 4, y en el paso 4 apagar el mock sí deja a Pedidos sin proveedor.
// Si la jornada se corta a la mitad, `docker compose up -d` deja todo como dice el .env.
import { execSync } from "node:child_process";
import { readFileSync } from "node:fs";

const env = Object.fromEntries(readFileSync(".env", "utf8").split(/\r?\n/)
  .filter((l) => /^[A-Z_]+=/.test(l)).map((l) => [l.slice(0, l.indexOf("=")), l.slice(l.indexOf("=") + 1).trim()]));
const API = "http://localhost:8080/api/v1";
const marca = Date.now().toString(36);
const inicio = Date.now();
let fallas = 0;

// ---------- ayudas ----------
const esperar = (ms) => new Promise((r) => setTimeout(r, ms));
const reloj = () => `[${String(Math.round((Date.now() - inicio) / 1000)).padStart(4)} s]`;
function log(texto) { console.log(`${reloj()} ${texto}`); }
function revisar(ok, que, detalle = "") {
  console.log(`${reloj()} ${ok ? "OK   " : "FALLA"} ${que}${ok ? "" : "  -> " + detalle}`);
  if (!ok) fallas++;
}
function docker(accion, servicio) {
  execSync(`docker compose ${accion} ${servicio}`, { stdio: "ignore" });
  log(`docker compose ${accion} ${servicio}`);
}

async function pedir(metodo, ruta, { token, cuerpo, headers = {} } = {}) {
  const h = { ...headers };
  if (token) h.Authorization = "Bearer " + token;
  if (cuerpo !== undefined) h["Content-Type"] = "application/json";
  const t0 = Date.now();
  const r = await fetch(API + ruta, { method: metodo, headers: h, body: cuerpo === undefined ? undefined : JSON.stringify(cuerpo) });
  const texto = await r.text();
  let json = null;
  try { json = JSON.parse(texto); } catch { /* sin cuerpo */ }
  return { estado: r.status, json, texto, ms: Date.now() - t0 };
}

// Espera hasta que la condición dé true (o se cumpla el máximo) y devuelve cuánto tardó.
async function hasta(condicion, maxSeg) {
  const t0 = Date.now();
  while (Date.now() - t0 < maxSeg * 1000) {
    try { if (await condicion()) return Math.round((Date.now() - t0) / 1000); } catch { /* sigue */ }
    await esperar(1000);
  }
  return null;
}

function sqlPedidos(consulta) {
  const salida = execSync(`docker compose exec -T -e MYSQL_PWD mysql mysql -upedidos pedidos_db -N -e "${consulta}"`,
    { env: { ...process.env, MYSQL_PWD: env.PEDIDOS_DB_PASSWORD }, stdio: ["ignore", "pipe", "ignore"] });
  return salida.toString().trim();
}

// Cuántos mails hay en Mailpit de un pedido. El asunto lleva el código corto (los primeros 8
// caracteres del id, en mayúsculas) y el estado en palabras: "Tu pedido #2207B999 está en preparación".
// Con "texto" se buscan además esas palabras del asunto.
const codigo = (pedidoId) => String(pedidoId).slice(0, 8).toUpperCase();
async function mailsDe(pedidoId, texto = "") {
  const busqueda = `subject:"#${codigo(pedidoId)}${texto ? " " + texto : ""}"`;
  try {
    const r = await fetch(`http://localhost:8025/api/v1/search?query=${encodeURIComponent(busqueda)}`);
    return (await r.json()).messages_count ?? 0;
  } catch {
    return 0; // el correo está apagado: todavía no hay mails
  }
}

async function cola(nombre) {
  const auth = "Basic " + Buffer.from(env.RABBITMQ_USER + ":" + env.RABBITMQ_PASSWORD).toString("base64");
  const r = await fetch(`http://localhost:15672/api/queues/%2F/${nombre}`, { headers: { Authorization: auth } });
  return (await r.json()).messages ?? 0;
}

async function alertas() {
  const r = await fetch("http://localhost:9090/api/v1/alerts");
  return (await r.json()).data.alerts.map((a) => `${a.labels.alertname}(${a.labels.job || a.labels.queue || ""}):${a.state}`);
}

// ---------- preparación: todo prendido, una cocina con un plato con receta y stock, y un cliente ----------
console.log("\n== Preparación ==");
// Por si una corrida anterior se cortó con algo apagado: se arranca con todo prendido y sano.
try {
  execSync("docker compose up -d --wait", { stdio: "ignore" });
} catch {
  // --wait también "falla" si un trabajo de un solo paso (como la carga de colas) ya terminó: se sigue igual.
}
await hasta(async () => (await fetch("http://localhost:8025/api/v1/messages?limit=1")).ok, 60);
log("todo prendido");

// Reinicia Pedidos con las variables del proveedor que se le pasen (o las del .env si no se pasa
// nada) y espera a que vuelva a estar sano. Las variables del proceso le ganan al .env en Compose.
async function reiniciarPedidos(variables, aviso) {
  try {
    execSync("docker compose up -d --wait pedidos", { stdio: "ignore", env: { ...process.env, ...variables } });
  } catch { /* --wait puede avisar por un trabajo ya terminado: se sigue igual */ }
  // Según la versión, Compose devuelve un objeto JSON por línea o una lista.
  const sano = () => {
    const salida = execSync("docker compose ps --format json pedidos").toString().trim();
    const datos = salida.startsWith("[") ? JSON.parse(salida)[0] : JSON.parse(salida.split("\n")[0] || "{}");
    return datos?.Health === "healthy";
  };
  await hasta(async () => sano(), 90);
  log(aviso);
}
const proveedorReal = Boolean(env.PROVIDER_BASE_URL);
if (proveedorReal) {
  await reiniciarPedidos({ PROVIDER_BASE_URL: "http://comprobantes-mock:4010", PROVIDER_API_KEY: "clave-del-mock" },
    "el .env apunta al proveedor real: durante la jornada Pedidos usa el mock");
}
let r = await pedir("POST", "/auth/login", { cuerpo: { email: "admin.cerro@ghostkitchen.dev", password: env.ADMIN_PASSWORD } });
const admin = r.json.token;
const cocinaEmail = `cocina.fallas.${marca}@ghostkitchen.dev`;
await pedir("POST", "/admin/cuentas-cocina", { token: admin, cuerpo: { nombre: "Cocina fallas", email: cocinaEmail, password: "clave-cocina-fallas", cocinaId: "cerro" } });
const cocina = (await pedir("POST", "/auth/login", { cuerpo: { email: cocinaEmail, password: "clave-cocina-fallas" } })).json.token;
const modelo = (await pedir("GET", "/cocina/platos", { token: cocina })).json.items.find((p) => p.restauranteId);
const plato = (await pedir("POST", `/cocina/restaurantes/${modelo.restauranteId}/platos`, { token: cocina,
  cuerpo: { nombre: `Plato fallas ${marca}`, descripcion: "jornada de fallas", categoria: modelo.categoria, precio: 5000, etiquetas: [] } })).json;
const ingrediente = (await pedir("POST", "/cocina/ingredientes", { token: cocina, cuerpo: { nombre: `Ingrediente fallas ${marca}`, unidad: "unidades", stockInicial: 20 } })).json;
await hasta(async () => (await pedir("PUT", `/cocina/platos/${plato.id}/receta`, { token: cocina, cuerpo: { items: [{ ingredienteId: ingrediente.id, cantidad: 2 }] } })).estado === 200, 20);
await pedir("PATCH", `/cocina/platos/${plato.id}`, { token: cocina, cuerpo: { activo: true } });
const clienteEmail = `cliente.fallas.${marca}@mail.com`;
await pedir("POST", "/auth/registro", { cuerpo: { nombre: "Cliente fallas", email: clienteEmail, password: "clave-cliente-fallas", direccion: "Obispo Trejo 323" } });
const cliente = (await pedir("POST", "/auth/login", { cuerpo: { email: clienteEmail, password: "clave-cliente-fallas" } })).json.token;
const stock = async () => (await pedir("GET", `/cocina/ingredientes/${ingrediente.id}`, { token: cocina })).json?.total;
// Un plato recién activado tarda un poco en poder pedirse (Inventario se entera por un mensaje).
const agregar = (cantidad = 1) => pedir("POST", "/carrito/items", { token: cliente, cuerpo: { platoId: plato.id, cantidad } });
await hasta(async () => (await agregar()).estado === 200, 20);
await pedir("DELETE", "/carrito", { token: cliente });
log(`listo: plato con receta (2 unidades por plato) y stock ${await stock()}`);

async function confirmar(clave) {
  return pedir("POST", "/pedidos", { token: cliente, cuerpo: { direccionEntrega: "Obispo Trejo 323" }, headers: { "Idempotency-Key": clave } });
}

// =====================================================================================
console.log("\n== 1. Inventario caído ==");
r = await agregar(1);
revisar(r.estado === 200, `con Inventario arriba, el cliente arma el carrito (reserva) en ${r.ms} ms`, r.texto);
docker("stop", "inventario");

// Se confirma sin tocar más el carrito: si antes falla un "agregar", ese cambio queda sin resolver
// y Pedidos no deja confirmar hasta que Inventario vuelva (eso se prueba al final de este caso).
r = await confirmar(`fallas-${marca}-1`);
const conf = r.json?.confirmacionId;
revisar(r.estado === 202 && r.json?.mensaje === "Estamos confirmando tu pedido" && r.ms < 3000,
  `confirmar: ${r.estado} "${r.json?.mensaje}" en ${r.ms} ms`, r.texto);

// La alerta ServicioCaido espera 1 minuto caído (para no avisar por un reinicio), más lo que tarda
// Prometheus en volver a mirar: se espera a que dispare y se mide cuánto tardó.
log("Inventario sigue apagado hasta que Prometheus dispare la alerta...");
const tAlerta1 = await hasta(async () => (await alertas()).some((a) => a.startsWith("ServicioCaido(inventario):firing")), 180);
revisar(tAlerta1 !== null, `alerta ServicioCaido(inventario) disparada en Prometheus y Grafana ${tAlerta1 + 4} s después de la caída`, (await alertas()).join(", "));
r = await pedir("GET", `/confirmaciones/${conf}`, { token: cliente });
revisar(r.json?.estado === "EN_CURSO", `mientras tanto la confirmación sigue ${r.json?.estado} (no se dice que falló)`, r.texto);

docker("start", "inventario");
const tRecupera = await hasta(async () => (await pedir("GET", `/confirmaciones/${conf}`, { token: cliente })).json?.estado === "COMPLETADA", 90);
r = await pedir("GET", `/confirmaciones/${conf}`, { token: cliente });
const pedido1 = r.json?.pedidoId;
revisar(tRecupera !== null, `al volver Inventario, la confirmación se completa sola en ${tRecupera} s (pedido ${String(pedido1).slice(0, 8)})`, r.texto);
const s1 = await stock();
revisar(s1 === 18, `Inventario descontó lo reservado: stock 20 -> ${s1}`, String(s1));

// Cancelar con Inventario caído: se cancela igual y la devolución queda pendiente.
docker("stop", "inventario");
r = await pedir("POST", `/pedidos/${pedido1}/cancelar`, { token: cliente });
revisar(r.estado === 200 && r.json?.estado === "CANCELADO" && r.ms < 3000, `cancelar con Inventario caído: ${r.estado} ${r.json?.estado} en ${r.ms} ms`, r.texto);
await esperar(6000);
const dev = sqlPedidos(`SELECT estado, intentos FROM devolucion WHERE pedido_id='${pedido1}'`);
revisar(dev.startsWith("PENDIENTE"), `la devolución queda anotada: ${dev.replace(/\s+/g, " ")} (estado, intentos)`, dev);

// Agregar con Inventario caído: no se inventa una reserva. Como no se sabe si Inventario llegó a
// reservar (pudo ser un timeout), el cambio queda sin resolver y Pedidos no deja confirmar ese
// carrito hasta aclararlo: confirmar consumiría una reserva que quizás no coincide con el carrito.
r = await agregar(1);
revisar(r.estado === 503 && r.json?.code === "SERVICIO_NO_DISPONIBLE" && r.ms < 3000,
  `agregar con Inventario caído: ${r.estado} ${r.json?.code} en ${r.ms} ms (no se inventa una reserva)`, r.texto);
r = await confirmar(`fallas-${marca}-1b`);
revisar(r.estado === 409 && r.json?.code === "CARRITO_MODIFICADO",
  `confirmar con ese cambio sin resolver: ${r.estado} ${r.json?.code} (no confirma a ciegas)`, r.texto);

docker("start", "inventario");
const tDevuelve = await hasta(async () => (await stock()) === 20, 90);
revisar(tDevuelve !== null, `al volver, los ingredientes vuelven al stock (20) en ${tDevuelve} s`, String(await stock()));
// Pedidos resuelve el cambio pendiente cada 10 s (repite la reserva con la misma clave y la libera,
// porque el plato nunca llegó al carrito). Después, confirmar ya no lo frena: el carrito está vacío.
let intento = 0;
const tResuelve = await hasta(async () => (await confirmar(`fallas-${marca}-1c-${intento++}`)).json?.code === "CARRITO_VACIO", 60);
revisar(tResuelve !== null, `el cambio sin resolver se aclara solo en ${tResuelve} s y el plato no quedó en el carrito`, "");

// =====================================================================================
console.log("\n== 2. RabbitMQ caído ==");
docker("stop", "rabbitmq");
await agregar(1);
r = await confirmar(`fallas-${marca}-2`);
const pedido2 = r.json?.id;
revisar(r.estado === 201 && r.ms < 3000, `confirmar sin RabbitMQ: ${r.estado} ${r.json?.estado} en ${r.ms} ms`, r.texto);
r = await pedir("POST", `/cocina/pedidos/${pedido2}/aceptar`, { token: cocina });
revisar(r.estado === 200 && r.ms < 3000, `la cocina lo acepta sin RabbitMQ: ${r.estado} ${r.json?.estado} en ${r.ms} ms`, r.texto);
await esperar(3000);
const pendientes = Number(sqlPedidos("SELECT COUNT(*) FROM outbox WHERE publicado_en IS NULL"));
revisar(pendientes >= 2, `los avisos esperan en el outbox de Pedidos: ${pendientes} sin publicar`, String(pendientes));
revisar((await mailsDe(pedido2)) === 0, "todavía no llegó ningún mail de ese pedido");

log("RabbitMQ sigue apagado hasta que Prometheus dispare la alerta...");
const tAlerta2 = await hasta(async () => (await alertas()).some((a) => a.startsWith("ServicioCaido(rabbitmq):firing")), 180);
revisar(tAlerta2 !== null, `alerta ServicioCaido(rabbitmq) disparada ${tAlerta2} s después de los primeros pedidos sin RabbitMQ`, (await alertas()).join(", "));

docker("start", "rabbitmq");
const tOutbox = await hasta(() => Number(sqlPedidos("SELECT COUNT(*) FROM outbox WHERE publicado_en IS NULL")) === 0, 120);
revisar(tOutbox !== null, `al volver RabbitMQ, el outbox se vacía en ${tOutbox} s`);
const tMails = await hasta(async () => (await mailsDe(pedido2)) >= 2, 120);
revisar(tMails !== null, `y los 2 mails del pedido (RECIBIDO y ACEPTADO) llegan ${tMails} s después`, String(await mailsDe(pedido2)));

// =====================================================================================
console.log("\n== 3. Correo caído (Mailpit) ==");
// 3a. Corte corto: Notificaciones reintenta a los 5 s y a los 30 s; el mail llega solo.
docker("stop", "mailpit");
r = await pedir("POST", `/cocina/pedidos/${pedido2}/avanzar`, { token: cocina });
revisar(r.estado === 200 && r.ms < 3000, `con el correo caído, la cocina avanza el pedido: ${r.estado} ${r.json?.estado} en ${r.ms} ms`, r.texto);
await esperar(20000);
docker("start", "mailpit");
const tCorto = await hasta(async () => (await mailsDe(pedido2, "está en preparación")) >= 1, 120);
revisar(tCorto !== null, `corte de 20 s: el mail de "está en preparación" llega ${tCorto} s después de volver el correo (reintento)`);

// 3b. Corte largo: se agotan los reintentos (5 s, 30 s y 2 min) y el aviso va a la DLQ.
const dlqAntes = await cola("notificaciones.mailer.dlq");
docker("stop", "mailpit");
r = await pedir("POST", `/cocina/pedidos/${pedido2}/avanzar`, { token: cocina });
revisar(r.estado === 200, `la cocina lo marca LISTO con el correo caído: ${r.estado} ${r.json?.estado}`, r.texto);
log("el correo sigue apagado hasta que el aviso llegue a la DLQ (unos 2 min 40 s)...");
const tDlq = await hasta(async () => (await cola("notificaciones.mailer.dlq")) > dlqAntes, 240);
revisar(tDlq !== null, `agotados los reintentos, el aviso pasa a notificaciones.mailer.dlq a los ${tDlq} s (no se pierde: queda para revisar)`);
docker("start", "mailpit");

// =====================================================================================
console.log("\n== 4. Proveedor caído (Grupo 4, factura) ==");
docker("stop", "comprobantes-mock");
await agregar(1);
r = await confirmar(`fallas-${marca}-4`);
const pedido4 = r.json?.id;
revisar(r.estado === 201 && r.ms < 3000, `confirmar con el proveedor caído: ${r.estado} ${r.json?.estado} en ${r.ms} ms (no lo espera)`, r.texto);
await esperar(15000);
r = await pedir("GET", `/pedidos/${pedido4}`, { token: cliente });
revisar(r.json?.comprobante?.estado === "PENDIENTE", `la factura queda ${r.json?.comprobante?.estado}`, r.texto);
docker("start", "comprobantes-mock");
const tFactura = await hasta(async () => (await pedir("GET", `/pedidos/${pedido4}`, { token: cliente })).json?.comprobante?.estado === "EMITIDO", 60);
r = await pedir("GET", `/pedidos/${pedido4}`, { token: cliente });
revisar(tFactura !== null, `al volver el proveedor, la factura sale sola en ${tFactura} s: ${r.json?.comprobante?.numero}`, r.texto);

// =====================================================================================
console.log("\n== Al final ==");
if (proveedorReal) await reiniciarPedidos({}, "Pedidos vuelve a usar el proveedor del .env");
const a3 = await alertas();
revisar(!a3.some((a) => a.startsWith("MuchosErrores5xx") && a.endsWith("firing")), `sin la alerta de muchos errores 5xx (alertas ahora: ${a3.join(", ") || "ninguna"})`);
r = await pedir("GET", "/cocinas");
revisar(r.estado === 200, "todo sigue respondiendo");
console.log(`\n${fallas === 0 ? "JORNADA COMPLETA: el sistema se comportó como dicen los ADR" : `${fallas} FALLA(S): revisar arriba`} (duró ${Math.round((Date.now() - inicio) / 60000)} min)`);
process.exit(fallas === 0 ? 0 : 1);
