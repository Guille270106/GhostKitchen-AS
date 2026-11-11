import { estadoLabel, formatPrice, formatRelative, formatTime } from "../../utils/formatters";
import Button from "../common/Button";
import Icon from "../common/Icon";

// Columna del tablero de la cocina. Cada tarjeta es un pedido completo con formato de comanda:
// los platos van agrupados por restaurante para armar una bolsa por marca.
function KanbanColumn({ title, tone, action, pedidos, onAdvance, onReject, busyId }) {
  return (
    <section className="kanban-col">
      <div className={`kanban-title ${tone}`}>
        <h3>{title}</h3>
        <span>{pedidos.length}</span>
      </div>
      {pedidos.length === 0 && <p className="kanban-empty">Sin pedidos</p>}
      {pedidos.map((pedido) => (
        <article className={`order-card ticket ${pedido.estado === "RECIBIDO" ? "is-new" : ""}`} key={pedido.id}>
          <p className="ticket-head">
            Pedido #{pedido.codigo} · {estadoLabel(pedido.estado).toUpperCase()} · {formatTime(pedido.creadoEn)}
          </p>
          <p className="ticket-sub">{formatRelative(pedido.creadoEn)}</p>
          {/* Adónde se despacha: la dirección que el cliente confirmó con el pedido. */}
          <p className={`ticket-address ${pedido.direccion ? "" : "is-missing"}`}>
            <Icon name="pin" size={15} />
            <span>{pedido.direccion || "Sin dirección (pedido anterior a guardarla)"}</span>
          </p>

          {pedido.restaurantes.map((grupo) => (
            <div className="ticket-brand" key={grupo.restauranteId}>
              <p className="ticket-brand-name">
                <span>{grupo.nombre}</span>
              </p>
              {grupo.items.map((item) => (
                <p className="ticket-item" key={item.platoId}>
                  {item.cantidad} × {item.nombre}
                </p>
              ))}
            </div>
          ))}

          {pedido.nota && <div className="order-note">Nota: {pedido.nota}</div>}
          <div className="order-total">
            <span>Total del pedido</span>
            <strong>{formatPrice(pedido.total)}</strong>
          </div>
          <Button className="full-button" onClick={() => onAdvance(pedido)} loading={busyId === pedido.id}>
            {action}
          </Button>
          {onReject && (
            <Button variant="quiet" className="full-button" onClick={() => onReject(pedido)} disabled={busyId === pedido.id}>
              Rechazar
            </Button>
          )}
        </article>
      ))}
    </section>
  );
}

export default KanbanColumn;
