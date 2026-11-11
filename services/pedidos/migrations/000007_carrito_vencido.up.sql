-- Carrito vencido: cuando Inventario avisa que la reserva venció (reserva.vencida), Pedidos
-- vacía el carrito y anota cuándo, para que el cliente vea "Tu carrito venció" en vez de un
-- carrito vacío sin explicación. Cualquier cambio posterior del cliente lo vuelve a NULL.
ALTER TABLE carrito ADD COLUMN vencio_en DATETIME(3) NULL;
