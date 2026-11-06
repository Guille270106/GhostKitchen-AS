CREATE TABLE reserva (
 id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 carrito_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL UNIQUE,
 cocina_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 estado VARCHAR(16) NOT NULL DEFAULT 'ACTIVA',
 creada_en DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 vence_en DATETIME(6) NOT NULL,
 UNIQUE KEY uq_reserva_cocina (id,cocina_id),
 CONSTRAINT fk_reserva_cocina FOREIGN KEY (cocina_id) REFERENCES cocina_ref(id),
 CONSTRAINT chk_reserva_estado CHECK (estado IN ('ACTIVA','LIBERADA','VENCIDA','CONSUMIDA')),
 INDEX idx_reserva_vencimiento (estado,vence_en)
) ENGINE=InnoDB;

-- Los lotes conservan la receta de cada aumento de unidades del plato.
CREATE TABLE reserva_plato (
 reserva_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 plato_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 cocina_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 cantidad BIGINT NOT NULL,
 lotes JSON NOT NULL,
 PRIMARY KEY (reserva_id,plato_id),
 CONSTRAINT chk_reserva_plato_cantidad CHECK (cantidad > 0),
 CONSTRAINT fk_rp_reserva FOREIGN KEY (reserva_id,cocina_id) REFERENCES reserva(id,cocina_id),
 CONSTRAINT fk_rp_plato FOREIGN KEY (plato_id,cocina_id) REFERENCES plato_ref(id,cocina_id)
) ENGINE=InnoDB;

CREATE TABLE reserva_item (
 reserva_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 ingrediente_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 cocina_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 cantidad BIGINT NOT NULL,
 PRIMARY KEY (reserva_id,ingrediente_id),
 CONSTRAINT chk_reserva_item_cantidad CHECK (cantidad > 0),
 CONSTRAINT fk_ri_reserva FOREIGN KEY (reserva_id,cocina_id) REFERENCES reserva(id,cocina_id),
 CONSTRAINT fk_ri_ingrediente FOREIGN KEY (ingrediente_id,cocina_id) REFERENCES ingrediente(id,cocina_id)
) ENGINE=InnoDB;

CREATE TABLE idempotencia (
 operacion VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 clave VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 huella CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 resultado JSON NULL,
 creada_en DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 PRIMARY KEY (operacion,clave)
) ENGINE=InnoDB;
