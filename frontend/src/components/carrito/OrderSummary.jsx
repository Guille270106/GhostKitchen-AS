import { formatPrice } from "../../utils/formatters";
import Button from "../common/Button";
import Icon from "../common/Icon";

function OrderSummary({ cart, onConfirm, loading, disabled }) {
  return (
    <aside className="summary-card">
      <h2>Resumen</h2>
      <div className="summary-line">
        <span>Productos ({cart.cantidad})</span>
        <span>{formatPrice(cart.subtotal)}</span>
      </div>
      <div className="summary-line">
        <span>Envío</span>
        <span>{formatPrice(cart.envio)}</span>
      </div>
      <div className="summary-total">
        <span>Total</span>
        <strong>{formatPrice(cart.total)}</strong>
      </div>
      <Button className="full-button" icon="check" onClick={onConfirm} loading={loading} disabled={disabled}>
        Confirmar pedido
      </Button>
      <p className="secure-note">
        <Icon name="mail" size={15} /> Te avisamos por mail cada cambio de estado y te mandamos la factura
      </p>
    </aside>
  );
}

export default OrderSummary;
