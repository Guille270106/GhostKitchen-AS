# ADR-004 — Consistencia y concurrencia

- **Estado:** Aceptada e implementada. Validada con tests contra MySQL real, la prueba de carga y la jornada de fallas del 2026-11-10 (ver [Cómo lo validamos](#cómo-lo-validamos)).
- **Fecha:** 2026-11-06
- **Responsable:** Valentina Rey
- **Decisión del enunciado:** D4
- **Relacionado con:** [ADR-003](ADR-003-persistencia.md), [ADR-005](ADR-005-comunicacion-entre-servicios.md), [ADR-009](ADR-009-consumo-del-proveedor.md) y [SPEC](../../SPEC.md).

## Contexto

Varios clientes pueden reservar el último plato a la vez; una solicitud puede repetirse y Pedidos puede caer después de que Inventario descontó stock. También pueden coincidir aceptar y cancelar un pedido. Cada servicio tiene su propia base: no hay una transacción SQL que abarque ambos.

## Decisión

### Reservas todo o nada

Inventario usa MySQL con InnoDB. Para cada ingrediente se cumple `0 <= reservado <= total` y `disponible = total - reservado`. Las cantidades son enteras y positivas; una merma no puede comprometer lo reservado. Las restricciones CHECK de la base refuerzan estas reglas.

Cada carrito tiene una reserva única. Para crearla o ajustarla se abre una transacción, se bloquea la reserva existente (`SELECT ... FOR UPDATE`), se comprueba su estado y vencimiento con el reloj de la base y se agrupan los ingredientes compartidos por varios platos. Las filas se modifican en orden de ID para reducir deadlocks:

```sql
UPDATE ingrediente
   SET reservado = reservado + :cantidad
 WHERE id = :ingrediente_id
   AND cocina_id = :cocina_id
   AND total - reservado >= :cantidad;
```

Si alguna actualización no afecta una fila, se deshace toda la operación. Si todas tienen éxito, se guardan juntos el detalle reservado, el resultado de idempotencia y los eventos del outbox. No se decide a partir de una lectura de stock en memoria. La restricción única por carrito impide dos reservas iniciales simultáneas.

El primer agregado fija el vencimiento a los **10 minutos**, sin extenderlo al ajustar cantidades. Un ajuste recibe la cantidad final y aplica su diferencia, serializado con otros ajustes. Si falla, conserva completa la reserva anterior. Reservar y editar una receta coordinan la lectura de una versión coherente mediante el bloqueo de su cabecera; se bloquean recetas e ingredientes en orden estable.

La reserva conserva los ingredientes y cantidades apartados. Reducir, liberar y consumir usan ese detalle, aunque cambie la receta. Un aumento guarda también las cantidades de las unidades nuevas sin reemplazar las anteriores. Un deadlock implica rollback y, si queda presupuesto, reintento de la transacción completa con la misma clave.

### Consumo, vencimiento y devolución

| Operación | Efecto atómico en Inventario |
| --- | --- |
| Consumir | Bloquear reserva ACTIVA y exigir `vence_en > CURRENT_TIMESTAMP`; restar sus cantidades del total y de lo reservado, guardar consumo y marcar CONSUMIDA |
| Liberar | Bloquear reserva ACTIVA; restar de lo reservado y marcar LIBERADA |
| Vencer | Bloquear reserva ACTIVA vencida; liberar lo reservado, marcar VENCIDA y guardar `reserva.vencida` en outbox |
| Devolver | Bloquear consumo no devuelto; sumar al total las cantidades efectivamente descontadas, registrar movimiento y marcar DEVUELTO |

Consumir y vencer bloquean la misma reserva: solo una transición gana. El barrido cada **15 segundos** no sustituye el control de vencimiento al consumir o ajustar. Si estas operaciones detectan vencimiento, se confirma la liberación y su evento antes de responder `RESERVA_VENCIDA`. Una reserva terminal no se reactiva.

Un consumo es único por reserva y confirmación/pedido; una devolución es única por consumo. Estas restricciones impiden duplicar efectos incluso usando otra clave HTTP. Liberar una reserva consumida no devuelve ingredientes: se utiliza la devolución del consumo.

### Idempotencia

Las escrituras de Inventario y la confirmación de Pedidos reciben `Idempotency-Key`. Se persisten la clave, su ámbito (solicitante autenticado y operación), una huella de los datos normalizados y el resultado.

- Misma clave y mismos datos: devolver el resultado previo sin repetir efectos.
- Misma clave y otros datos: conflicto `409`.
- Confirmación en curso: devolver su identificador y avance, sin crear otra.

Una restricción única controla carreras sobre la clave. En operaciones locales, resultado y efecto se confirman juntos; un rollback no deja un éxito persistido. En una saga, se persisten primero identidad y progreso. Reservar/ajustar usa carrito, plato y número de cambio; consumir usa confirmación; liberar usa reserva; devolver usa la identidad estable del pedido, según ADR-005. No se genera una clave nueva por cada reintento.

Pedidos genera el identificador del pedido futuro al iniciar la confirmación. La restricción por carrito permite una sola confirmación activa y como máximo un pedido por carrito, incluso con claves distintas. Mientras se confirma, el carrito está congelado y sus modificaciones se rechazan. No se limpian registros de idempotencia con recuperación pendiente; su limpieza posterior conserva las restricciones únicas del negocio.

### Saga persistida, coordinada por Pedidos

La confirmación se guarda aparte del estado del pedido. No se mantienen transacciones SQL abiertas durante llamadas HTTP.

1. Persistir confirmación EN_CURSO, copia del carrito y precios, identificador del pedido futuro y claves de los pasos.
2. Registrar la intención de consumir y llamar a Inventario. Guardar el resultado. Si hay timeout, consultar reserva/consumo antes de decidir: **resultado desconocido no significa fracaso**.
3. **El proveedor no es un paso de la saga.** La factura del Grupo 4 no decide si el pedido existe: se anota como pendiente en la misma transacción del paso 4 y Pedidos la pide **después, en segundo plano**, con reintentos y siempre con la misma clave ([ADR-009](ADR-009-consumo-del-proveedor.md)). Si el proveedor está caído o lento, el pedido se crea igual y la factura llega cuando vuelve; por eso tampoco hay nada que compensar del lado del proveedor.
4. Con todos los pasos exitosos, guardar en una transacción de Pedidos: pedido RECIBIDO (con la dirección de entrega y el costo de envío guardados en la confirmación), ítems, historial inicial, outbox, factura pendiente, carrito CONFIRMADO y confirmación COMPLETADA.
5. Ante fallo definitivo, persistir la decisión de compensar. Si hubo consumo, devolver con su identidad estable; si solo hay reserva activa, liberarla.

Si cae Pedidos después del consumo o del commit final, el proceso de recuperación consulta y retoma el paso guardado; nunca vuelve a consumir con otra identidad. Si falla temporalmente la creación local del pedido, se reintenta esa transacción. Una decisión durable de abortar excluye la finalización: no se crea un pedido mientras se devuelve su stock.

Los trabajadores adquieren el avance mediante bloqueo breve o concesión con vencimiento y versión. Un trabajador obsoleto no puede guardar nuevos pasos; las llamadas externas siguen siendo idempotentes porque pueden completarse sin que se registre su respuesta. Se recuperan confirmaciones y compensaciones al reiniciar.

La solicitud comparte los **3 segundos totales** del ADR-005. Si sigue incierta, se muestra «Estamos confirmando tu pedido», con su identificador, y continúa en segundo plano. No se anuncia éxito ni rechazo definitivo sin evidencia. La ruta de consulta se definirá en el contrato de Pedidos. El flujo completo no queda validado hasta conocer y probar las garantías del proveedor.

### Versión del pedido

Se conserva la máquina de estados del SPEC:

```text
RECIBIDO → ACEPTADO → EN_PREPARACION → LISTO → EN_CAMINO → ENTREGADO
    └──→ CANCELADO
```

Cancelar o rechazar solo se permite desde RECIBIDO. Tras validar permisos y transición, se exige la versión esperada:

```sql
UPDATE pedido
   SET estado = :estado_nuevo, version = version + 1
 WHERE id = :pedido_id
   AND estado = :estado_anterior
   AND version = :version_esperada;
```

Si no afecta una fila, se devuelve `409` y no se guarda historial ni evento. Aceptar y cancelar con la misma versión permiten un solo ganador. Su repetición idempotente devuelve el resultado ya persistido.

Cambio, historial, idempotencia y outbox se confirman juntos. Cancelar guarda además la devolución pendiente en esa transacción: el pedido queda CANCELADO aunque Inventario no responda, y la recuperación completa una sola devolución.

### Outbox y consumidores

Inventario y Pedidos guardan cambio y evento en una transacción local. El publicador envía mensajes persistentes a colas durables y marca el evento solo después de confirmar publicación y enrutamiento. Si RabbitMQ está caído o no hay destino válido, queda pendiente.

Una caída entre publicar y marcar puede repetir el mensaje con el mismo `evento_id`: la entrega es **al menos una vez**. El consumidor guarda identificador único y efecto en una transacción y hace ACK después del commit. La secuencia se compara por fuente y agregado. Una proyección de estado completo descarta versiones antiguas; quien necesita cada transición detecta huecos y recupera el orden. Búsqueda usa las versiones persistidas de Solr definidas en ADR-005.

SMTP conserva la excepción que aceptamos en el SPEC: una caída justo después de enviar puede producir un correo repetido. No se promete entrega exactamente una vez de efectos externos.

## Alternativas que pensamos

| Alternativa | A favor | En contra | Decisión |
| --- | --- | --- | --- |
| Leer stock y escribir sin condición | Sencillo | Permite sobreventa | Descartada |
| Bloqueos en memoria | Simples en una instancia | No coordinan réplicas ni reinicios | Descartada |
| Transacción distribuida | Intenta confirmar todo junto | Acopla disponibilidad y no resuelve por sí sola al proveedor | Descartada |
| Reacciones sin coordinador | Menos coordinación central | Complica recuperar una compra | Coordina Pedidos |
| Publicar tras commit sin outbox | Menos tablas | Puede perder eventos | Descartada |

## Consecuencias

La base controla sobreventa y duplicados; las operaciones locales son atómicas y las confirmaciones pueden recuperarse. Aceptamos tablas y procesos adicionales, conflictos de versión y períodos con devoluciones o confirmaciones pendientes. No hay atomicidad global.

Se observan antigüedad del outbox, confirmaciones y compensaciones pendientes, conflictos y DLQ según ADR-011. Una caída prolongada mantiene las obligaciones pendientes; no las elimina.

## Cómo lo validamos

Con tests contra MySQL real y RabbitMQ (en Docker), la prueba de carga ([ADR-013](ADR-013-capacidad-y-costos.md)) y la jornada de fallas del 2026-11-10 ([POSTMORTEM](../POSTMORTEM.md)):

| Escenario | Resultado exigido | Cómo se comprobó |
| --- | --- | --- |
| 200 clientes reservando el último plato | Un ganador, ninguna reserva parcial y stock siempre válido | ✅ `TestV8UltimoPlato200Solicitudes` (Inventario, 1 ganador y 199 `SIN_STOCK`) y la prueba de carga (20 aceptados y 180 `SIN_STOCK` con stock para 20) |
| Falta el último ingrediente; falla un ajuste | Rollback completo y reserva anterior intacta | ✅ `TestMySQLReservaTodoONadaYErrorDurable` y `TestMySQLAjusteConservaRecetasYVencimiento` (Inventario) |
| Edición de receta tras reservar/consumir | Liberación, descuento y devolución usan cantidades guardadas | ✅ `TestMySQLAjusteConservaRecetasYVencimiento` y `TestMySQLConsumoYDevolucionIdempotentes` (Inventario) |
| Consumir y vencer simultáneamente | Una transición; ningún consumo vencido | Parcial: una reserva vencida rechaza los ajustes (`TestMySQLReservaVencidaYReferencias`). La carrera exacta entre consumir y vencer no tiene un test propio |
| Solicitudes duplicadas o misma clave con otros datos | Un efecto o conflicto, respectivamente | ✅ `TestMySQLReservaIdempotenteYCarritoUnico` y `TestMySQLConsumoYDevolucionIdempotentes` (Inventario); `TestDobleClicConLaMismaClaveEsUnSoloPedido` (Pedidos) |
| Confirmar el mismo carrito con claves distintas | Una confirmación activa y como máximo un pedido | ✅ `TestDosClavesALaVezConfirmanUnaSolaVez` (Pedidos) |
| Caídas antes/después del consumo y del commit de Pedidos | Recuperación sin pedido parcial ni doble descuento | ✅ `TestInventarioCaidoRespondeEstamosConfirmandoYDespuesSeRecupera`, `TestSiFallaGuardarElPedidoNoSeDevuelveNadaYSeReintenta` y `TestMySQLConfirmacionCompleta` (Pedidos); en la jornada de fallas el pedido se completó solo 13 s después de que volvió Inventario |
| Fallo definitivo y caída durante compensación | Ningún pedido creado; una devolución al recuperarse | ✅ `TestReservaVencidaFallaYVaciaElCarrito` e `TestInventarioCaidoLaDevolucionQuedaPendienteYSeReintentaConLaMismaClave` (Pedidos) |
| Aceptar y cancelar simultáneamente | Un cambio, historial y evento; conflicto del perdedor | ✅ `TestCancelarYAceptarALaVezGanaUnaSola` y `TestMySQLCambiosSimultaneosGanaUnoSolo` (Pedidos) |
| Cancelar con Inventario caído y reiniciar Pedidos | Cancelación durable y devolución recuperada una vez | ✅ `TestMySQLCancelarAnotaLaDevolucion` y `TestInventarioCaidoLaDevolucionQuedaPendienteYSeReintentaConLaMismaClave` (Pedidos); en la jornada de fallas el stock volvió 3 s después de que volvió Inventario |
| Caída de RabbitMQ y reentrega al consumidor | Outbox conservado y un efecto transaccional por evento | ✅ `TestSiRabbitMQNoConfirmaParaYReintenta` y `TestSiFallaLaBaseElMensajeSeReintenta` (Pedidos); en la jornada de fallas el outbox se vació 36 s después de que volvió RabbitMQ |
| Reenviar el mismo "agregar al carrito" | Se suma una sola vez | ✅ `TestReenviarElMismoAgregarNoSumaDosVeces` (Pedidos) y `TestMySQLLaMismaClaveNoSeAplicaDosVeces` (MySQL real): la `Idempotency-Key` del clic se guarda en la misma transacción que el carrito |
| Inventario reservó pero el carrito no se pudo guardar | La reserva no queda apartando ingredientes que ningún carrito tiene | ✅ `TestSiFallaGuardarElPrimerPlatoSeLiberaLaReserva` y `TestSiFallaGuardarUnCambioLaReservaVuelveALoGuardado` (Pedidos): se libera o se ajusta a lo guardado. Si la base no se puede ni leer, la reserva vence a los 10 minutos |

### Detalle: 200 clientes por el último plato (2026-11-06)

Se ejecutó `TestV8UltimoPlato200Solicitudes` contra la API real de Inventario y MySQL 8.4, en Docker, con las migraciones 000001–000004. Las 200 solicitudes POST usan claves y carritos distintos y se liberan mediante una barrera de inicio común. La receta necesita dos ingredientes; hay stock para una sola unidad del plato.

| Comprobación | Resultado observado |
| --- | --- |
| Solicitudes | 200 HTTP, concurrentes |
| Ganadores | 1 respuesta 201 y una reserva ACTIVA del mismo carrito |
| Perdedoras | 199 respuestas 409 con código SIN_STOCK |
| Errores técnicos o timeouts | 0 en la ejecución aprobada |
| Stock total | 1000 y 1; sin descuento al reservar |
| Stock reservado | 2 y 1; disponible 998 y 0 |
| Datos persistidos | Una reserva, un plato de cantidad 1 y dos detalles de ingredientes |
| Consistencia | Ninguna reserva parcial, ninguna sobre-reserva; reservado coincide con la suma del detalle |
| Idempotencia | 200 resultados finales persistidos |
| Duración del lote aprobado | 1,102 s en la ejecución local aislada |

La primera ejecución encontró un ganador, pero solo 150 rechazos SIN_STOCK y 49 respuestas técnicas o de operación en proceso: no se consideró aprobada. La contención ocurría al intentar escribir un ingrediente abundante antes de detectar que faltaba el último. Se incorporó un rechazo temprano de stock ya insuficiente mediante lectura; las escrituras siguen condicionadas atómicamente en MySQL y dentro de una transacción. No se amplió el presupuesto HTTP de 3 s ni se relajaron las condiciones de la prueba.

La suite completa de Inventario también pasó contra MySQL real después de la corrección, incluidas las pruebas anteriores de ingredientes, recetas, idempotencia, rollback, ajustes y liberación. En esa ejecución la prueba repitió el resultado 1/199/0, con duración de lote 1,101 s.

Esta prueba cubre la fase de **reserva**, con una instancia del servicio; el resto del flujo lo cubren los tests de la tabla de arriba. La duración es una observación local, no una garantía de rendimiento en otro entorno. La [guía de Inventario](../../services/inventario/README.md) contiene el comando de reproducción. El test inspecciona las filas antes de limpiar sus datos descartables y falla ante cualquier respuesta o estado final distinto del esperado.

## Referencias

- SPEC: reglas del stock, del carrito y del pedido.
- ADR-003, ADR-005 y ADR-009.
