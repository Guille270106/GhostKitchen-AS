import { useEffect, useState } from "react";

import { getCocinas } from "../../api/ghostkitchen";
import { useCart } from "../../context/CartContext";
import { useSession } from "../../context/SessionContext";
import useLiveQuery from "../../hooks/useLiveQuery";
import { formatPrice } from "../../utils/formatters";
import Button from "../common/Button";
import Icon from "../common/Icon";
import ZoneSearch from "../common/ZoneSearch";

import "../../styles/sede.css";

// Cuadro para elegir la sede (una de las cocinas). Aparece sobre la pantalla principal,
// con el fondo borroso, hasta que se elige una; despues se puede reabrir para cambiarla.
function SedeSelector() {
  const { localidadId, tieneSede, elegirSede, cerrarSelectorSede } = useSession();
  const { cart } = useCart();
  const { data: cocinas } = useLiveQuery(() => getCocinas(), []);
  const [sugerencia, setSugerencia] = useState(null);

  useEffect(() => {
    document.body.classList.add("no-scroll");
    const onKey = (event) => event.key === "Escape" && tieneSede && cerrarSelectorSede();
    window.addEventListener("keydown", onKey);
    return () => {
      document.body.classList.remove("no-scroll");
      window.removeEventListener("keydown", onKey);
    };
  }, [tieneSede, cerrarSelectorSede]);

  const sugerida = sugerencia?.cocina && cocinas?.find((c) => c.id === sugerencia.cocina.id);
  const carritoDeOtraSede = cart?.estado === "ACTIVO" && cart.cantidad > 0;

  return (
    <div className="sede-overlay" onMouseDown={(e) => e.target === e.currentTarget && tieneSede && cerrarSelectorSede()}>
      <div className="sede-dialog" role="dialog" aria-modal="true" aria-labelledby="sede-title">
        {tieneSede && (
          <button className="sede-close" onClick={cerrarSelectorSede} aria-label="Cerrar">
            <Icon name="close" />
          </button>
        )}
        <h2 id="sede-title">¿Desde qué cocina te mandamos tu pedido?</h2>
        <p className="sede-lead">Escribí tu zona y te decimos cuál de nuestras 4 cocinas te queda más cerca. Desde ahí salen todos sus restaurantes en un solo envío.</p>

        <ZoneSearch onSelect={setSugerencia} />
        {sugerencia && (
          <div className="sede-suggestion">
            {sugerida ? (
              <>
                <div>
                  <small>La cocina más cercana a {sugerencia.nombre}</small>
                  <strong>{sugerida.nombre}</strong>
                  <span>
                    a {sugerencia.cocina.distanciaKm.toLocaleString("es-AR")} km · {sugerida.marcas.map((m) => m.nombre).join(" · ")}
                  </span>
                </div>
                <Button icon="check" onClick={() => elegirSede(sugerida.localidadId)}>
                  Elegir esta sede
                </Button>
              </>
            ) : (
              <p>Todavía no tenemos cocinas activas cerca de {sugerencia.nombre}.</p>
            )}
          </div>
        )}

        <p className="sede-divider">
          <span>o elegí una sede</span>
        </p>

        {carritoDeOtraSede && (
          <p className="sede-warning">
            <Icon name="alert" size={16} /> Tu carrito tiene platos de {cart.cocina?.nombre}. Si cambiás de sede, vas a tener que vaciarlo para pedir en otra.
          </p>
        )}

        <div className="sede-grid">
          {(cocinas || []).map((cocina) => {
            const actual = tieneSede && cocina.localidadId === localidadId;
            return (
              <button key={cocina.id} className={`sede-card ${actual ? "current" : ""}`} onClick={() => elegirSede(cocina.localidadId)}>
                <img src={cocina.foto} alt="" />
                <div>
                  <h3>{cocina.nombre}</h3>
                  <p className="sede-address">
                    <Icon name="pin" size={14} /> {cocina.direccion}
                  </p>
                  <ul className="sede-brands">
                    {cocina.marcas.map((m) => (
                      <li key={m.id}>{m.nombre}</li>
                    ))}
                  </ul>
                  <p className="sede-meta">
                    <span>
                      <Icon name="clock" size={14} /> {cocina.tiempo}
                    </span>
                    <span>Envío {formatPrice(cocina.envio)}</span>
                  </p>
                </div>
                {actual && <span className="sede-current">Tu sede</span>}
              </button>
            );
          })}
        </div>
      </div>
    </div>
  );
}

export default SedeSelector;
