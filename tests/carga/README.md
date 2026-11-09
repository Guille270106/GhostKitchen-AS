# V18 · Carga de la API publicada

Con Búsqueda y Solr listos en Docker Compose, corré estos comandos desde la raíz del repo en PowerShell. Usá la red de tu proyecto si tiene otro nombre.

```powershell
$montaje = "${PWD}/tests/carga:/scripts:ro"
docker run --rm --network ghostkitchen-as_default -v $montaje -e HUMO=1 grafana/k6:0.54.0 run /scripts/api-publica.js
$resultados = Join-Path $env:TEMP "ghostkitchen-resultados-k6"
New-Item -ItemType Directory -Force $resultados | Out-Null
docker run --rm --network ghostkitchen-as_default -v $montaje -v "${resultados}:/resultados" grafana/k6:0.54.0 run --summary-export=/resultados/v18.json /scripts/api-publica.js
```

La primera ejecución comprueba las cuatro operaciones con un usuario. La segunda arranca con 10, sube a 100 en dos minutos, sostiene 100 durante tres minutos y baja a cero en 30 segundos. El resumen muestra p50, p95, p99, solicitudes por segundo y tasa de errores. El resumen completo también queda en v18.json para usarlo en el ADR-013.

El escenario apunta a Búsqueda dentro de la red interna. Cualquier respuesta distinta de HTTP 200 cuenta como error. El umbral de menos de 1 % se verifica tanto en el total como por operación. La prueba corta también comprueba que las cuatro sedes del seed estén indexadas. El límite de solicitudes del gateway se mide por separado.

## Corrida local del 9 de noviembre de 2026

Base de trabajo b25025f. k6 0.54.0, Docker con 12 CPU lógicas y 7,54 GiB de RAM. Búsqueda y Solr 9.7 con heap de 512 MiB, una copia de Catálogo con el seed y MongoDB. La corrida midió lecturas en la red interna, con la caché de Catálogo deshabilitada y sin la integración de disponibilidad de Inventario.

| Operación | Solicitudes | p50 (ms) | p95 (ms) | p99 (ms) | Errores |
| --- | ---: | ---: | ---: | ---: | ---: |
| Total | 102.692 | 2,80 | 10,11 | 18,44 | 0 |
| Buscar platos | 25.673 | 4,27 | 13,82 | 23,37 | 0 |
| Sede cercana | 25.673 | 2,22 | 7,42 | 13,95 | 0 |
| Proximidad | 25.673 | 2,16 | 6,79 | 12,89 | 0 |

La corrida alcanzó 100 usuarios virtuales y promedió 310,28 solicitudes por segundo. Cumplió los umbrales de latencia y errores. La prueba corta también pasó con 24 de 24 comprobaciones.
