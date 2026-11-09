import { ESTADOS_PEDIDO, estadoLabel } from "../../utils/formatters";
import Icon from "../common/Icon";

function OrderTracker({ estado }) {
  const actual = ESTADOS_PEDIDO.indexOf(estado);
  const progreso = actual <= 0 ? 0 : (actual / (ESTADOS_PEDIDO.length - 1)) * 94;

  return (
    <div className="tracker" style={{ "--progress": `${progreso}%` }}>
      {ESTADOS_PEDIDO.map((paso, i) => (
        <div className={`track-step ${i <= actual ? "done" : ""} ${i === actual ? "current" : ""}`} key={paso}>
          <span>{i < actual ? <Icon name="check" size={15} /> : i + 1}</span>
          <p>{estadoLabel(paso)}</p>
        </div>
      ))}
    </div>
  );
}

export default OrderTracker;
