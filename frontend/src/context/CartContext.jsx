import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";

import { addToCart, clearCart, getCart, subscribe } from "../api/ghostkitchen";
import Modal from "../components/common/Modal";
import Button from "../components/common/Button";
import { useSession } from "./SessionContext";
import { useToast } from "./ToastContext";

const CartContext = createContext(null);

export function CartProvider({ children }) {
  const { user } = useSession();
  const { notify } = useToast();
  const navigate = useNavigate();
  const location = useLocation();
  const [cart, setCart] = useState(null);
  const [conflict, setConflict] = useState(null);
  const [busyId, setBusyId] = useState(null);

  useEffect(() => {
    let active = true;
    const load = () => getCart().then((data) => active && setCart(data)).catch(() => active && setCart(null));
    load();
    const unsubscribe = subscribe(load);
    return () => {
      active = false;
      unsubscribe();
    };
  }, [user?.id]);

  // Cuando vence la reserva se vuelve a consultar: Pedidos responde que venció (Inventario ya
  // liberó los ingredientes) y el carrito muestra el aviso.
  useEffect(() => {
    if (cart?.estado !== "ACTIVO") return undefined;
    const wait = Math.max(0, cart.venceEn - Date.now()) + 200;
    const id = window.setTimeout(() => getCart().then(setCart), wait);
    return () => window.clearTimeout(id);
  }, [cart?.estado, cart?.venceEn]);

  const addItem = useCallback(
    async (plato) => {
      if (!user) {
        notify("Ingresá con tu cuenta para armar tu pedido.", "info");
        navigate("/login", { state: { from: location } });
        return;
      }
      if (user.rol !== "CLIENTE") {
        notify("Solo las cuentas de cliente pueden hacer pedidos.", "error");
        return;
      }
      setBusyId(plato.id);
      try {
        const updated = await addToCart(plato.id);
        setCart(updated);
        const first = updated.cantidad === 1;
        notify(first ? `Agregaste ${plato.nombre}. Te guardamos los ingredientes por 10 minutos.` : `Agregaste ${plato.nombre}.`, "success");
      } catch (error) {
        if (error.code === "OTRA_COCINA") setConflict(plato);
        else notify(error.message, "error");
        // Si la reserva venció, Pedidos ya vació el carrito: se vuelve a leer.
        if (error.code === "CARRITO_VENCIDO") getCart().then(setCart).catch(() => setCart(null));
      } finally {
        setBusyId(null);
      }
    },
    [user, notify, navigate, location]
  );

  const replaceCart = async () => {
    const plato = conflict;
    setConflict(null);
    try {
      await clearCart();
      const updated = await addToCart(plato.id);
      setCart(updated);
      notify(`Empezaste un carrito nuevo con ${plato.nombre}.`, "success");
    } catch (error) {
      notify(error.message, "error");
    }
  };

  const value = useMemo(() => ({ cart, addItem, busyId, count: cart?.estado === "ACTIVO" ? cart.cantidad : 0 }), [cart, addItem, busyId]);

  return (
    <CartContext.Provider value={value}>
      {children}
      {conflict && (
        <Modal title="Tu carrito es de otra cocina" onClose={() => setConflict(null)}>
          <p>
            Todos los platos de un pedido tienen que salir de la misma cocina, así llegan juntos. Tu carrito tiene platos de{" "}
            <strong>{cart?.cocina?.nombre}</strong> y <strong>{conflict.nombre}</strong> es de <strong>{conflict.cocinaNombre}</strong>.
          </p>
          <p className="modal-hint">Si vaciás el carrito, liberamos los ingredientes que tenías reservados.</p>
          <div className="modal-actions">
            <Button variant="secondary" onClick={() => setConflict(null)}>
              Cancelar
            </Button>
            <Button onClick={replaceCart}>Vaciar carrito y agregar</Button>
          </div>
        </Modal>
      )}
    </CartContext.Provider>
  );
}

// eslint-disable-next-line react-refresh/only-export-components
export function useCart() {
  return useContext(CartContext);
}
