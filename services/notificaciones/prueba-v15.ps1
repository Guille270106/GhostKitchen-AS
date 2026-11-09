$ErrorActionPreference='Stop'
Push-Location $PSScriptRoot
$s=[guid]::NewGuid().ToString('N').Substring(0,8)
$r="vale-v15-rabbit-$s"; $m="vale-v15-mongo-$s"; $mail="vale-v15-mailpit-$s"
$bin=Join-Path $env:TEMP "vale-v15-worker-$s.exe"
$worker=$null
function D {docker @args; if($LASTEXITCODE){throw 'Falló Docker'}}
function Esperar($nombre,[scriptblock]$paso) {
 $ultimo=$null
 for($i=0;$i -lt 45;$i++){
  try { $valor=& $paso; if($valor){return $valor} } catch { $ultimo=$_ }
  Start-Sleep -Seconds 1
 }
 throw "No se completó $nombre. $ultimo"
}
try {
 go build -o $bin ./cmd/worker
 if($LASTEXITCODE){throw 'Falló la compilación del worker'}
 D run -d --pull never --name $m -p 127.0.0.1::27017 mongo:7 | Out-Null
 D run -d --pull never --name $r -p 127.0.0.1::5672 -p 127.0.0.1::15672 -e RABBITMQ_DEFAULT_USER=v15test -e RABBITMQ_DEFAULT_PASS=v15test rabbitmq:3-management | Out-Null
 D run -d --name $mail -p 127.0.0.1::1025 -p 127.0.0.1::8025 axllent/mailpit@sha256:aef2aacc11f075ec0fb6874275ee4a421e688e11a96ebb296134d1735c7761ce | Out-Null
 $api="http://$(D port $r 15672)"
 $mailApi="http://$(D port $mail 8025)"
 $auth=@{Authorization='Basic '+[Convert]::ToBase64String([Text.Encoding]::ASCII.GetBytes('v15test:v15test'))}
 function J($method,$path,$data){Invoke-RestMethod -Method $method -Uri "$api$path" -Headers $auth -ContentType 'application/json' -Body ($data | ConvertTo-Json -Depth 8 -Compress) -TimeoutSec 3}
 Esperar 'RabbitMQ' {Invoke-RestMethod "$api/api/overview" -Headers $auth -TimeoutSec 2} | Out-Null
 Esperar 'Mailpit' {Invoke-RestMethod "$mailApi/api/v1/messages" -TimeoutSec 2} | Out-Null
 $topologia='../../infra/rabbitmq/definiciones.json'
 Invoke-RestMethod "$api/api/definitions" -Method Post -Headers $auth -ContentType 'application/json' -Body (Get-Content $topologia -Raw) | Out-Null
 $env:MONGO_URI="mongodb://$(D port $m 27017)"
 $env:RABBITMQ_URL="amqp://v15test:v15test@$(D port $r 5672)/"
 $env:SMTP_ADDR=D port $mail 1025
 $env:MONGO_DB='notificaciones_v15_prueba'
 $worker=Start-Process $bin -WindowStyle Hidden -PassThru
 function Publicar($payload) {
  $pub=J Post '/api/exchanges/%2F/ghostkitchen.events/publish' @{routing_key='pedido.estado_cambiado';properties=@{};payload=$payload;payload_encoding='string'}
  if(!$pub.routed){throw 'El evento no llegó a RabbitMQ'}
 }
 $evento=Get-Content evento-prueba.json -Raw
 Publicar $evento
 Publicar $evento
 # El mensaje inválido va detrás de los dos duplicados. Su llegada a la DLQ
 # comprueba que el consumidor avanzó después de procesar ambos.
 Publicar '{}'
 Esperar 'el mensaje inválido en la DLQ' {
  if($worker.HasExited){throw 'El worker terminó antes de completar la prueba'}
  $q=Invoke-RestMethod "$api/api/queues/%2F/notificaciones.mailer.dlq" -Headers $auth -TimeoutSec 3
  if($q.messages_ready -eq 1){$q}
 } | Out-Null
 $buzon=Invoke-RestMethod "$mailApi/api/v1/messages"
 if($buzon.total -ne 1){throw "Mailpit recibió $($buzon.total) correos en lugar de uno"}
 $result=D exec $m mongosh --quiet notificaciones_v15_prueba --eval 'JSON.stringify(db.notificaciones.find({}, {_id:1,estado:1,intentos:1}).toArray())'
 $docs=ConvertFrom-Json -InputObject ($result -join '') -NoEnumerate
 if($docs.Count -ne 1 -or $docs[0].estado -ne 'ENVIADO' -or $docs[0].intentos -ne 1 -or $docs[0]._id -ne ($evento | ConvertFrom-Json).evento_id){throw "Registro inesperado: $result"}
 Write-Output "PASS: dos entregas válidas, un correo y un registro ENVIADO. El mensaje inválido llegó a la DLQ. $result"
} finally {
 if($worker -and !$worker.HasExited){Stop-Process -InputObject $worker; $worker.WaitForExit()}
 foreach($c in @($mail,$r,$m)){docker rm -fv $c 2>$null | Out-Null}
 if(Test-Path -LiteralPath $bin){Remove-Item -LiteralPath $bin -Force}
 Pop-Location
}

