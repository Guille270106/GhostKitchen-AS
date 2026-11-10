DELETE FROM carrito_operacion WHERE interna = TRUE;
ALTER TABLE carrito_operacion
 DROP INDEX carrito_operacion_pendiente,
 DROP INDEX carrito_un_cambio_pendiente,
 DROP COLUMN pendiente,
 DROP PRIMARY KEY,
 ADD PRIMARY KEY (cliente_id, clave),
 DROP COLUMN datos,
 DROP COLUMN estado,
 DROP COLUMN interna;
