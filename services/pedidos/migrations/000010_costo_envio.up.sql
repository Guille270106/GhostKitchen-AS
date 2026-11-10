-- Envío cobrado en cada pedido. Antes el pedido guardaba solo la suma de los platos y las
-- pantallas le sumaban la tarifa ACTUAL de la cocina: si el admin la cambiaba, cambiaba el
-- importe de los pedidos ya confirmados. Ahora la tarifa se lee al confirmar, se guarda en la
-- confirmación (para que la recuperación la use igual) y se copia al pedido.
-- Los pedidos anteriores quedan con NULL: su envío original no se guardó y no se inventa.
ALTER TABLE confirmacion ADD COLUMN costo_envio INT NULL;
ALTER TABLE pedido ADD COLUMN costo_envio INT NULL;
