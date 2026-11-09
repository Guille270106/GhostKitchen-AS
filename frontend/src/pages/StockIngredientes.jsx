import { useEffect, useState } from "react";
import { pedir } from "../api/ghostkitchen";
import Button from "../components/common/Button";

import "../styles/inventario.css";

const rutaIngredientes = "/cocina/ingredientes";
const api = {
  async listar() {
    const { items } = await pedir("GET", rutaIngredientes);
    return items;
  },
  crear(cuerpo) {
    return pedir("POST", rutaIngredientes, { cuerpo });
  },
  reponer(id, cantidad) {
    return pedir("POST", `${rutaIngredientes}/${encodeURIComponent(id)}/reposiciones`, { cuerpo: { cantidad } });
  },
};

function entero(campo, minimo) {
  const valor = campo.valueAsNumber;
  if (!Number.isSafeInteger(valor) || valor < minimo) {
    throw new Error(`Ingresá una cantidad entera de al menos ${minimo}.`);
  }
  return valor;
}

export default function StockIngredientes() {
  const [items, setItems] = useState(null);
  const [error, setError] = useState("");
  const [aviso, setAviso] = useState("");
  const [cargando, setCargando] = useState(true);
  const [guardando, setGuardando] = useState(false);
  const [revision, setRevision] = useState(0);
  const [seleccion, setSeleccion] = useState("");

  useEffect(() => {
    let activo = true;
    api.listar()
      .then((datos) => { if (activo) setItems(datos); })
      .catch((fallo) => { if (activo) setError(fallo.message); })
      .finally(() => { if (activo) setCargando(false); });
    return () => { activo = false; };
  }, [revision]);

  function actualizar() {
    setError("");
    setCargando(true);
    setRevision((valor) => valor + 1);
  }

  async function guardar(evento, operacion, mensaje) {
    evento.preventDefault();
    const form = evento.currentTarget;
    setError("");
    setAviso("");
    setGuardando(true);
    try {
      await operacion(form.elements);
      form.reset();
      setAviso(mensaje);
      actualizar();
    } catch (fallo) {
      setError(fallo.message);
    } finally {
      setGuardando(false);
    }
  }

  function crear(campos) {
    const nombre = campos.namedItem("nombre").value.trim();
    const unidad = campos.namedItem("unidad").value.trim();
    if (!nombre || !unidad) throw new Error("Completá el nombre y la unidad.");
    return api.crear({ nombre, unidad, stockInicial: entero(campos.namedItem("stockInicial"), 0) });
  }

  const ingrediente = items?.find((item) => item.id === seleccion);
  const ocupado = cargando || guardando;

  return (
    <section className="stock-ingredientes" aria-labelledby="stock-titulo">
      <header>
        <h1 id="stock-titulo">Stock e ingredientes</h1>
        <p>Consultá y reponé los ingredientes de tu cocina.</p>
      </header>
      {error && <p role="alert">{error}</p>}
      {aviso && <p role="status">{aviso}</p>}

      <div aria-busy={cargando}>
        <Button variant="secondary" onClick={actualizar} disabled={ocupado}>Actualizar stock</Button>
        {cargando && <p>Cargando stock…</p>}
        {items?.length === 0 && <p>Todavía no hay ingredientes cargados.</p>}
        {items?.length > 0 && (
          <div className="stock-tabla">
            <table>
              <caption>Existencias por ingrediente</caption>
              <thead><tr>
                <th scope="col">Ingrediente</th><th scope="col">Unidad</th>
                <th scope="col">Total</th><th scope="col">Reservado</th><th scope="col">Disponible</th>
              </tr></thead>
              <tbody>
                {items.map((item) => (
                  <tr key={item.id}>
                    <th scope="row">{item.nombre}</th><td>{item.unidad}</td>
                    <td>{item.total}</td><td>{item.reservado}</td><td>{item.disponible}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <div className="stock-formularios">
        <form onSubmit={(evento) => guardar(evento, crear, "Ingrediente guardado.")}>
          <fieldset disabled={ocupado || items === null}>
            <legend>Nuevo ingrediente</legend>
            <label>Nombre <input name="nombre" required maxLength={120} /></label>
            <label>Unidad <input name="unidad" required maxLength={30} placeholder="gramos" /></label>
            <label>Stock inicial <input name="stockInicial" type="number" required min="0" step="1" defaultValue="0" /></label>
            <Button type="submit">Guardar ingrediente</Button>
          </fieldset>
        </form>

        <form onSubmit={(evento) => guardar(evento, (campos) =>
          api.reponer(ingrediente.id, entero(campos.namedItem("cantidad"), 1)), "Reposición registrada.")}>
          <fieldset disabled={ocupado || !items?.length}>
            <legend>Reponer stock</legend>
            <label>Ingrediente
              <select value={ingrediente?.id ?? ""} onChange={(evento) => setSeleccion(evento.target.value)} required>
                <option value="">Elegí un ingrediente</option>
                {items?.map((item) => <option key={item.id} value={item.id}>{item.nombre} ({item.unidad})</option>)}
              </select>
            </label>
            {ingrediente && <p>Disponible: {ingrediente.disponible} {ingrediente.unidad}</p>}
            <label>Cantidad <input name="cantidad" type="number" required min="1" step="1" /></label>
            <Button type="submit" disabled={!ingrediente}>Registrar reposición</Button>
          </fieldset>
        </form>
      </div>
    </section>
  );
}
