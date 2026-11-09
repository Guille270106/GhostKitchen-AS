import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";

import { clearCart, codigoPedido, confirmCart, removeFromCart, updateCartItem } from "../api/ghostkitchen";
import CartBrand from "../components/carrito/CartBrand";
import OrderSummary from "../components/carrito/OrderSummary";
import ReservationTimer from "../components/carrito/ReservationTimer";
import Button from "../components/common/Button";
import EmptyState from "../components/common/EmptyState";
import GhostLogo from "../components/common/GhostLogo";
import Icon from "../components/common/Icon";
import { useCart } from "../context/CartContext";
import { useToast } from "../context/ToastContext";

import "../styles/cart.css";

function Cart() {
  const { cart } = useCart();
  const { notify } = useToast();
  const navigate = useNavigate();
  const [working, setWorking] = useState(false);

  const run = async (action) => {
    setWorking(true);
    try {
      await action();
    } catch (error) {
      notify(error.message, "error");
    } finally {
      setWorking(false);
    }
  };

  if (!cart) {
    return (
      <main className="container cart-page">
        <EmptyState title="Tu carrito está vacío" text="Elegí una cocina y combiná platos de sus marcas en un solo pedido.">
          <Link className="btn btn-primary" to="/">
            Ver cocinas
          </Link>
        </EmptyState>
      </main>
    );
  }

  const vencido = cart.estado === "VENCIDO";

  // Si Inventario avisó que venció la reserva, Pedidos ya vació el carrito: no queda ni la cocina.
  if (vencido && !cart.cocina) {
    return (
      <main className="container cart-page">
        <div className="expired-banner">
          <div className="ghost-small">
            <GhostLogo compact />
          </div>
          <div>
            <strong>Se venció tu reserva y liberamos los ingredientes</strong>
            <p>Pasaron 10 minutos sin confirmar. Volvé a armar el carrito para revisar qué sigue disponible.</p>
          </div>
          <Link className="btn btn-primary" to="/">
            Volver a armar el carrito
          </Link>
        </div>
      </main>
    );
  }

  const confirmar = () =>
    run(async () => {
      const pedido = await confirmCart();
      if (pedido.confirmando) {
        notify("Estamos confirmando tu pedido. Te avisamos por mail apenas quede listo.", "mail");
      } else {
        notify(`¡Listo! Creamos el pedido #${codigoPedido(pedido.id)}. Te mandamos un mail con la confirmación y la factura.`, "mail");
      }
      navigate("/pedidos");
    });

  const volverAArmar = () =>
    run(async () => {
      const cocinaId = cart.cocina.id;
      await clearCart();
      navigate(`/cocinas/${cocinaId}`);
    });

  return (
    <main className="container cart-page">
      <div className="cart-title">
        <Link className="back-link" to={`/cocinas/${cart.cocina.id}`}>
          ← Seguir agregando de {cart.cocina.nombre}
        </Link>
        <h1>Tu pedido</h1>
        <p>
          {cart.cocina.nombre} · {cart.cantidad} producto{cart.cantidad === 1 ? "" : "s"}
        </p>
      </div>

      {vencido ? (
        <div className="expired-banner">
          <div className="ghost-small">
            <GhostLogo compact />
          </div>
          <div>
            <strong>Se venció tu reserva y liberamos los ingredientes</strong>
            <p>Pasaron 10 minutos sin confirmar. Volvé a armar el carrito para revisar qué sigue disponible.</p>
          </div>
          <Button onClick={volverAArmar} loading={working}>
            Volver a armar el carrito
          </Button>
        </div>
      ) : (
        <ReservationTimer venceEn={cart.venceEn} />
      )}

      <div className="cart-layout">
        <section className={`cart-items ${vencido ? "is-expired" : ""}`}>
          {cart.grupos.map((grupo) => (
            <CartBrand
              key={grupo.restaurante.id}
              restaurante={grupo.restaurante}
              items={grupo.items}
              disabled={working || vencido}
              onChange={(platoId, delta) => run(() => updateCartItem(platoId, delta))}
              onRemove={(platoId) => run(() => removeFromCart(platoId))}
            />
          ))}
          {!vencido && (
            <>
              <div className="same-kitchen-note">
                <Icon name="check" size={17} /> Todos tus platos salen de la misma cocina, así llegan juntos.
              </div>
              <Link className="add-more" to={`/cocinas/${cart.cocina.id}`}>
                <Icon name="plus" /> Agregar algo más de esta cocina
              </Link>
              <button className="clear-cart" onClick={() => run(clearCart)} disabled={working}>
                <Icon name="trash" size={16} /> Vaciar carrito
              </button>
            </>
          )}
        </section>
        <OrderSummary cart={cart} onConfirm={confirmar} loading={working} disabled={vencido} />
      </div>
    </main>
  );
}

export default Cart;
