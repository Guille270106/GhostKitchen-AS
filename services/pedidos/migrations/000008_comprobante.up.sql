-- Factura del pedido: la emite el grupo proveedor (Grupo 4, API de comprobantes) y se la
-- manda por correo al cliente. Se anota PENDIENTE en la MISMA transacción que crea el pedido; un
-- proceso en segundo plano la pide y reintenta si el proveedor no contesta. El pedido nunca
-- espera a la factura (ADR-009).
CREATE TABLE comprobante (
 pedido_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 estado ENUM('PENDIENTE','EMITIDO','RECHAZADO') NOT NULL,
 comprobante_id VARCHAR(64) NULL,   -- id que le dio el proveedor
 numero VARCHAR(64) NULL,           -- número de factura, por ejemplo B-0001-00000042
 motivo VARCHAR(255) NULL,          -- por qué lo rechazó (si lo rechazó)
 intentos INT NOT NULL DEFAULT 0,
 creado_en DATETIME(3) NOT NULL,
 actualizado_en DATETIME(3) NOT NULL,
 INDEX idx_comprobante_pendientes (estado, actualizado_en),
 CONSTRAINT fk_comprobante_pedido FOREIGN KEY (pedido_id) REFERENCES pedido(id)
) ENGINE=InnoDB;
