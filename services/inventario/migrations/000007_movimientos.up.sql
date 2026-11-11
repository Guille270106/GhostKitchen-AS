CREATE TABLE movimiento_stock (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
 ingrediente_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 usuario_id VARCHAR(255) NOT NULL,
 delta BIGINT NOT NULL,
 creado_en DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 CONSTRAINT fk_movimiento_ingrediente FOREIGN KEY (ingrediente_id) REFERENCES ingrediente(id),
 CONSTRAINT chk_movimiento_delta CHECK (delta <> 0)
) ENGINE=InnoDB;
