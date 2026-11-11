import { formatPrice } from "../../utils/formatters";
import Button from "../common/Button";
import Icon from "../common/Icon";

// Resumen del carrito y confirmación. Es un formulario para que la dirección de entrega se
// valide como el resto de los campos de la página antes de confirmar.
function OrderSummary({ cart, direccion, onDireccion, onConfirm, loading, disabled }) {
  const enviar = (event) => {
    event.preventDefault();
    onConfirm();
  };

  return (
    <form className="summary-card" onSubmit={enviar}>
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
      <label className="delivery-field">
        <span>
          <Icon name="pin" size={15} /> Entregar en
        </span>
        <input
          name="direccionEntrega"
          autoComplete="street-address"
          placeholder="Calle, número, piso y depto"
          maxLength={255}
          value={direccion}
          onChange={(event) => onDireccion(event.target.value)}
          disabled={disabled}
          required
          // Solo espacios también es "vacía": se avisa acá en lugar de esperar la respuesta del servidor.
          pattern=".*\S.*"
          title="Escribí la dirección de entrega."
        />
        <small>Es la de tu cuenta. Si hoy lo querés en otro lugar, cambiala acá.</small>
      </label>
      <Button type="submit" className="full-button" icon="check" loading={loading} disabled={disabled}>
        Confirmar pedido
      </Button>
      <p className="secure-note">
        <Icon name="mail" size={15} /> Te avisamos por mail cada cambio de estado y te mandamos la factura
      </p>
    </form>
  );
}

export default OrderSummary;
