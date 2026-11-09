import { useState } from "react";

import { advanceOrder, getKitchenBoard, rejectOrder } from "../api/ghostkitchen";
import KanbanColumn from "../components/cocina/KanbanColumn";
import Button from "../components/common/Button";
import Loader from "../components/common/Loader";
import Modal from "../components/common/Modal";
import Stat from "../components/common/Stat";
import { useToast } from "../context/ToastContext";
import useLiveQuery from "../hooks/useLiveQuery";
import { formatPrice, formatToday } from "../utils/formatters";

import "../styles/workspace.css";

const COLUMNAS = [
  { estado: "RECIBIDO", title: "Nuevos", tone: "red", action: "Aceptar pedido", reject: true },
  { estado: "ACEPTADO", title: "Aceptados", tone: "blue", action: "Empezar a preparar" },
  { estado: "EN_PREPARACION", title: "En preparación", tone: "orange", action: "Marcar como listo" },
  { estado: "LISTO", title: "Listos", tone: "green", action: "Entregado al repartidor" },
];

const MENSAJES = {
  RECIBIDO: "Aceptaste el pedido",
  ACEPTADO: "El pedido pasó a preparación",
  EN_PREPARACION: "Marcaste el pedido como listo",
  LISTO: "Le entregaste el pedido al repartidor",
};

function KitchenOrders() {
  const { notify } = useToast();
  const { data, loading } = useLiveQuery(getKitchenBoard, []);
  const [busyId, setBusyId] = useState(null);
  const [toReject, setToReject] = useState(null);

  if (loading && !data) return <Loader />;
  if (!data) return null;

  const advance = async (pedido) => {
    setBusyId(pedido.id);
    try {
      await advanceOrder(pedido);
      notify(`${MENSAJES[pedido.estado]} #${pedido.codigo}. Le avisamos al cliente por mail.`, "success");
    } catch (error) {
      notify(error.message, "error");
    } finally {
      setBusyId(null);
    }
  };

  const reject = async () => {
    const pedido = toReject;
    setToReject(null);
    setBusyId(pedido.id);
    try {
      await rejectOrder(pedido.id);
      notify(`Rechazaste el pedido #${pedido.codigo}. Los ingredientes vuelven al stock.`, "success");
    } catch (error) {
      notify(error.message, "error");
    } finally {
      setBusyId(null);
    }
  };

  return (
    <>
      <div className="workspace-header">
        <div>
          <p className="eyebrow red">{data.marcas.map((m) => m.nombre).join(" · ")}</p>
          <h1>Pedidos de hoy</h1>
          <p>
            {data.cocina.nombre} · {formatToday()}
          </p>
        </div>
        <div className="service-status">
          <i /> Recibiendo pedidos
        </div>
      </div>
      <div className="quick-stats">
        <Stat label="Pedidos hoy" value={data.stats.pedidosHoy} detail="De todos tus restaurantes" />
        <Stat label="En preparación" value={data.stats.enPreparacion} detail="Ahora mismo" />
        <Stat label="Ventas de hoy" value={formatPrice(data.stats.ventasHoy)} detail="Con envío incluido" />
      </div>
      <div className="kanban">
        {COLUMNAS.map((col) => (
          <KanbanColumn
            key={col.estado}
            title={col.title}
            tone={col.tone}
            action={col.action}
            pedidos={data.pedidos.filter((p) => p.estado === col.estado)}
            onAdvance={advance}
            onReject={col.reject ? setToReject : undefined}
            busyId={busyId}
          />
        ))}
      </div>

      {toReject && (
        <Modal title={`¿Rechazar el pedido #${toReject.codigo}?`} onClose={() => setToReject(null)}>
          <p>
            Se cancela el pedido completo, con los platos de todos sus restaurantes. Los ingredientes vuelven al stock de la cocina y el cliente recibe un mail.
          </p>
          <div className="modal-actions">
            <Button variant="secondary" onClick={() => setToReject(null)}>
              Volver
            </Button>
            <Button variant="danger" onClick={reject}>
              Rechazar pedido
            </Button>
          </div>
        </Modal>
      )}
    </>
  );
}

export default KitchenOrders;
