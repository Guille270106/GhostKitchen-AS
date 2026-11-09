import { formatPrice, initials } from "../../utils/formatters";
import Icon from "../common/Icon";

function CartBrand({ restaurante, items, onChange, onRemove, disabled }) {
  return (
    <div className="cart-brand">
      <div className="brand-header">
        <span className={`brand-avatar ${restaurante.color}`}>{initials(restaurante.nombre)}</span>
        <div>
          <p className="eyebrow">MARCA</p>
          <h3>{restaurante.nombre}</h3>
        </div>
        <span>{items.reduce((acc, i) => acc + i.cantidad, 0)} productos</span>
      </div>
      {items.map((item) => (
        <div className="cart-line" key={item.platoId}>
          {item.plato.foto ? (
            <img src={item.plato.foto} alt="" />
          ) : (
            <span className={`brand-avatar cart-photo ${restaurante.color}`}>{initials(item.plato.nombre)}</span>
          )}
          <div className="cart-product">
            <h4>{item.plato.nombre}</h4>
            <p>{formatPrice(item.plato.precio)} c/u</p>
            <button onClick={() => onRemove(item.platoId)} disabled={disabled}>
              Quitar
            </button>
          </div>
          <div className="quantity">
            <button onClick={() => onChange(item.platoId, -1)} disabled={disabled} aria-label={`Quitar una unidad de ${item.plato.nombre}`}>
              <Icon name="minus" size={15} />
            </button>
            <span>{item.cantidad}</span>
            <button
              onClick={() => onChange(item.platoId, 1)}
              disabled={disabled || item.plato.sinStock}
              aria-label={`Sumar una unidad de ${item.plato.nombre}`}
              title={item.plato.sinStock ? "No hay más stock" : undefined}
            >
              <Icon name="plus" size={15} />
            </button>
          </div>
          <strong>{formatPrice(item.subtotal)}</strong>
        </div>
      ))}
    </div>
  );
}

export default CartBrand;
