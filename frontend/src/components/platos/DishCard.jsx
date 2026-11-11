import { useCart } from "../../context/CartContext";
import { formatPrice } from "../../utils/formatters";
import Icon from "../common/Icon";

function DishCard({ plato, preview = false }) {
  const { addItem, busyId } = useCart();
  const pocas = !plato.sinStock && plato.unidadesDisponibles <= 3;

  return (
    <article className={`dish-card ${plato.sinStock ? "sold-out" : ""}`}>
      <div className="dish-image">
        {plato.foto ? (
          <img src={plato.foto} alt={plato.nombre} loading="lazy" />
        ) : (
          // Sin foto del plato: un recuadro neutro, nunca la foto de otra comida.
          <div className="dish-placeholder" aria-hidden="true">
            <Icon name="utensils" size={30} />
          </div>
        )}
        {plato.destacado && !plato.sinStock && <span className="dish-tag">{plato.destacado}</span>}
        {pocas && <span className="dish-tag low">Quedan {plato.unidadesDisponibles}</span>}
        {plato.sinStock && <span className="sold-label">Sin stock</span>}
        {plato.stockSinConfirmar && !preview && <span className="dish-tag unknown">Stock sin confirmar</span>}
      </div>
      <div className="dish-info">
        <p className="eyebrow">{plato.restauranteNombre}</p>
        <h3>{plato.nombre}</h3>
        {plato.etiquetas?.length > 0 && (
          <div className="dish-tags">
            {plato.etiquetas.map((etiqueta) => (
              <span key={etiqueta}>{etiqueta}</span>
            ))}
          </div>
        )}
        <div className="dish-bottom">
          <strong>{formatPrice(plato.precio)}</strong>
          {!preview && (
            <button
              className="add-button"
              aria-label={`Agregar ${plato.nombre}`}
              onClick={() => addItem(plato)}
              disabled={plato.sinStock || busyId === plato.id}
              title={plato.sinStock ? "Sin stock" : plato.stockSinConfirmar ? "No pudimos confirmar el stock: se controla al agregar" : "Agregar al carrito"}
            >
              {busyId === plato.id ? <span className="spinner dark" /> : <Icon name="plus" size={15} />}
              {plato.sinStock ? "Agotado" : "Agregar"}
            </button>
          )}
        </div>
      </div>
    </article>
  );
}

export default DishCard;
