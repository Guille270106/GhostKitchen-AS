import { Fragment, useEffect, useId, useRef, useState } from "react";

import Icon from "./Icon";
import { MINIMO_SUGERENCIAS, autocompletePlatos } from "../../api/ghostkitchen";
import { useSession } from "../../context/SessionContext";

// Espera entre que se deja de escribir y se pide a Búsqueda, para no consultar en cada tecla.
const ESPERA_MS = 200;

const sinTildes = (texto) => String(texto || "").normalize("NFD").replace(/[̀-ͯ]/g, "").toLowerCase();

// Pone en negrita el principio de cada palabra que coincide con lo escrito, como Google.
function Resaltado({ texto, buscado }) {
  const tokens = sinTildes(buscado).split(/[^a-z0-9ñ]+/).filter(Boolean);
  return texto.split(/(\s+)/).map((parte, i) => {
    const largo = Math.max(0, ...tokens.filter((t) => sinTildes(parte).startsWith(t)).map((t) => t.length));
    if (!largo) return <Fragment key={i}>{parte}</Fragment>;
    return (
      <Fragment key={i}>
        <mark>{parte.slice(0, largo)}</mark>
        {parte.slice(largo)}
      </Fragment>
    );
  });
}

// Buscador con sugerencias mientras se escribe. Se mueve con las flechas, Enter elige y Escape
// cierra. Elegir un plato busca ese nombre; elegir una categoría llama a onCategoria (si la
// pantalla no lo pasa, busca el nombre de la categoría).
function SearchBox({ initialValue = "", onSearch, onCategoria }) {
  const listId = useId();
  const { localidadId } = useSession();
  const [value, setValue] = useState(initialValue);
  const [sugerencias, setSugerencias] = useState({ texto: "", categorias: [], platos: [] });
  const [abierto, setAbierto] = useState(false);
  const [activa, setActiva] = useState(-1);
  const pedido = useRef(0);

  const texto = value.trim();
  const opciones = [
    ...sugerencias.categorias.map((c) => ({ tipo: "categoria", clave: `c-${c.id}`, valor: c })),
    ...sugerencias.platos.map((p) => ({ tipo: "plato", clave: `p-${p.id}`, valor: p })),
  ];
  const visible = abierto && texto.length >= MINIMO_SUGERENCIAS && opciones.length > 0;

  useEffect(() => {
    const numero = ++pedido.current;
    // Con menos letras no se consulta (la lista ya no se muestra, ver "visible").
    if (texto.length < MINIMO_SUGERENCIAS) return undefined;
    const timer = setTimeout(async () => {
      try {
        const res = await autocompletePlatos(texto, { localidadId });
        // Si mientras tanto se siguió escribiendo, esta respuesta ya no sirve.
        if (numero === pedido.current) {
          setSugerencias({ texto, ...res });
          setActiva(-1);
        }
      } catch {
        if (numero === pedido.current) setSugerencias({ texto, categorias: [], platos: [] });
      }
    }, ESPERA_MS);
    return () => clearTimeout(timer);
  }, [texto, localidadId]);

  const cerrar = () => {
    setAbierto(false);
    setActiva(-1);
  };

  const elegir = (opcion) => {
    cerrar();
    if (opcion.tipo === "categoria") {
      if (onCategoria) onCategoria(opcion.valor);
      else {
        setValue(opcion.valor.label);
        onSearch?.(opcion.valor.label);
      }
      return;
    }
    setValue(opcion.valor.nombre);
    onSearch?.(opcion.valor.nombre);
  };

  const submit = (event) => {
    event.preventDefault();
    if (visible && activa >= 0) {
      elegir(opciones[activa]);
      return;
    }
    cerrar();
    onSearch?.(texto);
  };

  const teclas = (event) => {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      if (!opciones.length) return;
      event.preventDefault();
      if (!visible) {
        setAbierto(true);
        return;
      }
      const paso = event.key === "ArrowDown" ? 1 : -1;
      // -1 es "ninguna": volver al texto escrito, como en Google.
      setActiva((actual) => ((actual + 1 + paso + opciones.length + 1) % (opciones.length + 1)) - 1);
    } else if (event.key === "Escape" && visible) {
      event.preventDefault();
      cerrar();
    }
  };

  const idOpcion = (i) => `${listId}-op-${i}`;
  let titulo = null;

  return (
    <form className={`search-box ${visible ? "con-sugerencias" : ""}`} onSubmit={submit} role="search">
      <Icon name="search" />
      <input
        role="combobox"
        aria-label="Buscar comida"
        aria-autocomplete="list"
        aria-expanded={visible}
        aria-controls={listId}
        aria-activedescendant={visible && activa >= 0 ? idOpcion(activa) : undefined}
        autoComplete="off"
        placeholder="¿Qué tenés ganas de comer?"
        value={value}
        onChange={(event) => {
          setValue(event.target.value);
          setAbierto(true);
        }}
        onFocus={() => setAbierto(true)}
        onBlur={cerrar}
        onKeyDown={teclas}
      />
      <button type="submit">Buscar</button>
      {visible && (
        <ul className="search-suggestions" id={listId} role="listbox" aria-label="Sugerencias">
          {opciones.map((opcion, i) => {
            const grupo = opcion.tipo === "categoria" ? "Categorías" : "Platos";
            const encabezado = grupo !== titulo;
            titulo = grupo;
            return (
              <Fragment key={opcion.clave}>
                {encabezado && (
                  <li className="search-suggestions-title" role="presentation">
                    {grupo}
                  </li>
                )}
                <li
                  id={idOpcion(i)}
                  role="option"
                  aria-selected={i === activa}
                  className={i === activa ? "activa" : ""}
                  // mousedown en lugar de click: así el input no pierde el foco antes de elegir.
                  onMouseDown={(event) => {
                    event.preventDefault();
                    elegir(opcion);
                  }}
                  onMouseEnter={() => setActiva(i)}
                >
                  {opcion.tipo === "categoria" ? (
                    <>
                      <span className="sugerencia-icono">
                        <Icon name={opcion.valor.esEtiqueta ? "filter" : "list"} size={16} />
                      </span>
                      <span className="sugerencia-texto">
                        <strong>
                          <Resaltado texto={opcion.valor.label} buscado={sugerencias.texto} />
                        </strong>
                        <small>{opcion.valor.esEtiqueta ? "Ver todos los platos con esta etiqueta" : "Ver toda la categoría"}</small>
                      </span>
                    </>
                  ) : (
                    <>
                      {opcion.valor.foto ? <img className="sugerencia-foto" src={opcion.valor.foto} alt="" /> : <span className="sugerencia-icono"><Icon name="utensils" size={16} /></span>}
                      <span className="sugerencia-texto">
                        <span>
                          <Resaltado texto={opcion.valor.nombre} buscado={sugerencias.texto} />
                        </span>
                        <small>
                          {opcion.valor.restauranteNombre}
                          {opcion.valor.sinStock ? " · Sin stock" : ""}
                        </small>
                      </span>
                    </>
                  )}
                </li>
              </Fragment>
            );
          })}
        </ul>
      )}
    </form>
  );
}

export default SearchBox;
