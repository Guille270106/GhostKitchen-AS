-- Claves de "agregar al carrito": "agregar" suma, así que si el cliente reenvía la misma
-- solicitud (por ejemplo, porque se cortó la respuesta) se sumaría dos veces. El frontend manda
-- una Idempotency-Key por clic; se anota en la MISMA transacción que guarda el carrito, y si
-- vuelve a llegar no se suma de nuevo.
CREATE TABLE carrito_operacion (
 cliente_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 clave VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 creada_en DATETIME(3) NOT NULL,
 PRIMARY KEY (cliente_id, clave)
);
