import { createContext, useCallback, useContext, useMemo } from "react";
import { useNavigate } from "react-router-dom";

import { useSession } from "./SessionContext";
import { useToast } from "./ToastContext";

// Versión provisoria del carrito (G18): el botón "Agregar" de cada plato ya existe, pero el
// carrito real con su reserva en Inventario llega con L17, que reemplaza este archivo por el
// CartContext completo. Mientras tanto, agregar lleva al login o avisa que todavía no está.
const CartContext = createContext({ cart: null, count: 0, busyId: null, addItem: () => {} });

export function CartProvider({ children }) {
  const { user } = useSession();
  const { notify } = useToast();
  const navigate = useNavigate();

  const addItem = useCallback(() => {
    if (!user) {
      navigate("/login");
      return;
    }
    notify("El carrito todavía no está disponible.", "info");
  }, [user, navigate, notify]);

  const value = useMemo(() => ({ cart: null, count: 0, busyId: null, addItem }), [addItem]);
  return <CartContext.Provider value={value}>{children}</CartContext.Provider>;
}

// eslint-disable-next-line react-refresh/only-export-components
export function useCart() {
  return useContext(CartContext);
}
