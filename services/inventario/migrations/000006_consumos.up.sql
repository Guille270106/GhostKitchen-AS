CREATE TABLE consumo (
 pedido_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 reserva_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL UNIQUE,
 consumido_en DATETIME(6) NOT NULL,
 devuelto_en DATETIME(6) NULL,
 CONSTRAINT fk_consumo_reserva FOREIGN KEY (reserva_id) REFERENCES reserva(id)
) ENGINE=InnoDB;
