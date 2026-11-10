# Pruebas de carga

Usamos **k6** en Docker, así no hay que instalar nada. Los resultados y su análisis están en el [ADR-013](../../docs/adr/ADR-013-capacidad-y-costos.md).

## Carga de la API publicada

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

## 200 clientes por el mismo plato

Usá una instalación local **descartable**, con Gateway, Usuarios, Catálogo, Pedidos e Inventario listos y el seed de Cerro cargado. La preparación crea cuentas de prueba, un ingrediente con 20 unidades y reemplaza la receta de un plato del seed de Cerro. La limpieza se hace descartando los datos del entorno de prueba.

```powershell
$montaje = "${PWD}/tests/carga:/scripts:ro"
$env:ADMIN_PASSWORD = Read-Host "Contraseña del admin de Cerro del entorno descartable" -MaskInput
try {
    docker run --rm --network ghostkitchen-as_default -v $montaje -e ADMIN_PASSWORD -e ENTORNO_DESCARTABLE=1 grafana/k6:0.54.0 run /scripts/ultimo-plato.js
} finally {
    Remove-Item Env:ADMIN_PASSWORD
}
```

Ajustá la red al nombre de tu proyecto. Los 200 clientes se registran e inician sesión antes de medir. Durante 10 segundos, k6 inicia 20 solicitudes por segundo al endpoint de carrito del gateway. La prueba exige 20 carritos aceptados, 180 rechazos SIN_STOCK, cero respuestas inesperadas y cero iteraciones descartadas. Después consulta Inventario y verifica total 20, reservado 20 y disponible 0. Los percentiles de la operación medida aparecen con la etiqueta fase:carga.

### Corrida local del 9 de noviembre de 2026

Base de trabajo 2f37248. k6 0.54.0 en Docker, con gateway real, Usuarios, Pedidos, Inventario, una copia de Catálogo sin caché, MySQL y MongoDB descartables. La carga midió solo agregar al carrito, con llegadas escalonadas a 20 por segundo.

| Intentos | Aceptados | SIN_STOCK | Respuestas inesperadas | Reservas por encima del stock | p50 | p95 | p99 |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 200 | 20 | 180 | 0 | 0 | 19,22 ms | 34,63 ms | 42,83 ms |

Inventario terminó con total 20, reservado 20 y disponible 0. k6 no descartó iteraciones.

## Caché, balanceo y caída de una copia

Desde la raíz del repositorio, con Docker y PowerShell 7:

```powershell
& ./tests/carga/probar-catalogo.ps1
```

El runner crea un proyecto Docker aislado con volúmenes propios. Corre cinco casos secuenciales por el gateway real, cada uno con 100 usuarios virtuales durante 3 minutos. Alterna una y dos copias de Catálogo, con y sin Memcached. En el quinto caso apaga catalogo-2 a mitad de la carga. El comando imprime el directorio temporal con el JSON de k6 y el resumen de cada caso. Para comprobar el montaje en menos tiempo, se puede usar -Segundos 12 -Usuarios 10. Usá -SoloCaida para repetir únicamente el quinto caso. El parámetro opcional -BinarioCatalogo acepta un binario Linux amd64 de Catálogo compilado desde la misma versión del repo.

Cada caso espera que Catálogo termine de verificar el seed antes de medir. La caché arranca vacía y el setup de k6 valida los cuatro menús antes de iniciar la carga. Los percentiles de fase:carga excluyen ese setup. Los deltas de MongoDB (comandos find y aggregate) y de caché (aciertos, fallos y errores de menú) sí incluyen el setup. Son contadores de comandos observados durante el caso, no un conteo de lecturas físicas en disco. El conteo de la copia caída usa su última muestra anterior al apagado.

En el caso de caída, el resumen registra las respuestas de la copia activa, los errores visibles y el tiempo hasta el último intento observado a la IP de la copia caída. Ese tiempo sale del log del gateway, con resolución de un segundo. El valor nulo indica que el log no mostró un intento después del apagado. El runner elimina sus contenedores y volúmenes al terminar y conserva los resultados para el ADR-013.

### Corrida local del 9 de noviembre de 2026

Base de trabajo a769d3b. k6 0.54.0, NGINX del proyecto, MongoDB y Memcached en Docker local. Se usó un binario de Catálogo de esa base compilado para Linux. Cada caso mantuvo 100 usuarios virtuales durante tres minutos, con una pausa de un segundo entre lecturas por usuario.

| Configuración | Solicitudes | Solicitudes/s | p50 (ms) | p95 (ms) | p99 (ms) | Errores | find + aggregate | Aciertos de menú |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Sin caché, una copia | 17.900 | 99,31 | 4,02 | 13,44 | 24,76 | 0 | 35.842 | 0 |
| Sin caché, dos copias | 17.900 | 99,27 | 3,95 | 14,05 | 31,69 | 0 | 35.843 | 0 |
| Con caché, una copia | 17.900 | 99,39 | 3,62 | 11,66 | 23,67 | 0 | 67 | 17.879 |
| Con caché, dos copias | 17.900 | 99,41 | 3,38 | 12,49 | 23,41 | 0 | 82 | 17.872 |
| Con caché, caída de una de dos copias | 17.936 | 99,08 | 3,25 | 10,32 | 20,79 | 0 | 66 | 17.870 |

En ambos casos estables con dos copias, el reparto fue 8.950 y 8.950. Tras apagar catalogo-2, el gateway registró 8.815 respuestas exitosas de catalogo-1 y cero errores visibles. El último intento observado hacia la IP apagada apareció 83 segundos después de iniciar el apagado. Hubo solicitudes de hasta aproximadamente 1 segundo durante el caso de caída. Se repitió ese caso completo después de corregir el campo estado del resumen de logs.
