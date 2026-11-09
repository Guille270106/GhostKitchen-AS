param(
    [ValidateRange(12, 3600)][int]$Segundos = 180,
    [ValidateRange(1, 1000)][int]$Usuarios = 100,
    [string]$BinarioCatalogo,
    [switch]$SoloCaida
)

$ErrorActionPreference = 'Stop'
Push-Location (Join-Path $PSScriptRoot '../..')
$id = [guid]::NewGuid().ToString('N').Substring(0, 8)
$proyecto = "ghostkitchen-v20-$id"
$raiz = Join-Path $env:TEMP $proyecto
$resultados = Join-Path $raiz 'resultados'
$config = Join-Path $raiz 'compose.json'
$archivoEnv = Join-Path $raiz 'prueba.env'
New-Item -ItemType Directory -Path $resultados -Force | Out-Null

function Dc {
    docker compose -p $proyecto -f $config @args
    if ($LASTEXITCODE) { throw "Falló Docker Compose: $args" }
}

function EsperarSeed([string[]]$copias) {
    foreach ($copia in $copias) {
        $listo = $false
        for ($intento = 0; $intento -lt 60; $intento++) {
            if (Dc logs --tail 100 $copia | Select-String -Quiet 'datos iniciales verificados') {
                $listo = $true
                break
            }
            Start-Sleep -Seconds 1
        }
        if (!$listo) { throw "Catálogo no terminó de cargar el seed en $copia" }
    }
}

function LeerMongo {
    $salida = Dc exec -T mongo sh -ec 'mongosh --quiet -u root -p "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin --eval "const c=db.serverStatus().metrics.commands; print(Number(c.find.total)+Number(c.aggregate.total))"'
    return [long]($salida | Select-Object -Last 1)
}

function LeerCache([string[]]$copias) {
    $suma = @{ acierto = 0L; fallo = 0L; error = 0L }
    foreach ($copia in $copias) {
        $texto = (Dc exec -T $copia wget -qO- http://127.0.0.1:8082/metrics) -join "`n"
        foreach ($m in [regex]::Matches($texto, '(?m)^catalogo_cache_total\{resultado="(acierto|fallo|error)",tipo="menu"\}\s+([\d.eE+\-]+)\s*$')) {
            $suma[$m.Groups[1].Value] += [long][double]::Parse($m.Groups[2].Value, [Globalization.CultureInfo]::InvariantCulture)
        }
    }
    return $suma
}

function MedirCaida([string]$ip, [DateTimeOffset]$desde) {
    $lineas = Dc logs --since ($desde.ToString('o')) --no-log-prefix gateway
    $ultimo = $null
    $exitos = 0
    $errores = 0
    foreach ($linea in $lineas) {
        if (!$linea.StartsWith('{')) { continue }
        $log = $linea | ConvertFrom-Json -ErrorAction SilentlyContinue
        if ($log.ruta -notmatch '^/api/v1/cocinas/.+/platos$') { continue }
        $fecha = [DateTimeOffset]$log.time
        if ($fecha.AddSeconds(1) -lt $desde) { continue }
        if ($log.estado -ne 200) { $errores++ }
        if ($log.estado -eq 200 -and $log.instancia -eq 'catalogo-1') { $exitos++ }
        if (($log.upstream -split ',\s*') -contains "${ip}:8082" -and
            ($null -eq $ultimo -or $fecha -gt $ultimo)) { $ultimo = $fecha }
    }
    return @{
        exitos_copia_activa = $exitos
        errores = $errores
        segundos_hasta_ultimo_intento = if ($null -ne $ultimo) {
            [Math]::Round([Math]::Max(0, ($ultimo - $desde).TotalSeconds), 1)
        } else { $null }
    }
}

try {
    $binario = $null
    if ($BinarioCatalogo) { $binario = (Resolve-Path -LiteralPath $BinarioCatalogo).Path }
    [regex]::Matches((Get-Content docker-compose.yml -Raw), '\$\{([A-Z_]+):\?') |
        ForEach-Object { $_.Groups[1].Value } | Sort-Object -Unique |
        ForEach-Object { "$_=v20-local-descartable-$id" } | Set-Content -LiteralPath $archivoEnv
    $base = docker compose --env-file $archivoEnv config --format json | ConvertFrom-Json -AsHashtable
    if ($LASTEXITCODE) { throw 'No se pudo leer el Compose del proyecto' }

    $servicios = @('mongo', 'memcached', 'catalogo-1', 'catalogo-2', 'gateway', 'jaeger')
    foreach ($nombre in @($base.services.Keys)) {
        if ($nombre -notin $servicios) { $base.services.Remove($nombre) }
    }
    foreach ($servicio in $base.services.Values) {
        foreach ($campo in 'container_name', 'ports', 'restart') { $servicio.Remove($campo) }
        if ($servicio.depends_on) {
            foreach ($dependencia in @($servicio.depends_on.Keys)) {
                if ($dependencia -notin $servicios) { $servicio.depends_on.Remove($dependencia) }
            }
        }
    }
    # El gateway conserva la configuración del repo, incluso cuando se mide una copia.
    $base.services.gateway.Remove('depends_on')
    foreach ($nombre in 'catalogo-1', 'catalogo-2') {
        $servicio = $base.services[$nombre]
        $servicio.environment.Remove('RABBITMQ_URL')
        $servicio.environment.PUBLICAR_OUTBOX = 'false'
        if ($binario) {
            $servicio.Remove('build')
            $servicio.image = 'golang:1.24-alpine'
            $servicio.entrypoint = @('/prueba/catalogo')
            $servicio.volumes = @("${binario}:/prueba/catalogo:ro")
        }
    }
    foreach ($volumen in $base.volumes.Values) { $volumen.Remove('name') }
    foreach ($red in $base.networks.Values) { $red.Remove('name') }
    $base.name = $proyecto
    $base | ConvertTo-Json -Depth 80 | Set-Content -LiteralPath $config -Encoding utf8
    Dc up -d --wait mongo memcached jaeger

    $matriz = @(
        @{ nombre = 'sin-cache-una'; cache = $false; copias = 1 },
        @{ nombre = 'sin-cache-dos'; cache = $false; copias = 2 },
        @{ nombre = 'con-cache-una'; cache = $true; copias = 1 },
        @{ nombre = 'con-cache-dos'; cache = $true; copias = 2 },
        @{ nombre = 'caida-una-copia'; cache = $true; copias = 2; caida = $true }
    )
    if ($SoloCaida) { $matriz = @($matriz | Where-Object { $_.caida }) }
    $montaje = "$($PSScriptRoot):/scripts:ro"
    foreach ($caso in $matriz) {
        Write-Host "== $($caso.nombre) =="
        $base.services.'catalogo-1'.environment.CACHE_HABILITADA = $caso.cache.ToString().ToLowerInvariant()
        $base.services.'catalogo-2'.environment.CACHE_HABILITADA = $caso.cache.ToString().ToLowerInvariant()
        $base | ConvertTo-Json -Depth 80 | Set-Content -LiteralPath $config -Encoding utf8
        $copias = @('catalogo-1')
        if ($caso.copias -eq 2) { $copias += 'catalogo-2' }
        else { Dc stop catalogo-2 }
        if ($caso.cache) { Dc up -d --force-recreate --wait memcached }
        Dc up -d --wait --force-recreate @copias
        Dc up -d --wait --force-recreate gateway
        EsperarSeed $copias
        $mongoAntes = LeerMongo
        $cacheAntes = LeerCache $copias
        $contenedor = "$proyecto-k6"
        try {
            docker run -d --name $contenedor --network "${proyecto}_default" -v $montaje -v "${resultados}:/resultados" -e "USUARIOS=$Usuarios" -e "DURACION=${Segundos}s" grafana/k6:0.54.0 run --quiet "--summary-export=/resultados/$($caso.nombre).json" /scripts/menu-catalogo.js | Out-Null
            if ($LASTEXITCODE) { throw 'No se pudo iniciar k6' }
            $instante = $null
            $ip = $null
            $cacheCopiaCaida = @{ acierto = 0L; fallo = 0L; error = 0L }
            if ($caso.caida) {
                $enCarga = $false
                for ($intento = 0; $intento -lt 20; $intento++) {
                    $logs = docker logs $contenedor 2>&1
                    if ($logs -match 'INICIO_CARGA_V20') { $enCarga = $true; break }
                    if ((docker inspect -f '{{.State.Running}}' $contenedor) -ne 'true') { break }
                    Start-Sleep -Milliseconds 500
                }
                if (!$enCarga) { throw "k6 no inició la carga: $logs" }
                Start-Sleep -Milliseconds ([int]($Segundos * 500))
                $cacheCopiaCaida = LeerCache @('catalogo-2')
                $copiaId = Dc ps -q catalogo-2
                $ip = docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' $copiaId
                if ($LASTEXITCODE -or !$ip) { throw 'No se encontró la IP de la copia de prueba' }
                $instante = [DateTimeOffset]::UtcNow
                Dc kill -s SIGKILL catalogo-2
            }
            $codigo = docker wait $contenedor
            if ($LASTEXITCODE) { throw 'No se pudo esperar a k6' }
            docker logs $contenedor
            $activas = if ($caso.caida) { @('catalogo-1') } else { $copias }
            $cacheDespues = LeerCache $activas
            $delta = @{}
            foreach ($clave in $cacheDespues.Keys) {
                $delta[$clave] = $cacheDespues[$clave] + $cacheCopiaCaida[$clave] - $cacheAntes[$clave]
            }
            $mongo = (LeerMongo) - $mongoAntes
            $caida = if ($instante) { MedirCaida $ip $instante } else { $null }
            $k6 = Get-Content (Join-Path $resultados "$($caso.nombre).json") -Raw | ConvertFrom-Json
            $resumen = [ordered]@{
                escenario = $caso.nombre
                codigo_k6 = [int]$codigo
                solicitudes = $k6.metrics.'http_reqs{fase:carga}'.count
                solicitudes_por_segundo = $k6.metrics.'http_reqs{fase:carga}'.rate
                tasa_errores = $k6.metrics.'http_req_failed{fase:carga}'.value
                p95_ms = $k6.metrics.'http_req_duration{fase:carga}'.'p(95)'
                catalogo_1 = $k6.metrics.respuestas_catalogo_1.count
                catalogo_2 = $k6.metrics.respuestas_catalogo_2.count
                mongo_find_mas_aggregate = $mongo
                cache_menu = $delta
                caida = $caida
            }
            $resumen | ConvertTo-Json | Tee-Object -FilePath (Join-Path $resultados "$($caso.nombre)-resumen.json")
            if ([int]$codigo -ne 0) { throw "Fallaron los umbrales de $($caso.nombre)" }
            if ($resumen.catalogo_1 -lt 1 -or ($caso.copias -eq 2 -and $resumen.catalogo_2 -lt 1) -or
                ($caso.copias -eq 1 -and $resumen.catalogo_2 -ne 0)) {
                throw "La distribución de respuestas no coincide con las copias de $($caso.nombre)"
            }
            if ($caso.cache -and ($delta.acierto -lt 1 -or $delta.error -gt 0)) {
                throw "La caché no respondió como se esperaba en $($caso.nombre)"
            }
            if ($caso.caida -and ($caida.exitos_copia_activa -lt 1 -or $caida.errores -gt 0)) {
                throw 'La copia activa no sostuvo el menú sin errores después de la caída'
            }
        } finally {
            docker rm -f $contenedor 2>$null | Out-Null
        }
    }
} finally {
    if (Test-Path -LiteralPath $config) { Dc down --volumes }
    Write-Host "Resultados: $resultados"
    Pop-Location
}
