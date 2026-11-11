-- Agregado Pedido. Un pedido es uno solo, con un estado para todo el pedido.
-- version sube con cada cambio de estado: GuardarCambio solo escribe si la versión no cambió
-- (control optimista), así dos cambios simultáneos no se pisan (ADR-004).
CREATE TABLE pedido (
 id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 cliente_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 cliente_email VARCHAR(254) NOT NULL,
 cocina_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 estado ENUM('RECIBIDO','ACEPTADO','EN_PREPARACION','LISTO','EN_CAMINO','ENTREGADO','CANCELADO') NOT NULL,
 total BIGINT NOT NULL,
 moneda CHAR(3) NOT NULL DEFAULT 'ARS',
 version BIGINT UNSIGNED NOT NULL,
 creado_en DATETIME(3) NOT NULL,
 actualizado_en DATETIME(3) NOT NULL,
 CONSTRAINT chk_pedido_total CHECK (total >= 0),
 CONSTRAINT chk_pedido_version CHECK (version >= 1),
 INDEX idx_pedido_cliente (cliente_id, creado_en),
 INDEX idx_pedido_cocina (cocina_id, estado, creado_en)
) ENGINE=InnoDB;

-- Copia de cada plato al momento de pedirlo: si cambia en Catálogo, el pedido no cambia.
CREATE TABLE pedido_item (
 pedido_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 plato_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 nombre VARCHAR(160) NOT NULL,
 restaurante_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 restaurante_nombre VARCHAR(160) NOT NULL DEFAULT '',
 precio_unitario BIGINT NOT NULL,
 cantidad BIGINT NOT NULL,
 PRIMARY KEY (pedido_id, plato_id),
 CONSTRAINT fk_pedido_item_pedido FOREIGN KEY (pedido_id) REFERENCES pedido(id),
 CONSTRAINT chk_pedido_item_cantidad CHECK (cantidad > 0),
 CONSTRAINT chk_pedido_item_precio CHECK (precio_unitario >= 0)
) ENGINE=InnoDB;

-- Cada cambio de estado: anterior, nuevo, quién y cuándo. Una entrada por versión.
CREATE TABLE pedido_historial (
 pedido_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 version BIGINT UNSIGNED NOT NULL,
 estado_anterior ENUM('RECIBIDO','ACEPTADO','EN_PREPARACION','LISTO','EN_CAMINO','ENTREGADO','CANCELADO') NULL,
 estado_nuevo ENUM('RECIBIDO','ACEPTADO','EN_PREPARACION','LISTO','EN_CAMINO','ENTREGADO','CANCELADO') NOT NULL,
 actor_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 actor_rol VARCHAR(16) NOT NULL,
 ocurrido_en DATETIME(3) NOT NULL,
 PRIMARY KEY (pedido_id, version),
 CONSTRAINT fk_pedido_historial_pedido FOREIGN KEY (pedido_id) REFERENCES pedido(id)
) ENGINE=InnoDB;
