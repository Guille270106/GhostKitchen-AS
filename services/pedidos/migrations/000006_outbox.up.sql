-- Outbox (ADR-005): cada cambio de estado del pedido deja su mensaje pedido.estado_cambiado
-- en esta tabla, en la MISMA transacción que el cambio. Otro proceso lo publica en RabbitMQ y lo
-- marca recién cuando RabbitMQ confirma. Así no se pierde un aviso aunque RabbitMQ esté caído,
-- ni se avisa de un cambio que al final no se guardó.
CREATE TABLE outbox (
 id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,  -- evento_id del sobre
 tipo VARCHAR(64) NOT NULL,                                          -- también es la clave de ruteo
 cuerpo JSON NOT NULL,                                               -- el sobre completo
 creado_en DATETIME(3) NOT NULL,
 publicado_en DATETIME(3) NULL,                                      -- NULL: todavía no se publicó
 INDEX idx_outbox_pendientes (publicado_en, creado_en)
) ENGINE=InnoDB;
