import http from 'k6/http';
import { check, fail, sleep } from 'k6';
import { Counter } from 'k6/metrics';

const base = (__ENV.GATEWAY || 'http://gateway:8080/api/v1').replace(/\/+$/, '');
const sedes = ['cerro', 'nueva-cordoba', 'villa-allende', 'barrio-jardin'];
const copia1 = new Counter('respuestas_catalogo_1');
const copia2 = new Counter('respuestas_catalogo_2');
const sinCopia = new Counter('respuestas_sin_instancia');
const carga = { tags: { fase: 'carga', name: 'GET /cocinas/:id/platos' }, timeout: '5s' };

http.setResponseCallback(http.expectedStatuses(200));
export const options = {
  scenarios: {
    menu: {
      executor: 'constant-vus',
      vus: Number(__ENV.USUARIOS || '100'),
      duration: __ENV.DURACION || '3m',
      gracefulStop: '5s',
    },
  },
  summaryTrendStats: ['med', 'p(95)', 'p(99)', 'max'],
  thresholds: {
    'http_req_duration{fase:carga}': ['p(95)>=0'],
    'http_req_failed{fase:carga}': ['rate<0.01'],
    'http_reqs{fase:carga}': ['count>0'],
    checks: ['rate>0.99'],
  },
};

function menuValido(r) {
  if (r.status !== 200) return false;
  try {
    const items = r.json('items');
    return Array.isArray(items) && items.length > 0;
  } catch {
    return false;
  }
}

export function setup() {
  for (const sede of sedes) {
    const r = http.get(`${base}/cocinas/${sede}/platos`, { timeout: '5s' });
    if (!menuValido(r)) fail(`El menú de ${sede} no está listo (HTTP ${r.status})`);
  }
  console.log('INICIO_CARGA_V20');
}

export default function () {
  const sede = sedes[(__VU + __ITER) % sedes.length];
  const r = http.get(`${base}/cocinas/${sede}/platos`, carga);
  check(r, { 'menú válido': menuValido });
  const instancia = r.headers['X-Instancia'];
  copia1.add(instancia === 'catalogo-1' ? 1 : 0);
  copia2.add(instancia === 'catalogo-2' ? 1 : 0);
  sinCopia.add(instancia !== 'catalogo-1' && instancia !== 'catalogo-2' ? 1 : 0);
  sleep(1);
}
