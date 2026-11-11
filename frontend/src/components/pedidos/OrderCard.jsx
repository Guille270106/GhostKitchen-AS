import { estadoLabel, formatPrice, formatRelative, formatTime } from "../../utils/formatters";
import Button from "../common/Button";
import Icon from "../common/Icon";
import OrderTracker from "./OrderTracker";

const MENSAJES = {
  RECIBIDO: ["Recibimos tu pedido", "La cocina lo está revisando. Todavía podés cancelarlo."],
  ACEPTADO: ["La cocina aceptó tu pedido", "En unos minutos empiezan a prepararlo."],
  EN_PREPARACION: ["Tu pedido está en marcha", "La cocina está preparando los platos de todos tus restaurantes para que salgan juntos."],
  LISTO: ["¡Tu pedido está listo!", "Está esperando al repartidor."],
  EN_CAMINO: ["Tu pedido está en camino", "Llega en unos minutos."],
};

// Cómo va la factura que emite el grupo proveedor (Grupo 4) y manda por mail.
function textoFactura(comprobante) {
  if (comprobante.estado === "EMITIDO") return `Factura ${comprobante.numero}: te la mandamos por mail.`;
  if (comprobante.estado === "RECHAZADO") return "No pudimos emitir la factura de este pedido.";
  return "Estamos generando tu factura: te llega por mail en unos minutos.";
}

function OrderCard({ pedido, onCancel, onConfirmDelivery, busy }) {
  const [titulo, detalle] = MENSAJES[pedido.estado] || ["", ""];

  return (
    <article className="tracking-card">
      <div className="tracking-top">
        <div>
          <span className={`status-pill status-${pedido.estado.toLowerCase()}`}>{estadoLabel(pedido.estado)}</span>
          <h2>{titulo}</h2>
          <p>
            {pedido.cocinaNombre} · Pedido #{pedido.codigo} · {formatRelative(pedido.creadoEn)}
          </p>
          {pedido.direccion && (
            <p className="order-address">
              <Icon name="pin" size={15} /> Entregar en {pedido.direccion}
            </p>
          )}
        </div>
        <div className="eta">
          <small>Total</small>
          <strong>
            {formatPrice(pedido.total)}
            {!pedido.envioRegistrado && <small title="Pedido anterior a guardar el envío: el total es solo de los platos"> (sin envío registrado)</small>}
          </strong>
        </div>
      </div>

      <OrderTracker estado={pedido.estado} />

      <div className="preparing-note">
        <Icon name="utensils" />
        <div>
          <strong>{detalle}</strong>
          <div className="brand-chips">
            {pedido.restaurantes.map((grupo) => (
              <span key={grupo.restauranteId} className="brand-chip">
                {grupo.nombre}
              </span>
            ))}
          </div>
        </div>
      </div>

      <div className="order-detail">
        {pedido.restaurantes.map((grupo) => (
          <div className="order-detail-group" key={grupo.restauranteId}>
            <p className="order-detail-brand">{grupo.nombre}</p>
            <ul>
              {grupo.items.map((item) => (
                <li key={item.platoId}>
                  <span>
                    {item.cantidad}× {item.nombre}
                  </span>
                  <span>{formatPrice(item.precio * item.cantidad)}</span>
                </li>
              ))}
            </ul>
          </div>
        ))}
        {pedido.nota && <p className="order-note">Nota: {pedido.nota}</p>}
        {pedido.comprobante && <p className="order-note">{textoFactura(pedido.comprobante)}</p>}
      </div>

      {pedido.notificaciones?.length > 0 && (
        <details className="mail-log">
          <summary>
            <Icon name="mail" size={16} /> Avisos que te enviamos por mail ({pedido.notificaciones.length})
          </summary>
          <ul>
            {pedido.notificaciones.map((n) => (
              <li key={n.id}>
                <strong>{formatTime(n.fecha)}</strong> {n.asunto}
                {n.detalle && <small> · {n.detalle}</small>}
              </li>
            ))}
          </ul>
        </details>
      )}

      <div className="tracking-actions">
        {pedido.estado === "RECIBIDO" && (
          <Button variant="danger" onClick={() => onCancel(pedido)} loading={busy}>
            Cancelar pedido
          </Button>
        )}
        {pedido.estado === "EN_CAMINO" && (
          <Button icon="check" onClick={() => onConfirmDelivery(pedido)} loading={busy}>
            Ya lo recibí
          </Button>
        )}
      </div>
    </article>
  );
}

export default OrderCard;
