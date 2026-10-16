# Prueba de punta a punta de la capacidad publicada, a través del gateway.
# Uso (con `docker compose up --build` corriendo):
#   .\scripts\probar-api-publica.ps1 -Clave "la-clave-de-tu-.env"
param(
    [Parameter(Mandatory = $true)][string]$Clave,
    [string]$Base = "http://localhost:8080/public"
)

$cuerpoOk   = '{"ubicacionCliente":{"lat":-31.4262,"lng":-64.1883},"ubicacionSede":{"lat":-31.40,"lng":-64.20},"radioKm":5,"incluirDarkKitchen":true}'
$cuerpoMalo = '{"ubicacionCliente":{"lat":-31.4262,"lng":-64.1883},"ubicacionSede":{"lat":-31.40,"lng":-64.20},"radioKm":5,"incluirDarkKitchen":"true"}'

$casos = @(
    @{ n = "Listar sedes";                     m = "GET";  r = "/v1/sedes";                                          e = 200 },
    @{ n = "Sede mas cercana";                 m = "GET";  r = "/v1/sedes/cercana?lat=-31.333&lng=-64.255";          e = 200 },
    @{ n = "Sin cobertura (Buenos Aires)";     m = "GET";  r = "/v1/sedes/cercana?lat=-34.6037&lng=-58.3816";        e = 404 },
    @{ n = "Latitud invalida";                 m = "GET";  r = "/v1/sedes/cercana?lat=200&lng=-64.18";               e = 400 },
    @{ n = "Buscar platos";                    m = "GET";  r = "/v1/sedes/nueva-cordoba/platos?q=hamburguesa";       e = 200 },
    @{ n = "Buscar sin acentos (cafe)";        m = "GET";  r = "/v1/sedes/barrio-jardin/platos?q=CAFE";              e = 200 },
    @{ n = "Sede inexistente";                 m = "GET";  r = "/v1/sedes/rio-cuarto/platos";                        e = 404 },
    @{ n = "Detalle de plato";                 m = "GET";  r = "/v1/platos/p5";                                      e = 200 },
    @{ n = "Plato desactivado";                m = "GET";  r = "/v1/platos/p64";                                     e = 404 },
    @{ n = "Proximidad";                       m = "POST"; r = "/v1/proximidad"; b = $cuerpoOk;                      e = 200 },
    @{ n = "Proximidad con booleano en texto"; m = "POST"; r = "/v1/proximidad"; b = $cuerpoMalo;                    e = 400 },
    @{ n = "Sin API key";                      m = "GET";  r = "/v1/sedes"; sinClave = $true;                        e = 401 }
)

$fallas = 0
foreach ($c in $casos) {
    $parametros = @("-s", "-o", "NUL", "-w", "%{http_code}", "-X", $c.m)
    if (-not $c.sinClave) { $parametros += @("-H", "X-API-Key: $Clave") }
    if ($c.b) {
        $tmp = New-TemporaryFile
        Set-Content -Path $tmp -Value $c.b -NoNewline -Encoding ascii
        $parametros += @("-H", "Content-Type: application/json", "--data-binary", "@$tmp")
    }
    $codigo = [int](& curl.exe @parametros ($Base + $c.r))
    if ($c.b) { Remove-Item $tmp }
    if ($codigo -eq $c.e) {
        Write-Host ("OK    {0,-36} {1}" -f $c.n, $codigo) -ForegroundColor Green
    } else {
        Write-Host ("FALLA {0,-36} {1} (esperaba {2})" -f $c.n, $codigo, $c.e) -ForegroundColor Red
        $fallas++
    }
}
Write-Host ""
if ($fallas -eq 0) { Write-Host "Todo bien." -ForegroundColor Green } else { Write-Host "$fallas fallas." -ForegroundColor Red; exit 1 }
