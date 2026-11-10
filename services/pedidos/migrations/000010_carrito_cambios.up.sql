ALTER TABLE carrito_operacion
 ADD COLUMN interna BOOLEAN NOT NULL DEFAULT FALSE,
 ADD COLUMN estado ENUM('incierta','por_liberar','completada','compensada') NOT NULL DEFAULT 'completada',
 ADD COLUMN datos JSON NULL,
 ADD COLUMN pendiente VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin
  GENERATED ALWAYS AS (IF(interna AND estado IN ('incierta','por_liberar'), cliente_id, NULL)) VIRTUAL,
 ADD UNIQUE KEY carrito_un_cambio_pendiente (pendiente),
 DROP PRIMARY KEY,
 ADD PRIMARY KEY (cliente_id, clave, interna),
 ADD INDEX carrito_operacion_pendiente (estado, cliente_id);
