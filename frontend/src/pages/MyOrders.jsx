import { useState } from "react";
import { Link } from "react-router-dom";

import { cancelOrder, confirmDelivery, getMyOrders } from "../api/ghostkitchen";
import EmptyState from "../components/common/EmptyState";
import Icon from "../components/common/Icon";
import Loader from "../components/common/Loader";
import Modal from "../components/common/Modal";
import Button from "../components/common/Button";
import OrderCard from "../components/pedidos/OrderCard";
import { useToast } from "../context/ToastContext";
import useLiveQuery from "../hooks/useLiveQuery";
import { estadoLabel, formatDate, formatPrice } from "../utils/formatters";

import "../styles/orders.css";

function MyOrders() {
  const { notify } = useToast();
  const { data: pedidos, loading } = useLiveQuery(getMyOrders, []);
  const [busyId, setBusyId] = useState(null);
  const [toCancel, setToCancel] = useState(null);
  const [expanded, setExpanded] = useState(null);

  const run = async (pedido, action, message) => {
    setBusyId(pedido.id);
    try {
      await action(pedido.id);
      notify(message, "mail");
    } catch (error) {
      notify(error.message, "error");
    } finally {
      setBusyId(null);
    }
  };

  if (loading && !pedidos) return <Loader />;

  const activos = (pedidos || []).filter((p) => !["ENTREGADO", "CANCELADO"].includes(p.estado));
  const anteriores = (pedidos || []).filter((p) => ["ENTREGADO", "CANCELADO"].includes(p.estado));

  return (
    <main className="container page-space orders-page">
      <div className="section-heading">
        <div>
          <h1>Mis pedidos</h1>
        </div>
      </div>

      {!pedidos?.length && (
        <EmptyState title="Todavía no hiciste pedidos" text="Cuando confirmes un carrito, lo vas a poder seguir desde acá.">
          <Link className="btn btn-primary" to="/">
            Empezar a pedir
          </Link>
        </EmptyState>
      )}

      <div className="active-orders">
        {activos.map((pedido) => (
          <OrderCard
            key={pedido.id}
            pedido={pedido}
            busy={busyId === pedido.id}
            onCancel={setToCancel}
            onConfirmDelivery={(p) => run(p, confirmDelivery, "¡Buen provecho! Marcamos el pedido como entregado.")}
          />
        ))}
      </div>

      {anteriores.length > 0 && (
        <>
          <h2 className="history-title">Pedidos anteriores</h2>
          <div className="order-history">
            {anteriores.map((pedido) => (
              <div key={pedido.id}>
                <div className="history-row">
                  <span className="history-icon">
                    <Icon name="receipt" />
                  </span>
                  <div>
                    <strong>{pedido.cocinaNombre}</strong>
                    <p>
                      {formatDate(pedido.creadoEn)} · {pedido.cantidad} productos · #{pedido.codigo}
                    </p>
                  </div>
                  <strong>
                    {formatPrice(pedido.total)}
                    {!pedido.envioRegistrado && <small title="Pedido anterior a guardar el envío: el total es solo de los platos"> (sin envío registrado)</small>}
                  </strong>
                  <span className={pedido.estado === "ENTREGADO" ? "delivered" : "cancelled"}>{estadoLabel(pedido.estado)}</span>
                  <button onClick={() => setExpanded(expanded === pedido.id ? null : pedido.id)}>
                    {expanded === pedido.id ? "Ocultar" : "Ver detalle"} <Icon name="chevron" size={16} />
                  </button>
                </div>
                {expanded === pedido.id && (
                  <div className="history-detail">
                    {pedido.items.map((item) => (
                      <p key={item.platoId}>
                        {item.cantidad}× {item.nombre} <span>{formatPrice(item.precio * item.cantidad)}</span>
                      </p>
                    ))}
                    {pedido.comprobante?.numero && <p className="muted">Factura {pedido.comprobante.numero}</p>}
                  </div>
                )}
              </div>
            ))}
          </div>
        </>
      )}

      {toCancel && (
        <Modal title="¿Cancelar el pedido?" onClose={() => setToCancel(null)}>
          <p>
            Vas a cancelar el pedido <strong>#{toCancel.codigo}</strong>. Los ingredientes vuelven al stock de la cocina y no se te cobra nada.
          </p>
          <div className="modal-actions">
            <Button variant="secondary" onClick={() => setToCancel(null)}>
              Volver
            </Button>
            <Button
              variant="danger"
              onClick={() => {
                const pedido = toCancel;
                setToCancel(null);
                run(pedido, cancelOrder, "Cancelamos tu pedido. Te mandamos un mail con el detalle.");
              }}
            >
              Sí, cancelar
            </Button>
          </div>
        </Modal>
      )}
    </main>
  );
}

export default MyOrders;
