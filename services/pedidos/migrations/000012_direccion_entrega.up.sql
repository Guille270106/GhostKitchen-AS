-- Dirección de entrega de cada pedido. La manda el cliente al confirmar (la pantalla la completa
-- con la de su cuenta y la puede cambiar) y la cocina la necesita para despachar el pedido.
-- Se guarda en la confirmación (para que la recuperación cree el pedido con la misma) y se copia
-- al pedido. Los pedidos anteriores quedan con NULL: no se completa con la dirección de hoy.
ALTER TABLE confirmacion ADD COLUMN direccion_entrega VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL;
ALTER TABLE pedido ADD COLUMN direccion_entrega VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NULL;
