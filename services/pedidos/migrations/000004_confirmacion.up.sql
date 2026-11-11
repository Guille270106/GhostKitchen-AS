-- Confirmación del pedido como saga: en qué paso va cada confirmación, para poder seguir
-- desde ahí si algo se corta. Guarda una copia del carrito (items) con los precios.
CREATE TABLE confirmacion (
 id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 cliente_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 cliente_email VARCHAR(254) NOT NULL,
 clave VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 carrito_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 reserva_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 cocina_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 pedido_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 items JSON NOT NULL,
 estado ENUM('EN_CURSO','COMPLETADA','FALLIDA') NOT NULL,
 paso ENUM('DESCONTAR','CREAR_PEDIDO') NOT NULL,
 motivo VARCHAR(64) NULL,
 creada_en DATETIME(3) NOT NULL,
 actualizada_en DATETIME(3) NOT NULL,
 -- La misma clave del mismo cliente es la misma confirmación (doble clic: un solo pedido).
 UNIQUE KEY uq_confirmacion_clave (cliente_id, clave),
 -- Un carrito se confirma una sola vez, aunque lleguen dos claves distintas a la vez.
 UNIQUE KEY uq_confirmacion_carrito (carrito_id),
 UNIQUE KEY uq_confirmacion_pedido (pedido_id),
 INDEX idx_confirmacion_pendientes (estado, actualizada_en)
) ENGINE=InnoDB;
