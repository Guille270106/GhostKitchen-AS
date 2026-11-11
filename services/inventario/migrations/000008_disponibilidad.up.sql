CREATE VIEW disponibilidad_actual AS
SELECT p.id AS plato_id,p.cocina_id,
 EXISTS (SELECT 1 FROM receta_item ri WHERE ri.plato_id=p.id) AS tiene_receta,
 (p.activo AND c.activa
  AND EXISTS (SELECT 1 FROM receta_item ri WHERE ri.plato_id=p.id)
  AND NOT EXISTS (
   SELECT 1 FROM receta_item ri JOIN ingrediente i ON i.id=ri.ingrediente_id
   WHERE ri.plato_id=p.id AND i.total-i.reservado<ri.cantidad
  )) AS disponible
FROM plato_ref p JOIN cocina_ref c ON c.id=p.cocina_id;

CREATE TABLE disponibilidad_estado (
 plato_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 tiene_receta BOOLEAN NOT NULL,
 disponible BOOLEAN NOT NULL,
 secuencia BIGINT UNSIGNED NOT NULL,
 FOREIGN KEY (plato_id) REFERENCES plato_ref(id) ON DELETE CASCADE
) ENGINE=InnoDB;

INSERT INTO disponibilidad_estado (plato_id,tiene_receta,disponible,secuencia)
SELECT plato_id,tiene_receta,disponible,0 FROM disponibilidad_actual;

CREATE TABLE disponibilidad_bloqueo (
 cocina_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY
) ENGINE=InnoDB;

CREATE TABLE mensaje_procesado (
 evento_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY
) ENGINE=InnoDB;
