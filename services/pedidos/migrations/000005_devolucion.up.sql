-- Devolución de ingredientes (L10): cuando un pedido se cancela (el cliente lo cancela o la
-- cocina lo rechaza), hay que devolverle a Inventario lo que se descontó al confirmar.
-- Se anota en la MISMA transacción que el cambio a CANCELADO, así no se pierde aunque
-- Inventario esté caído: un proceso en segundo plano la manda y reintenta hasta que salga.
CREATE TABLE devolucion (
 pedido_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 estado ENUM('PENDIENTE','HECHA') NOT NULL,
 intentos INT NOT NULL DEFAULT 0,
 creada_en DATETIME(3) NOT NULL,
 actualizada_en DATETIME(3) NOT NULL,
 INDEX idx_devolucion_pendientes (estado, actualizada_en),
 CONSTRAINT fk_devolucion_pedido FOREIGN KEY (pedido_id) REFERENCES pedido(id)
) ENGINE=InnoDB;
