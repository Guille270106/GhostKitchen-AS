import { useEffect, useId, useState } from "react";

import { buscarZonas } from "../../api/ghostkitchen";
import Icon from "./Icon";

// Buscador de zona con autocompletado. Cada sugerencia muestra la cocina mas cercana.
function ZoneSearch({ onSelect, placeholder = "Escribí tu barrio o ciudad", initialValue = "" }) {
  const listId = useId();
  const [texto, setTexto] = useState(initialValue);
  const [resultados, setResultados] = useState([]);
  const [abierto, setAbierto] = useState(false);
  const [activo, setActivo] = useState(0);

  useEffect(() => {
    let vigente = true;
    const timer = window.setTimeout(async () => {
      const lista = await buscarZonas(texto);
      if (vigente) {
        setResultados(lista);
        setActivo(0);
      }
    }, 150);
    return () => {
      vigente = false;
      window.clearTimeout(timer);
    };
  }, [texto]);

  const elegir = (zona) => {
    setTexto(zona.nombre);
    setAbierto(false);
    onSelect(zona);
  };

  const onKeyDown = (event) => {
    if (!resultados.length) return;
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setAbierto(true);
      setActivo((i) => Math.min(i + 1, resultados.length - 1));
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      setActivo((i) => Math.max(i - 1, 0));
    } else if (event.key === "Enter") {
      event.preventDefault();
      elegir(resultados[activo]);
    } else if (event.key === "Escape") {
      setAbierto(false);
    }
  };

  const mostrar = abierto && texto.trim().length >= 2;

  return (
    <div className="zone-search">
      <div className="zone-input">
        <Icon name="pin" size={18} />
        <input
          value={texto}
          onChange={(e) => {
            setTexto(e.target.value);
            setAbierto(true);
          }}
          onFocus={() => setAbierto(true)}
          onBlur={() => window.setTimeout(() => setAbierto(false), 150)}
          onKeyDown={onKeyDown}
          placeholder={placeholder}
          aria-label="Tu zona"
          role="combobox"
          aria-expanded={mostrar}
          aria-controls={listId}
          autoComplete="off"
        />
      </div>
      {mostrar && (
        <ul className="zone-results" id={listId} role="listbox">
          {resultados.map((zona, i) => (
            <li key={zona.id} role="option" aria-selected={i === activo}>
              <button type="button" className={i === activo ? "active" : ""} onMouseDown={(e) => e.preventDefault()} onClick={() => elegir(zona)}>
                <strong>{zona.nombre}</strong>
                {zona.cocina ? (
                  <small>
                    {zona.cocina.nombre} · a {zona.cocina.distanciaKm.toLocaleString("es-AR")} km
                  </small>
                ) : (
                  <small>Sin cocinas activas cerca</small>
                )}
              </button>
            </li>
          ))}
          {!resultados.length && <li className="zone-empty">No encontramos esa zona. Probá con tu barrio o ciudad.</li>}
        </ul>
      )}
    </div>
  );
}

export default ZoneSearch;
