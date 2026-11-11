// Prepara los datos de los recorridos por la API, sin pasar por la pantalla: una cuenta Cocina
// nueva para Cocina Cerro y un plato propio con stock, así la prueba no depende del stock de los
// datos de ejemplo y se puede repetir. Es lo mismo que hace services/pedidos/testdata/e2e.mjs.
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

// Directo al gateway (no por el frontend), así estos logins no gastan el límite por IP que usan
// los de la pantalla.
const API = process.env.GATEWAY_URL || "http://localhost:8080/api/v1";
const esperar = (ms) => new Promise((r) => setTimeout(r, ms));

// La clave del admin sale del .env de la raíz del repositorio (o de ADMIN_PASSWORD). No se imprime.
function claveAdmin() {
  if (process.env.ADMIN_PASSWORD) return process.env.ADMIN_PASSWORD;
  const repo = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
  const linea = readFileSync(join(repo, ".env"), "utf8").split(/\r?\n/).find((l) => l.startsWith("ADMIN_PASSWORD="));
  if (!linea) throw new Error("Falta ADMIN_PASSWORD en el .env");
  return linea.slice("ADMIN_PASSWORD=".length).trim();
}

async function pedir(metodo, ruta, { token, cuerpo } = {}) {
  const headers = {};
  if (token) headers.Authorization = "Bearer " + token;
  if (cuerpo !== undefined) headers["Content-Type"] = "application/json";
  // Si el gateway frena por el límite de logins (429), se espera y se reintenta.
  for (let intento = 0; ; intento++) {
    const r = await fetch(API + ruta, { method: metodo, headers, body: cuerpo === undefined ? undefined : JSON.stringify(cuerpo) });
    if (r.status === 429 && intento < 6) {
      await esperar(7000);
      continue;
    }
    const texto = await r.text();
    let json = null;
    try {
      json = JSON.parse(texto);
    } catch {
      /* sin cuerpo */
    }
    return { estado: r.status, json, texto };
  }
}

function exigir(ok, que, r) {
  if (!ok) throw new Error(`${que}: ${r.estado} ${r.texto}`);
}

export async function prepararDatos() {
  const marca = Date.now().toString(36);

  let r = await pedir("POST", "/auth/login", { cuerpo: { email: "admin.cerro@ghostkitchen.dev", password: claveAdmin() } });
  exigir(r.estado === 200, "login del admin de Cerro", r);
  const admin = r.json.token;

  const cocina = { email: `cocina.ui.${marca}@ghostkitchen.dev`, password: "clave-cocina-ui" };
  r = await pedir("POST", "/admin/cuentas-cocina", { token: admin, cuerpo: { nombre: "Cocina prueba UI", ...cocina, cocinaId: "cerro" } });
  exigir(r.estado === 201, "el admin crea la cuenta Cocina", r);
  r = await pedir("POST", "/auth/login", { cuerpo: cocina });
  exigir(r.estado === 200, "login de la cocina", r);
  const tokenCocina = r.json.token;

  r = await pedir("GET", "/cocina/platos", { token: tokenCocina });
  exigir(r.estado === 200, "la cocina ve sus platos", r);
  const lista = r.json.items ?? r.json.platos ?? r.json;
  const modelo = lista.find((p) => p.restauranteId) ?? lista[0];

  const nombrePlato = `Plato navegador ${marca}`;
  r = await pedir("POST", `/cocina/restaurantes/${modelo.restauranteId}/platos`, {
    token: tokenCocina,
    cuerpo: { nombre: nombrePlato, descripcion: "plato de la prueba de pantalla", categoria: modelo.categoria, precio: 4500, etiquetas: [] },
  });
  exigir(r.estado === 201, "la cocina crea el plato", r);
  const plato = r.json;

  r = await pedir("POST", "/cocina/ingredientes", { token: tokenCocina, cuerpo: { nombre: `Ingrediente navegador ${marca}`, unidad: "unidades", stockInicial: 10 } });
  exigir(r.estado === 201, "la cocina crea el ingrediente", r);
  const ingrediente = r.json;

  // Inventario conoce el plato por el mensaje plato.creado de Catálogo: puede tardar unos segundos.
  for (let i = 0; i < 20; i++) {
    r = await pedir("PUT", `/cocina/platos/${plato.id}/receta`, { token: tokenCocina, cuerpo: { items: [{ ingredienteId: ingrediente.id, cantidad: 1 }] } });
    if (r.estado === 200) break;
    await esperar(1000);
  }
  exigir(r.estado === 200, "la receta del plato", r);
  r = await pedir("PATCH", `/cocina/platos/${plato.id}`, { token: tokenCocina, cuerpo: { activo: true } });
  exigir(r.estado === 200, "la cocina activa el plato", r);

  return {
    marca,
    plato: { id: plato.id, nombre: nombrePlato },
    cocina,
    cliente: { nombre: "Cliente navegador", email: `cliente.ui.${marca}@mail.com`, password: "clave-cliente-ui", direccion: "Obispo Trejo 323" },
  };
}
