-- Referencias iniciales de las cuatro cocinas del seed de Catálogo.
-- La actualización por eventos se implementa en V12.
CREATE TABLE cocina_ref (
 id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 activa BOOLEAN NOT NULL,
 version BIGINT UNSIGNED NOT NULL DEFAULT 0
) ENGINE=InnoDB;
INSERT INTO cocina_ref (id,activa) VALUES
 ('barrio-jardin',TRUE),('cerro',TRUE),('nueva-cordoba',TRUE),('villa-allende',TRUE);

CREATE TABLE ingrediente (
 id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 cocina_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 nombre VARCHAR(120) NOT NULL,
 unidad VARCHAR(30) NOT NULL,
 total BIGINT NOT NULL,
 reservado BIGINT NOT NULL DEFAULT 0,
 creado_en TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
 CONSTRAINT fk_ingrediente_cocina FOREIGN KEY (cocina_id) REFERENCES cocina_ref(id),
 CONSTRAINT chk_ingrediente_stock CHECK (reservado >= 0 AND total >= reservado),
 CONSTRAINT chk_ingrediente_nombre CHECK (CHAR_LENGTH(TRIM(nombre)) > 0),
 CONSTRAINT chk_ingrediente_unidad CHECK (CHAR_LENGTH(TRIM(unidad)) > 0),
 INDEX idx_ingrediente_cocina_nombre (cocina_id,nombre)
) ENGINE=InnoDB;
