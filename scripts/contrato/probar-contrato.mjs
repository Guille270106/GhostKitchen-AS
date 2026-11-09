// Test de contrato de nuestra API publicada (L21, ADR-008).
//
// Le pide cada operación del contrato (docs/contracts/v1/openapi.yaml) a nuestra API real:
//   - por el proxy de Prism (contrato-proxy), que compara cada respuesta con el OpenAPI: si no lo
//     cumple, responde 500 con la lista de diferencias;
//   - directo al gateway, para los errores que Prism contestaría solo sin preguntarle a nuestra
//     API (sin API key, datos inválidos): ahí se revisa a mano el formato Problem Details.
//
// Uso (con docker-compose.contrato.yml levantado, ver su encabezado):
//   API_KEY=<una clave de PUBLIC_API_KEYS> node scripts/contrato/probar-contrato.mjs
// Sin API_KEY, la toma del .env de la raíz. Sale con código 1 si algo no cumple el contrato.
import { readFileSync } from "node:fs";

const PROXY = process.env.PROXY_URL || "http://localhost:4020";
const DIRECTO = process.env.DIRECTO_URL || "http://localhost:8080/public";
const API_KEY = process.env.API_KEY || claveDelEnv();
let fallas = 0;

function claveDelEnv() {
  try {
    const linea = readFileSync(".env", "utf8").split(/\r?\n/).find((l) => l.startsWith("PUBLIC_API_KEYS="));
    return linea?.slice("PUBLIC_API_KEYS=".length).split(",")[0].trim();
  } catch {
    return "";
  }
}

async function pedir(base, metodo, ruta, { cuerpo, conClave = true } = {}) {
  const headers = { Accept: "application/json" };
  if (conClave) headers["X-API-Key"] = API_KEY;
  if (cuerpo) headers["Content-Type"] = "application/json";
  const r = await fetch(base + ruta, { method: metodo, headers, body: cuerpo ? JSON.stringify(cuerpo) : undefined });
  const texto = await r.text();
  let json = null;
  try {
    json = JSON.parse(texto);
  } catch {
    /* sin cuerpo JSON */
  }
  return { estado: r.status, json, texto };
}

function revisar(ok, que, detalle) {
  console.log(`${ok ? "OK   " : "FALLA"} ${que}${ok ? "" : "\n        " + detalle}`);
  if (!ok) fallas++;
}

// Por el proxy: el código esperado y que Prism no haya encontrado diferencias con el contrato.
async function porElContrato(que, metodo, ruta, esperado, cuerpo) {
  const r = await pedir(PROXY, metodo, ruta, { cuerpo });
  const diferencias = r.json?.validation?.map((v) => `${v.location?.join(".")}: ${v.message}`).join("\n        ");
  revisar(r.estado === esperado, `${que} -> ${esperado}`, diferencias ? `no cumple el contrato:\n        ${diferencias}` : `${r.estado} ${r.texto.slice(0, 300)}`);
  return r;
}

// Directo: el error de nuestra API, con el formato Problem Details y su code estable.
async function errorDirecto(que, metodo, ruta, esperado, code, opciones) {
  const r = await pedir(DIRECTO, metodo, ruta, opciones);
  const p = r.json || {};
  const formato = typeof p.type === "string" && typeof p.title === "string" && p.status === esperado && p.code === code;
  revisar(r.estado === esperado && formato, `${que} -> ${esperado} ${code}`, `${r.estado} ${r.texto.slice(0, 300)}`);
}

// 0. Esperamos a que estén listos la API (Búsqueda arma el índice al arrancar) y el proxy de Prism.
let lista = false;
for (let i = 0; i < 60 && !lista; i++) {
  try {
    // El proxy alcanza con que conteste (si el contrato está roto, contesta 500: eso lo informa el test).
    lista = (await pedir(DIRECTO, "GET", "/v1/sedes")).estado === 200 && (await pedir(PROXY, "GET", "/v1/sedes")).estado > 0;
  } catch {
    /* todavía no contesta */
  }
  if (!lista) await new Promise((r) => setTimeout(r, 2000));
}
if (!lista) {
  console.log("FALLA la API o el proxy de Prism no respondieron en 2 minutos");
  process.exit(1);
}

console.log("\n== Operaciones (validadas contra el OpenAPI) ==");
await porElContrato("listarSedes", "GET", "/v1/sedes", 200);
await porElContrato("buscarSedeCercana", "GET", "/v1/sedes/cercana?lat=-31.4262&lng=-64.1883", 200);
await porElContrato("buscarSedeCercana sin cobertura (Rosario)", "GET", "/v1/sedes/cercana?lat=-32.9468&lng=-60.6393", 404);
const platos = await porElContrato("buscarPlatos con filtros y orden", "GET", "/v1/sedes/nueva-cordoba/platos?q=burger&soloDisponibles=false&orden=precio_asc&page=1&pageSize=5", 200);
await porElContrato("buscarPlatos de una sede que no existe", "GET", "/v1/sedes/no-existe/platos", 404);
const platoId = platos.json?.items?.[0]?.id || "p1";
await porElContrato("obtenerPlato", "GET", `/v1/platos/${platoId}`, 200);
await porElContrato("obtenerPlato que no existe", "GET", "/v1/platos/no-existe", 404);
await porElContrato("calcularProximidad", "POST", "/v1/proximidad", 200, {
  ubicacionCliente: { lat: -31.4262, lng: -64.1883 },
  ubicacionSede: { lat: -31.4262, lng: -64.1883 },
  radioKm: 5,
  incluirDarkKitchen: true,
});

console.log("\n== Errores de nuestra API (Problem Details con code estable) ==");
await errorDirecto("sin API key", "GET", "/v1/sedes", 401, "API_KEY_INVALIDA", { conClave: false });
await errorDirecto("latitud fuera de rango", "GET", "/v1/sedes/cercana?lat=200&lng=-64.18", 400, "PARAMETRO_INVALIDO");

console.log(fallas === 0 ? "\nTODO CUMPLE EL CONTRATO" : `\n${fallas} FALLA(S): la API no cumple el contrato`);
process.exit(fallas === 0 ? 0 : 1);
