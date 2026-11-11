-- Carrito. Cada cliente tiene un solo carrito activo: una fila por cliente.
-- id cambia cuando el carrito se vacía, así la próxima reserva en Inventario es otra.
-- version sube con cada cambio y se usa igual que en pedido (control optimista).
CREATE TABLE carrito (
 cliente_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 cocina_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
 reserva_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
 vence_en DATETIME(3) NULL,
 version BIGINT UNSIGNED NOT NULL,
 actualizado_en DATETIME(3) NOT NULL,
 UNIQUE KEY uq_carrito_id (id)
) ENGINE=InnoDB;

-- Platos del carrito, con el precio copiado al agregarlos. agregado_en solo ordena la lista.
CREATE TABLE carrito_item (
 cliente_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 plato_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 nombre VARCHAR(160) NOT NULL,
 restaurante_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 precio_unitario BIGINT NOT NULL,
 cantidad BIGINT NOT NULL,
 agregado_en INT NOT NULL,
 PRIMARY KEY (cliente_id, plato_id),
 CONSTRAINT fk_carrito_item_carrito FOREIGN KEY (cliente_id) REFERENCES carrito(cliente_id),
 CONSTRAINT chk_carrito_item_cantidad CHECK (cantidad > 0 AND cantidad <= 50),
 CONSTRAINT chk_carrito_item_precio CHECK (precio_unitario >= 0)
) ENGINE=InnoDB;
