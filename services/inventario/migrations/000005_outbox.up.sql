CREATE TABLE outbox (
 id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 tipo VARCHAR(64) NOT NULL,
 cuerpo JSON NOT NULL,
 creado_en DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 INDEX idx_outbox_orden (creado_en,id)
) ENGINE=InnoDB;
