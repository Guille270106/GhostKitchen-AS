import http from 'k6/http';
import { fail } from 'k6';
import exec from 'k6/execution';
import { Counter } from 'k6/metrics';

const gateway = (__ENV.GATEWAY || 'http://gateway:8080/api/v1').replace(/\/+$/, '');
const usuarios = (__ENV.USUARIOS || 'http://usuarios:8081').replace(/\/+$/, '');
const capacidad = 20;
const participantes = 200;

const aceptadas = new Counter('carritos_aceptados');
const sinStock = new Counter('carritos_sin_stock');
const inesperadas = new Counter('respuestas_inesperadas');
http.setResponseCallback(http.expectedStatuses(200, 201));

export const options = {
  scenarios: {
    ultimo_plato: {
      executor: 'constant-arrival-rate',
      rate: 20,
      timeUnit: '1s',
      duration: '10s',
      preAllocatedVUs: participantes,
      gracefulStop: '10s',
    },
  },
  summaryTrendStats: ['med', 'p(95)', 'p(99)', 'max'],
  thresholds: {
    'http_req_duration{fase:carga}': ['p(95)>=0'],
    'http_reqs{fase:carga}': ['count==200'],
    dropped_iterations: ['count==0'],
    carritos_aceptados: ['count==20'],
    carritos_sin_stock: ['count==180'],
    respuestas_inesperadas: ['count==0'],
  },
  setupTimeout: '5m',
};

function pedir(metodo, url, cuerpo, token, params = {}) {
  const headers = {};
  if (token) headers.Authorization = `Bearer ${token}`;
  if (cuerpo !== null) headers['Content-Type'] = 'application/json';
  return http.request(metodo, url, cuerpo === null ? null : JSON.stringify(cuerpo), {
    headers, timeout: '5s', ...params,
  });
}

function exigir(respuesta, estado, operacion) {
  if (respuesta.status !== estado) {
    fail(`${operacion}: HTTP ${respuesta.status}, se esperaba ${estado}`);
  }
  return respuesta.json();
}

export function setup() {
  if (__ENV.ENTORNO_DESCARTABLE !== '1' || !__ENV.ADMIN_PASSWORD) {
    fail('Usá un entorno descartable y configurá ENTORNO_DESCARTABLE=1 y ADMIN_PASSWORD');
  }
  const sufijo = Date.now().toString(36);
  const password = `v19-prueba-${sufijo}`;
  const emailCocina = `cocina-v19-${sufijo}@example.test`;
  const admin = exigir(pedir('POST', `${gateway}/auth/login`, {
    email: 'admin.cerro@ghostkitchen.dev', password: __ENV.ADMIN_PASSWORD,
  }), 200, 'login admin');
  exigir(pedir('POST', `${gateway}/admin/cuentas-cocina`, {
    nombre: 'Cocina V19', email: emailCocina, password, cocinaId: 'cerro',
  }, admin.token), 201, 'crear cuenta Cocina');
  const cocina = exigir(pedir('POST', `${gateway}/auth/login`, {
    email: emailCocina, password,
  }), 200, 'login Cocina').token;
  const platos = exigir(pedir('GET', `${gateway}/cocina/platos`, null, cocina), 200, 'listar platos');
  const plato = platos.items.find((item) => item.activo && /^p(?:1[7-9]|2[0-9]|3[0-2])$/.test(item.id));
  if (!plato) fail('No se encontró un plato activo del seed de Cerro');

  const ingrediente = exigir(pedir('POST', `${gateway}/cocina/ingredientes`, {
    nombre: `Stock V19 ${sufijo}`, unidad: 'unidades', stockInicial: capacidad,
  }, cocina), 201, 'crear ingrediente');
  exigir(pedir('PUT', `${gateway}/cocina/platos/${plato.id}/receta`, {
    items: [{ ingredienteId: ingrediente.id, cantidad: 1 }],
  }, cocina), 200, 'cargar receta');

  // Los clientes se preparan antes de medir y por la red interna para evitar
  // el límite del gateway en login y registro. La carga sí pasa por el gateway.
  const tokens = Array.from({ length: participantes }, (_, i) => {
    const email = `cliente-v19-${sufijo}-${i}@example.test`;
    exigir(pedir('POST', `${usuarios}/auth/registro`, {
      nombre: 'Cliente V19', email, password, direccion: 'Obispo Trejo 323',
    }), 201, 'registrar cliente');
    return exigir(pedir('POST', `${usuarios}/auth/login`, { email, password }), 200, 'login cliente').token;
  });
  return { tokens, cocina, platoId: plato.id, ingredienteId: ingrediente.id };
}

export default function ({ tokens, platoId }) {
  const i = exec.scenario.iterationInTest;
  // El ejecutor puede programar una iteración en el borde de los 10 segundos.
  if (i >= participantes) return;
  const respuesta = pedir('POST', `${gateway}/carrito/items`, { platoId, cantidad: 1 }, tokens[i], {
    tags: { fase: 'carga', name: 'POST /carrito/items' },
    responseCallback: http.expectedStatuses(200, 409),
  });
  let datos = {};
  try { datos = respuesta.json() || {}; } catch { /* se registra como respuesta inesperada */ }
  const aceptada = respuesta.status === 200 && datos.reservaId &&
    datos.items?.length === 1 && datos.items[0].platoId === platoId && datos.items[0].cantidad === 1;
  const rechazada = respuesta.status === 409 && datos.code === 'SIN_STOCK';
  aceptadas.add(aceptada ? 1 : 0);
  sinStock.add(rechazada ? 1 : 0);
  inesperadas.add(!aceptada && !rechazada ? 1 : 0, {
    estado: String(respuesta.status), codigo: datos.code || 'SIN_CODIGO',
  });
}

export function teardown({ cocina, ingredienteId }) {
  const stock = exigir(pedir('GET', `${gateway}/cocina/ingredientes/${ingredienteId}`, null, cocina),
    200, 'verificar stock después de la carga');
  const exceso = Math.max(0, stock.reservado - capacidad);
  console.log(`Stock final: total=${stock.total}, reservado=${stock.reservado}, disponible=${stock.disponible}; exceso=${exceso}`);
  if (stock.total !== capacidad || stock.reservado !== capacidad || stock.disponible !== 0) {
    fail('El stock persistido no coincide con las 20 reservas esperadas');
  }
}
