# SPEC: alcance y requisitos de GhostKitchen

> **Versión:** Entrega 1 (9/10/2026). **Forma de trabajo:** Scrum, con la cátedra como Product Owner.
> Acá definimos qué tiene que hacer el sistema. Como trabajamos con Scrum, lo escribimos como **historias de usuario (HU)** con criterios de aceptación, más **reglas de negocio (RN)** numeradas, para poder nombrarlas en el backlog, en los tests y en los ADR. Cómo está armado el sistema está en [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## 1. Problema y objetivo

Una **cocina fantasma** (*dark kitchen*) es una cocina sin salón que vende solo por delivery. Desde una misma cocina venden varios restaurantes: marcas que usan la cocina, su personal y sus ingredientes. Todas esas marcas **comparten el mismo stock**.

El problema no es cargar datos, sino **la competencia por ese stock compartido**: varios clientes pueden querer los últimos ingredientes al mismo tiempo, las reservas vencen, y cada cambio de estado del pedido afecta al stock y a lo que ve el cliente.

**Objetivo:** que un cliente pueda pedir platos de varios restaurantes de una misma cocina en **un solo pedido y un solo envío**, sin que nunca se venda un plato si no hay ingredientes para prepararlo, aunque muchos clientes pidan al mismo tiempo.

**Acción principal:** *agregar un plato al carrito*, que reserva todos sus ingredientes por 10 minutos (todos o ninguno), y *confirmar el pedido*, que descuenta esa reserva del stock sin descontar de más ni de menos.

## 2. Cocinas y restaurantes

El sistema tiene **4 cocinas**, una por localidad. Cada una tiene como **máximo 4 restaurantes**.

| Cocina | Localidad | Restaurantes |
| --- | --- | --- |
| Cocina Nueva Córdoba | Nueva Córdoba | Mostaza · Sushi Club · Kentucky · Grido |
| Cocina Cerro | Cerro de las Rosas | Johnny B. Good · Il Gatto · Green Eat · Havanna |
| Cocina Villa Allende | Villa Allende | Big Pons · KFC · El Club de la Milanesa · Dunkin' Donuts |
| Cocina Barrio Jardín | Barrio Jardín | Dean & Dennys · Sushi Pop · La Cabrera · Café Martínez |

> Los nombres son de restaurantes reales, usados como ejemplo en un trabajo académico. Los platos, precios y stock son inventados.

## 3. Glosario

| Palabra | Qué significa |
| --- | --- |
| **Localidad** | Zona donde está una cocina. El admin maneja las cocinas de su localidad. El cliente **no** tiene localidad: elige la cocina que quiere. |
| **Sede** | Nombre que usamos para la cocina en el contrato que publicamos para otros grupos. Es lo mismo que una cocina. |
| **Cocina** | Lugar que prepara los pedidos y es dueño del stock. También es el nombre del **rol** de la cuenta que trabaja los pedidos de esa cocina. |
| **Restaurante** | Marca dentro de una cocina. No tiene stock propio, ni cuenta, ni estados propios en el pedido. |
| **Plato** | Lo que se vende. Es de un restaurante y tiene nombre, descripción, categoría, precio, etiquetas y si está activo. |
| **Receta** | Qué ingredientes y cuántas unidades lleva **una** unidad de un plato. |
| **Stock total** | Unidades que hay de un ingrediente en la cocina. |
| **Stock reservado** | Unidades apartadas por carritos que todavía no se confirmaron. |
| **Stock disponible** | `total − reservado`. Es lo que se puede reservar. |
| **Carrito** | Los platos que el cliente eligió y todavía no confirmó, con sus ingredientes reservados. |
| **Reserva** | Los ingredientes apartados para un carrito. Vence a los 10 minutos. |
| **Descuento** | Cuando se confirma, la reserva se descuenta del stock total. |
| **Devolución** | Cuando se cancela un pedido, lo descontado vuelve al stock. |
| **Merma** | Ingrediente que se perdió o venció; baja el stock total. |
| **Pedido** | Carrito confirmado. Tiene **un solo estado** y un historial de cambios. |

## 4. Roles

| Rol | Quién es | Cómo se crea |
| --- | --- | --- |
| **Cliente** | Persona que compra | Se registra solo |
| **Admin de cocina** | Maneja las cocinas de su localidad | Se carga al iniciar el sistema (no se puede registrar solo) |
| **Cocina** | Cuenta del personal que prepara los pedidos de una cocina | La crea el admin y la asocia a una cocina de su localidad |
| **Grupo consumidor** | Sistema de otro grupo | Usa lo que publicamos con una API key ([contrato](docs/contracts/README.md)) |

## 5. Alcance

**Incluye:**
- registro e inicio de sesión por rol;
- manejo de cocinas, restaurantes, platos, recetas, ingredientes y stock (con reposiciones y mermas);
- búsqueda de platos con filtros, orden y páginas, y sugerencia de la cocina más cercana;
- carrito con reserva de ingredientes por 10 minutos;
- confirmación del pedido con descuento de stock, y devolución si se cancela o se rechaza;
- tablero de la cocina para aceptar, rechazar y avanzar pedidos;
- historial de estados y mails al cliente;
- publicar algo para otro grupo y usar lo que publica otro grupo.

**Fuera de alcance:**
- cobro real (el pago se simula);
- repartidores y seguimiento por GPS (la cocina marca "En camino" y el cliente confirma "Entregado");
- dividir el pedido por restaurante con estados propios;
- facturación y costos de los ingredientes;
- cupones, promociones y reseñas;
- apps móviles.

## 6. Reglas de negocio

| # | Regla |
| --- | --- |
| **RN01** | El stock de ingredientes es de la **cocina** y lo comparten todos sus restaurantes. |
| **RN02** | Un plato solo se vende si hay stock disponible de **todos** los ingredientes de su receta. |
| **RN03** | Stock disponible = total − reservado. **Nunca puede quedar negativo**, aunque muchos reserven a la vez. |
| **RN04** | Agregar un plato o aumentar su cantidad reserva **todos los ingredientes o ninguno**. |
| **RN05** | Si dos clientes quieren la última unidad a la vez, **solo uno** la consigue. |
| **RN06** | Un cliente tiene **un solo carrito activo**, con platos de **una sola cocina** (pueden ser de distintos restaurantes de esa cocina). |
| **RN07** | La reserva dura **10 minutos desde el primer plato**. Agregar más platos no reinicia el tiempo. |
| **RN08** | Sacar un plato, bajar una cantidad o vaciar el carrito **libera** esos ingredientes. Al vencer los 10 minutos se libera todo y el carrito queda vencido. |
| **RN09** | Solo se puede confirmar si la reserva **sigue vigente**, y eso lo controla Inventario en el momento. Si confirmar y vencer pasan al mismo tiempo, **solo uno de los dos** tiene efecto. |
| **RN10** | Al confirmar, la reserva se **descuenta** del stock (bajan el total y lo reservado). Si un paso posterior falla, se **devuelve** lo descontado. Si un paso no contesta a tiempo, **primero se pregunta si se hizo** antes de repetirlo o devolverlo. |
| **RN11** | **Nada se hace dos veces** por un reintento, un doble clic o un mensaje repetido: no hay reservas, descuentos, devoluciones ni pedidos duplicados. Una devolución que ya se hizo no se vuelve a hacer. |
| **RN12** | El pedido es **uno solo**: la cocina lo acepta, lo prepara y lo despacha completo. Los platos se **muestran** agrupados por restaurante, pero los restaurantes no tienen estados propios. |
| **RN13** | El pedido solo pasa por estos estados, en este orden: RECIBIDO → ACEPTADO → EN_PREPARACION → LISTO → EN_CAMINO → ENTREGADO. La cocina avanza hasta EN_CAMINO y el cliente confirma ENTREGADO. Cualquier otro salto se rechaza. |
| **RN14** | El pedido pasa a **CANCELADO** solo desde RECIBIDO, ya sea porque lo cancela el cliente o porque lo rechaza la cocina (no existe un estado "rechazado"). En los dos casos **vuelven los ingredientes** al stock. |
| **RN15** | El precio de cada plato se guarda **al agregarlo al carrito** y es el que queda en el pedido, aunque después cambie en el catálogo. |
| **RN16** | Cada cambio de estado queda en un **historial**: estado anterior, estado nuevo, quién lo hizo y cuándo. |
| **RN17** | Cada cambio de estado manda **un mail** al cliente. **No se pierde ninguno:** si falla el correo, el pedido sigue igual y el mail se reintenta. Normalmente llega una sola vez; solo si el servicio de mails se cae en el instante exacto del envío, ese mail puede llegar **repetido** (preferimos un mail repetido a uno perdido). |
| **RN18** | La receta solo usa **ingredientes de la misma cocina** del plato, en unidades enteras mayores a 0. Un plato **sin receta no se publica**. |
| **RN19** | Cocinas, restaurantes y platos **no se borran, se desactivan**. Lo desactivado no aparece en la búsqueda ni se puede agregar al carrito, pero los pedidos que ya lo tenían no cambian. |
| **RN20** | Una cocina tiene como máximo **4 restaurantes**, sin repetir. |
| **RN21** | La disponibilidad que muestra la búsqueda es **orientativa** (puede tener hasta 5 s de atraso). El control real se hace al reservar. |
| **RN22** | El cliente **no tiene localidad**: elige la cocina desde la que pide y puede cambiarla cuando quiera. El admin solo maneja cocinas de su localidad. |
| **RN23** | El acceso se controla por **rol, cocina y localidad del admin** (ver la sección 9). |
| **RN24** | Una cocina **cubre hasta 25 km** en línea recta desde su ubicación. Si ninguna cocina activa está a menos de 25 km de una ubicación, la consulta de cocina más cercana responde que **no hay cobertura**. Al cliente se le siguen mostrando las 4 cocinas para que elija una a mano (RN22). |

### Qué pasa al agregar un plato al carrito

| Situación | Qué hace el sistema |
| --- | --- |
| Alcanzan todos los ingredientes y el plato es de la misma cocina que el carrito | Reserva los ingredientes y agrega el plato. Si es el primero, empieza a correr el tiempo de 10 min |
| Algún ingrediente no alcanza | No reserva nada y muestra "Sin stock" |
| El plato es de otra cocina | No reserva nada y explica que el carrito admite una sola cocina |
| La reserva del carrito ya venció | Libera lo reservado, marca el carrito como vencido y ofrece empezar uno nuevo |
| El plato está desactivado | No reserva nada y avisa que no está disponible |

## 7. Estados

### 7.1 Carrito

| Estado | Qué significa | Puede pasar a |
| --- | --- | --- |
| `ACTIVO` | Tiene platos y la reserva sigue vigente | `CONFIRMADO` (el cliente confirma), `VACIADO` (el cliente lo vacía), `VENCIDO` (pasaron 10 minutos) |
| `CONFIRMADO` | Se convirtió en pedido | — |
| `VACIADO` | El cliente lo descartó; se liberaron los ingredientes | — |
| `VENCIDO` | Venció la reserva; se liberaron los ingredientes | — |

### 7.2 Pedido

| Estado | Qué significa | Puede pasar a | Quién |
| --- | --- | --- | --- |
| `RECIBIDO` | Se confirmó y se descontó el stock; espera que la cocina lo acepte | `ACEPTADO`, `CANCELADO` | Cocina / Cliente |
| `ACEPTADO` | La cocina lo tomó | `EN_PREPARACION` | Cocina |
| `EN_PREPARACION` | Se está preparando | `LISTO` | Cocina |
| `LISTO` | Terminado | `EN_CAMINO` | Cocina |
| `EN_CAMINO` | Se le dio al repartidor | `ENTREGADO` | Cocina |
| `ENTREGADO` | El cliente lo recibió | — | Cliente |
| `CANCELADO` | Lo canceló el cliente o lo rechazó la cocina; volvieron los ingredientes | — | Cliente / Cocina |

**Mientras se confirma:** el pedido recién se crea en RECIBIDO cuando todos los pasos de la confirmación salieron bien. Mientras tanto, Pedidos anota en qué paso va, **por separado del estado del pedido**. Si la respuesta tarda, al cliente le mostramos "estamos confirmando tu pedido" y no le pedimos que vuelva a confirmar.

## 8. Historias de usuario

**Prioridad:** **M** = imprescindible, **S** = importante. **Cuándo:** **E2** = Entrega 2 (23/10), **P** = presentación (11 o 13/11).

### 8.1 Cliente

**HU01 · Registrarme** (M · P)
Como cliente quiero registrarme con nombre, correo, contraseña y dirección para poder hacer pedidos.
- [ ] No se crea la cuenta si falta un dato o si el correo ya existe (sin mostrar datos de la otra cuenta).
- [ ] La contraseña se guarda encriptada y nunca se devuelve.
- [ ] No se pide localidad (RN22).

**HU02 · Iniciar y cerrar sesión** (M · E2)
- [ ] Con datos correctos recibo un token que vence; cada rol llega a su pantalla (cliente al inicio, cocina a su tablero, admin a su panel).
- [ ] Con datos incorrectos veo un error que no dice qué dato falló.
- [ ] Si entro a una pantalla de otro rol, veo "acceso denegado".

**HU03 · Elegir la cocina** (M · P)
Como cliente quiero elegir desde qué cocina pedir, y ver sus restaurantes.
- [ ] Escribo mi zona y el sistema me sugiere la cocina más cercana; también puedo elegir otra de las 4.
- [ ] Solo aparecen cocinas y restaurantes activos.
- [ ] La elección queda guardada para la próxima vez, y la puedo cambiar cuando quiera.

**HU04 · Buscar platos** (M · P)
Como cliente quiero buscar platos por texto y filtrarlos.
- [ ] Solo aparecen platos activos de la cocina elegida.
- [ ] Puedo filtrar por restaurante, categoría, etiqueta (vegano, sin TACC…), precio máximo y disponibilidad.
- [ ] Puedo ordenar por relevancia o por precio, y los resultados vienen en páginas.
- [ ] Un cambio en un plato o en su stock aparece en 5 segundos como máximo (RN21).

**HU05 · Ver el detalle de un plato** (M · P)
- [ ] Veo el restaurante, la descripción, el precio y si está disponible.

**HU06 · Agregar un plato al carrito** (M · E2)
Como cliente quiero agregar un plato al carrito si hay ingredientes.
- [ ] Si alcanzan los ingredientes, el plato entra y se reservan todos (RN04).
- [ ] Si no alcanzan, veo "Sin stock" y el carrito no cambia.
- [ ] Si el carrito tiene platos de otra cocina, se rechaza con un mensaje claro (RN06).
- [ ] Si es el primer plato, empiezan a correr los 10 minutos (RN07).
- [ ] Si dos clientes agregan el último plato a la vez, solo uno lo consigue (RN05).
- [ ] Repetir la misma solicitud no reserva dos veces (RN11).

**HU07 · Cambiar el carrito** (M · E2)
- [ ] Subir una cantidad funciona igual que HU06: se reserva lo que falta, todo o nada.
- [ ] Bajar una cantidad, sacar un plato o vaciar el carrito libera esos ingredientes al momento (RN08).

**HU08 · Ver el tiempo que queda** (M · E2)
- [ ] El carrito muestra la cuenta regresiva.
- [ ] Al vencer, el carrito pasa a VENCIDO, se liberan los ingredientes y se me avisa.

**HU09 · Confirmar el pedido** (M · E2)
Como cliente quiero confirmar mi carrito antes de que venza.
- [ ] Si la reserva sigue vigente, se descuenta el stock y se crea **un solo pedido** en RECIBIDO, con los platos de todos los restaurantes (RN09, RN10).
- [ ] Si la reserva venció, se me avisa y no se crea el pedido.
- [ ] Un doble clic o un reintento no crea dos pedidos ni descuenta dos veces (RN11).
- [ ] Si la confirmación tarda, veo "estamos confirmando tu pedido" y no se me pide que confirme de nuevo.

**HU10 · Cancelar el pedido** (S · E2)
- [ ] Solo puedo cancelar mientras está RECIBIDO (RN14).
- [ ] Al cancelar, vuelven los ingredientes al stock.
- [ ] Si la cocina lo aceptó en el mismo instante, se me avisa que ya no se puede cancelar.

**HU11 · Ver mis pedidos** (M · E2)
- [ ] Veo mis pedidos con su estado y su historial.
- [ ] Los platos aparecen agrupados por restaurante: el nombre del restaurante y abajo sus platos con cantidades.

**HU12 · Confirmar que lo recibí** (S · P)
- [ ] Cuando el pedido está EN_CAMINO, puedo marcarlo como ENTREGADO.

**HU13 · Recibir mails** (M · P)
- [ ] Recibo un mail cada vez que cambia el estado de mi pedido (RN17).
- [ ] Si el mismo aviso llega dos veces al servicio de mails (por un reintento o un mensaje repetido), se manda un solo mail.
- [ ] Solo puede llegar un mail repetido si el servicio de mails se cae justo después de mandarlo y antes de anotarlo como enviado.
- [ ] Si el correo falla, el pedido sigue igual y el mail llega cuando el correo vuelve.

### 8.2 Admin de cocina

**HU14 · Manejar cocinas** (M · P)
- [ ] Puedo crear, modificar y desactivar cocinas **de mi localidad**; las de otra localidad, no.
- [ ] De cada cocina cargo nombre, dirección, ubicación (latitud y longitud), tiempo estimado de entrega (mínimo y máximo, en minutos) y costo de envío. Ninguno puede quedar vacío y el costo de envío no puede ser negativo.

**HU15 · Crear la cuenta de una cocina** (M · P)
- [ ] Creo una cuenta con rol Cocina asociada a una cocina de mi localidad. Esa cuenta solo ve los pedidos de esa cocina.

**HU16 · Manejar restaurantes** (M · P)
- [ ] Puedo crear, modificar y desactivar restaurantes dentro de una cocina.
- [ ] Se rechaza el quinto restaurante y no se puede repetir uno en la misma cocina (RN20).

**HU17 · Manejar platos** (M · E2)
- [ ] Puedo crear, modificar y desactivar platos con nombre, descripción, categoría, precio, etiquetas y una foto opcional (RN19).

**HU18 · Cargar recetas** (M · E2)
- [ ] Le asigno a cada plato sus ingredientes y cantidades.
- [ ] Se rechaza un ingrediente de otra cocina o una cantidad menor o igual a 0 (RN18).

**HU19 · Cargar ingredientes** (M · E2)
- [ ] Cargo nombre, unidad y stock inicial en unidades enteras (no puede ser negativo).

**HU20 · Reponer stock y registrar mermas** (M · E2)
- [ ] Al reponer, sube el total y los platos que estaban sin stock vuelven a estar disponibles si alcanza.
- [ ] Una merma baja el total, pero nunca por debajo de lo reservado.

**HU21 · Ver el stock** (M · E2)
- [ ] Veo el total, lo reservado y lo disponible de cada ingrediente de mis cocinas.

**HU22 · Ver métricas** (S · P)
- [ ] Veo cuántos pedidos tuvo cada cocina de mi localidad.

### 8.3 Cocina

**HU23 · Ver los pedidos de mi cocina** (M · E2)
- [ ] Solo veo pedidos de mi cocina, con los platos y cantidades agrupados por restaurante (una bolsa por marca).

**HU24 · Aceptar o rechazar un pedido** (M · E2)
- [ ] Acepto o rechazo el pedido **completo**, solo mientras está RECIBIDO.
- [ ] Si lo rechazo, pasa a CANCELADO y vuelven los ingredientes (RN14).

**HU25 · Avanzar el pedido** (M · E2)
- [ ] ACEPTADO → EN_PREPARACION → LISTO → EN_CAMINO. Cualquier otro salto se rechaza (RN13).

**HU26 · Ver recetas** (S · P)
- [ ] Veo las recetas de los platos de mi cocina para organizar la preparación.

**HU27 · Ver el historial** (S · P)
- [ ] Veo los pedidos anteriores de mi cocina y sus cambios de estado (RN16).

### 8.4 Integración con otros grupos

**HU28 · Publicar "Cocina más cercana y platos disponibles"** (M · E2 con mock; P en la nube)
Como sistema de otro grupo quiero mandar una ubicación y saber qué cocina de GhostKitchen me queda más cerca, y qué platos se pueden pedir ahí.
- [x] El contrato está en `docs/contracts/`, con versión, operaciones, errores y ejemplos.
- [x] Hay un mock funcionando desde la Entrega 1.
- [ ] Es **solo de lectura**: desde afuera no se puede reservar, tocar el stock ni crear pedidos.
- [ ] Si ninguna cocina activa está a menos de 25 km de la ubicación enviada, responde `404 SIN_COBERTURA` (RN24).
- [ ] Cada cocina se informa con su dirección, ubicación, tiempo estimado de entrega, costo de envío y restaurantes.
- [ ] Está en una URL pública (Entrega 2).

**HU29 · Usar lo que publica el grupo proveedor** (M · P)
- [ ] Se usa dentro de la confirmación del pedido (lo definimos cuando la cátedra nos asigne el proveedor).
- [ ] Si el proveedor falla o no contesta, el cliente ve un mensaje claro y el pedido no se pierde.

## 9. Quién puede hacer qué

| Regla | Si alguien intenta lo contrario… |
| --- | --- |
| Un cliente solo ve y cambia su carrito y sus pedidos | Ver el pedido de otro cliente → 404 |
| Un admin solo maneja cocinas, restaurantes, platos, recetas, ingredientes y stock de su localidad | Cambiar un plato de otra localidad → 403 |
| Un admin solo crea cuentas Cocina para cocinas de su localidad | → 403 |
| Una cuenta Cocina solo ve y cambia los pedidos de su cocina | Aceptar un pedido de otra cocina → 404 |

## 10. Atributos de calidad

| Qué nos importa | Ejemplo | Cómo lo medimos |
| --- | --- | --- |
| Que el stock sea correcto | 200 clientes quieren el último plato en 10 s | 0 platos vendidos de más |
| Que se pueda seguir pidiendo | Se cae el correo, la búsqueda o Notificaciones | Los pedidos se siguen confirmando |
| Caída de Inventario | Inventario no contesta | No se puede reservar ni confirmar, pero se puede navegar y ver los pedidos |
| Que responda rápido | Viernes a las 21 h, con 100 usuarios a la vez | Agregar al carrito tarda menos de 500 ms y buscar menos de 300 ms (en el 95 % de los casos) |
| Que nadie espere de más | Un servicio no contesta | Ninguna pantalla espera más de 3 s |
| Que la búsqueda esté al día | Cambia un plato o su stock | Se ve en 5 s como máximo |
| Que no se pierda nada | Se cae RabbitMQ o un servicio que escucha mensajes | No se pierde ningún pedido, devolución ni mail |
| Seguridad | Una cocina quiere ver pedidos de otra | No puede; las contraseñas van encriptadas y las claves no están en el repo |
| Encontrar errores rápido | Falla la confirmación de un pedido | Encontramos la causa en menos de 10 min mirando el tablero y los logs |
| Calidad del código | Reglas del negocio | Tests unitarios que cubran al menos el 70 % |

## 11. Cuándo algo está terminado (Definition of Done)

- [ ] Cumple sus criterios de aceptación.
- [ ] Tiene tests unitarios de sus reglas.
- [ ] Entró a `develop` con un pull request revisado por otro integrante.
- [ ] Si cambió una decisión, se actualizó la documentación (ARCHITECTURE, ADR o contrato).
- [ ] No tiene contraseñas ni claves en el código.

## 12. Qué pide el enunciado y dónde está

| Lo que pide el enunciado | Dónde lo cubrimos |
| --- | --- |
| Una acción principal con restricciones, estados y consecuencias | RN04–RN14, HU06–HU10, HU24–HU25 |
| Una operación que no pueda duplicar ni perder datos | RN03, RN05, RN09, RN10, RN11 |
| Búsqueda con páginas, filtros y orden | HU04 |
| Integración con otros grupos | HU28, HU29 |
| Tests unitarios, de integración y de carga | Sección 10 y [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) |

## 13. Para validar con la cátedra

| Tema | Lo que proponemos |
| --- | --- |
| ¿Quién recibe los pedidos? | La **cocina**, que prepara todo. El restaurante es solo una marca |
| ¿Los 10 minutos se reinician al agregar platos? | No se reinician |
| ¿El cliente puede cancelar? ¿Hasta cuándo? | Sí, hasta que la cocina acepta |
| ¿Hace falta simular el pago? | No, queda fuera de alcance |
| ¿Qué publicamos y qué consumimos? | Publicamos "cocina más cercana y platos disponibles" ([contrato](docs/contracts/README.md)). Lo que consumimos lo asigna la cátedra |
