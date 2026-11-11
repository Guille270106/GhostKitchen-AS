// Cobertura de las reglas de negocio de un servicio, a partir del perfil de `go test`.
//
// El enunciado pide tests unitarios sobre las reglas de negocio "superando el porcentaje que
// represente calidad"; nosotros fijamos el 70 % (SPEC, sección 10). Se mide sobre los paquetes
// que tienen las reglas (dominio y casos de uso), no sobre todo el servicio: el arranque (main),
// la configuración, las trazas y los adaptadores a MySQL, MongoDB y RabbitMQ no tienen reglas, y
// los adaptadores ya se prueban contra la base o el broker reales en los tests de integración.
//
// Uso, desde la carpeta del servicio:
//   go test -count=1 -coverpkg=./internal/... -coverprofile=cobertura.out ./...
//   node ../../scripts/cobertura/reglas.mjs <servicio> cobertura.out
//
// -coverpkg cuenta también lo que se ejecuta desde los tests de otro paquete (por ejemplo, los
// casos de uso que se prueban a través de los handlers HTTP). Imprime el detalle por paquete y,
// en GitHub Actions, lo agrega al resumen del job. Sale con error si queda por debajo del mínimo.
import { appendFileSync, readFileSync } from "node:fs";

const MINIMO = 70;

// Dónde están las reglas de cada servicio (ADR-002). Se compara con la ruta de cada archivo.
const REGLAS = {
  usuarios: /\/internal\/(domain|services)\//,
  catalogo: /\/internal\/(domain|services)\//,
  busqueda: /\/internal\/(domain|services|texto|geo)\//,
  inventario: /\/internal\/(domain|application)\//,
  pedidos: /\/internal\/(domain|application)\//,
  // Notificaciones tiene un solo paquete: las reglas son armar el mail a partir del evento;
  // mongo.go es el registro en la base (se prueba en su test de integración).
  notificaciones: /\/internal\/notificaciones\/(evento|mailpit)\.go$/,
};

const [servicio, perfil] = process.argv.slice(2);
if (!REGLAS[servicio] || !perfil) {
  console.error(`Uso: node reglas.mjs <${Object.keys(REGLAS).join("|")}> <cobertura.out>`);
  process.exit(2);
}

// Cada línea del perfil es "archivo.go:inicio,fin sentencias veces". Con -coverpkg el mismo
// bloque aparece una vez por paquete de tests: cuenta como cubierto si alguno lo ejecutó.
const bloques = new Map();
for (const linea of readFileSync(perfil, "utf8").split("\n").slice(1)) {
  const m = linea.match(/^(.+):(\S+) (\d+) (\d+)$/);
  if (!m) continue;
  const [, archivo, rango, sentencias, veces] = m;
  if (!REGLAS[servicio].test(archivo)) continue;
  const clave = `${archivo}:${rango}`;
  const previo = bloques.get(clave);
  bloques.set(clave, { archivo, sentencias: Number(sentencias), cubierto: (previo?.cubierto ?? false) || Number(veces) > 0 });
}
if (bloques.size === 0) {
  console.error(`No hay sentencias de reglas de ${servicio} en ${perfil}: ¿se corrió con -coverpkg=./internal/...?`);
  process.exit(2);
}

const porPaquete = new Map();
for (const { archivo, sentencias, cubierto } of bloques.values()) {
  const paquete = archivo.slice(0, archivo.lastIndexOf("/"));
  const p = porPaquete.get(paquete) ?? { total: 0, cubiertas: 0 };
  p.total += sentencias;
  if (cubierto) p.cubiertas += sentencias;
  porPaquete.set(paquete, p);
}
const pct = (c, t) => (100 * c) / t;
let total = 0;
let cubiertas = 0;
const filas = [...porPaquete.entries()].sort().map(([paquete, p]) => {
  total += p.total;
  cubiertas += p.cubiertas;
  return `| \`${paquete.replace(/^.*?\/internal\//, "internal/")}\` | ${pct(p.cubiertas, p.total).toFixed(1)} % | ${p.cubiertas} de ${p.total} |`;
});
const resultado = pct(cubiertas, total);
const ok = resultado >= MINIMO;

const resumen = [
  `### Cobertura de las reglas de negocio: ${servicio}`,
  "",
  "| Paquete | Cobertura | Sentencias |",
  "| --- | --- | --- |",
  ...filas,
  `| **Total** | **${resultado.toFixed(1)} %** | ${cubiertas} de ${total} |`,
  "",
  ok ? `Supera el mínimo del ${MINIMO} %.` : `**Por debajo del mínimo del ${MINIMO} %.**`,
  "",
].join("\n");
console.log(resumen);
if (process.env.GITHUB_STEP_SUMMARY) appendFileSync(process.env.GITHUB_STEP_SUMMARY, resumen + "\n");
process.exit(ok ? 0 : 1);
