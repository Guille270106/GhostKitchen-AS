import { useEffect, useState } from "react";

import { formatCountdown } from "../../utils/formatters";
import Icon from "../common/Icon";

const TOTAL_MS = 10 * 60 * 1000;

function ReservationTimer({ venceEn, onDemoExpire }) {
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, []);

  const restante = Math.max(0, venceEn - now);
  const progreso = Math.min(100, (restante / TOTAL_MS) * 100);
  const urgente = restante < 2 * 60 * 1000;

  return (
    <div className={`timer-banner ${urgente ? "urgent" : ""}`}>
      <div className="timer-icon">
        <Icon name="clock" size={25} />
      </div>
      <div>
        <strong>Te guardamos los ingredientes</strong>
        <p>Confirmá tu pedido antes de que se liberen para otra persona.</p>
        <div className="timer-progress" aria-hidden="true">
          <i style={{ width: `${progreso}%` }} />
        </div>
      </div>
      <span className="timer" aria-label="Tiempo restante de la reserva">
        {formatCountdown(restante)}
      </span>
      {onDemoExpire && (
        <button className="demo-expire" onClick={onDemoExpire} title="Para la demo: vence la reserva ahora">
          Simular vencimiento
        </button>
      )}
    </div>
  );
}

export default ReservationTimer;
