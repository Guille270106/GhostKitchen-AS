-- Plantilla de infraestructura; las tablas del negocio se agregan desde 000002.
CREATE TABLE servicio_metadata (
    id TINYINT UNSIGNED NOT NULL PRIMARY KEY,
    nombre VARCHAR(32) NOT NULL,
    creado_en TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_metadata_unica CHECK (id = 1)
) ENGINE=InnoDB;
INSERT INTO servicio_metadata (id, nombre) VALUES (1, 'usuarios');
