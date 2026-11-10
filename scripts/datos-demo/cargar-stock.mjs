// Datos para la demo: deja todos los platos con receta y stock, salvo dos "sin stock".
//
// Usa la API de Inventario (no SQL directo), así cada cambio publica disponibilidad.cambiada y
// Búsqueda y el menú se enteran solos. Por cada cocina:
//   - crea ingredientes compartidos: "Insumos de <categoría>" (200 porciones) y "Envases" (500);
//   - a cada plato activo SIN receta le carga una: 1 porción de su categoría + 1 envase;
//   - a los platos de SIN_STOCK les pone además un ingrediente con stock 0.
// No toca las recetas que ya existen. Se puede correr varias veces: reusa los ingredientes por
// nombre y repone los insumos que hayan bajado de 50.
//
// Corre adentro de la red de Docker (Inventario y Catálogo no se publican afuera). Desde la raíz:
//   docker run --rm --network ghostkitchen-git_default -v "${PWD}/scripts/datos-demo:/s:ro" node:22-alpine node /s/cargar-stock.mjs
// (la red es <carpeta del proyecto>_default; se ve con `docker network ls`).

const CATALOGO = process.env.CATALOGO_URL || "http://catalogo:8082";
const INVENTARIO = process.env.INVENTARIO_URL || "http://inventario:8084";
// Los dos platos que quedan sin stock (se pueden cambiar con SIN_STOCK=p11,p57).
const SIN_STOCK = (process.env.SIN_STOCK || "p11,p57").split(",").map((s) => s.trim()).filter(Boolean);
const PORCIONES = 200;
const ENVASES = 500;
const MINIMO = 50;

async function pedir(url, { metodo = "GET", cuerpo, cocina } = {}) {
  const headers = { Accept: "application/json" };
  if (cocina) Object.assign(headers, { "X-Usuario-Id": "datos-demo", "X-Rol": "cocina", "X-Cocina-Id": cocina });
  if (cuerpo) headers["Content-Type"] = "application/json";
  const r = await fetch(url, { method: metodo, headers, body: cuerpo ? JSON.stringify(cuerpo) : undefined });
  const texto = await r.text();
  const json = texto ? JSON.parse(texto) : null;
  if (!r.ok) {
    const error = new Error(`${metodo} ${url} -> ${r.status} ${json?.code || ""} ${json?.detail || ""}`);
    error.status = r.status;
    throw error;
  }
  return json;
}

// 1. Todos los platos de Catálogo, de a páginas.
const platos = [];
for (let page = 1; ; page++) {
  const r = await pedir(`${CATALOGO}/internal/platos?page=${page}&pageSize=500`);
  platos.push(...r.items);
  if (r.items.length < 500) break;
}
const activos = platos.filter((p) => p.activo);
const cocinas = [...new Set(activos.map((p) => p.cocinaId))].sort();
console.log(`${activos.length} platos activos en ${cocinas.length} cocinas`);

let recetasNuevas = 0;
for (const cocina of cocinas) {
  const deLaCocina = activos.filter((p) => p.cocinaId === cocina);
  const ingrediente = `${INVENTARIO}/cocina/ingredientes`;

  // 2. Ingredientes que ya tiene la cocina, por nombre.
  const existentes = new Map((await pedir(ingrediente, { cocina })).items.map((i) => [i.nombre, i]));
  async function asegurar(nombre, unidad, stock) {
    const ya = existentes.get(nombre);
    if (!ya) {
      const creado = await pedir(ingrediente, { metodo: "POST", cocina, cuerpo: { nombre, unidad, stockInicial: stock } });
      existentes.set(nombre, creado);
      return creado.id;
    }
    // Si se usó en otras demos y quedó poco, se repone (los de stock 0 se dejan en 0).
    if (stock > 0 && ya.disponible < MINIMO) {
      await pedir(`${ingrediente}/${ya.id}/reposiciones`, { metodo: "POST", cocina, cuerpo: { cantidad: stock - ya.disponible } });
    }
    return ya.id;
  }

  // 3. Recetas que ya existen: no se tocan (salvo los platos que tienen que quedar sin stock).
  const conReceta = new Set((await pedir(`${INVENTARIO}/cocina/recetas`, { cocina })).items.map((r) => r.platoId));
  const envases = await asegurar("Envases", "unidades", ENVASES);

  for (const plato of deLaCocina) {
    const sinStock = SIN_STOCK.includes(plato.id);
    if (conReceta.has(plato.id) && !sinStock) {
      // Ya tiene receta: solo se reponen sus insumos de la categoría, si existen.
      if (existentes.has(`Insumos de ${plato.categoria}`)) await asegurar(`Insumos de ${plato.categoria}`, "porciones", PORCIONES);
      continue;
    }
    const items = [
      { ingredienteId: await asegurar(`Insumos de ${plato.categoria}`, "porciones", PORCIONES), cantidad: 1 },
      { ingredienteId: envases, cantidad: 1 },
    ];
    if (sinStock) {
      items.push({ ingredienteId: await asegurar(`Ingrediente agotado: ${plato.nombre}`.slice(0, 120), "unidades", 0), cantidad: 1 });
    }
    try {
      await pedir(`${INVENTARIO}/cocina/platos/${encodeURIComponent(plato.id)}/receta`, { metodo: "PUT", cocina, cuerpo: { items } });
      recetasNuevas++;
      if (sinStock) console.log(`  ${cocina}: ${plato.nombre} (${plato.id}) queda SIN STOCK`);
    } catch (e) {
      // Un plato que Inventario todavía no conoce (no le llegó plato.creado) se saltea.
      console.log(`  ${cocina}: no se pudo cargar la receta de ${plato.id}: ${e.message}`);
    }
  }
}

// 4. Cómo quedó, según Inventario.
const disponibilidad = await pedir(`${INVENTARIO}/platos/disponibilidad`);
const items = disponibilidad.items || disponibilidad;
const idsActivos = new Set(activos.map((p) => p.id));
const deActivos = items.filter((d) => idsActivos.has(d.plato_id));
const sin = deActivos.filter((d) => !d.disponible);
console.log(`\nRecetas cargadas: ${recetasNuevas}`);
console.log(`Disponibles: ${deActivos.length - sin.length} de ${deActivos.length}`);
console.log(`Sin stock: ${sin.map((d) => d.plato_id).join(", ") || "ninguno"}`);
