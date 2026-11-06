// Escenario 1 del ADR-013: la API publicada (Búsqueda + Solr), sin pasar por el gateway.
// Sube de 10 a 100 usuarios en 2 minutos y sostiene 100 usuarios durante 3 minutos.
//
// Con `docker compose up` corriendo, desde la raíz del repo (PowerShell):
//   docker run --rm -i --network ghostkitchen-as_default -v "${PWD}/tests/carga:/scripts" grafana/k6:0.54.0 run /scripts/api-publica.js
//
// Los umbrales salen del SPEC: buscar < 300 ms en el 95 % de los casos y menos de 1 % de errores.
import http from 'k6/http';
import { check, sleep } from 'k6';

const BASE = __ENV.BASE || 'http://busqueda:8083/v1';

export const options = {
  stages: [
    { duration: '2m', target: 100 },
    { duration: '3m', target: 100 },
    { duration: '30s', target: 0 },
  ],
  thresholds: {
    'http_req_duration{operacion:buscar}': ['p(95)<300'],
    'http_req_duration{operacion:cercana}': ['p(95)<300'],
    'http_req_duration{operacion:proximidad}': ['p(95)<300'],
    http_req_failed: ['rate<0.01'],
  },
};

const sedes = ['nueva-cordoba', 'cerro', 'villa-allende', 'barrio-jardin'];
const textos = ['', 'hamburguesa', 'sushi', 'pollo', 'cafe', 'milanesa', 'helado'];
// Puntos dentro del área de cobertura (Córdoba), para que la mayoría tenga sede cercana.
const puntos = [[-31.4262, -64.1883], [-31.3682, -64.2318], [-31.2958, -64.2951], [-31.4531, -64.1824], [-31.40, -64.20]];

function alAzar(lista) { return lista[Math.floor(Math.random() * lista.length)]; }

export default function () {
  const sede = alAzar(sedes);
  const q = alAzar(textos);
  const [lat, lng] = alAzar(puntos);
  const corr = { 'X-Correlation-Id': `k6-${__VU}-${__ITER}` };

  const r1 = http.get(`${BASE}/sedes/${sede}/platos?q=${encodeURIComponent(q)}&pageSize=10`,
    { headers: corr, tags: { operacion: 'buscar' } });
  check(r1, { 'buscar 200': (r) => r.status === 200 });

  const r2 = http.get(`${BASE}/sedes/cercana?lat=${lat}&lng=${lng}`,
    { headers: corr, tags: { operacion: 'cercana' } });
  check(r2, { 'cercana 200': (r) => r.status === 200 });

  const cuerpo = JSON.stringify({
    ubicacionCliente: { lat, lng }, ubicacionSede: { lat: -31.40, lng: -64.20 }, radioKm: 5, incluirDarkKitchen: true,
  });
  const r3 = http.post(`${BASE}/proximidad`, cuerpo,
    { headers: Object.assign({ 'Content-Type': 'application/json' }, corr), tags: { operacion: 'proximidad' } });
  check(r3, { 'proximidad 200': (r) => r.status === 200 });

  sleep(1);
}
