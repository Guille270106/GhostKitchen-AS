import { useEffect, useRef, useState } from "react";
import { pedir } from "../api/ghostkitchen";
import { crearApiRecetasMermas, validarReceta } from "../api/recetas-mermas.mjs";
import Button from "../components/common/Button";
import "../styles/recetas-mermas.css";

const api = crearApiRecetasMermas(pedir);

export default function RecetasMermas() {
  const [datos, setDatos] = useState(null);
  const [cargando, setCargando] = useState(true);
  const [guardando, setGuardando] = useState(false);
  const [revision, setRevision] = useState(0);
  const [error, setError] = useState("");
  const [aviso, setAviso] = useState("");
  const [platoId, setPlatoId] = useState("");
  const [ingredienteId, setIngredienteId] = useState("");
  const [filas, setFilas] = useState([]);
  const proximaClave = useRef(0);

  useEffect(() => {
    let activo = true;
    api.cargar()
      .then((respuesta) => { if (activo) setDatos(respuesta); })
      .catch((fallo) => { if (activo) setError(fallo.message); })
      .finally(() => { if (activo) setCargando(false); });
    return () => { activo = false; };
  }, [revision]);

  function actualizar() {
    setError("");
    setCargando(true);
    setRevision((valor) => valor + 1);
  }

  function elegirPlato(id) {
    setPlatoId(id);
    const items = datos.recetas.find((receta) => receta.platoId === id)?.items ?? [];
    setFilas(items.map((item) => ({ ...item, clave: proximaClave.current++ })));
  }

  function cambiarFila(clave, cambio) {
    setFilas((actuales) => actuales.map((fila) => fila.clave === clave ? { ...fila, ...cambio } : fila));
  }

  async function guardar(evento, operacion, mensaje) {
    evento.preventDefault();
    const formulario = evento.currentTarget;
    setError("");
    setAviso("");
    setGuardando(true);
    try {
      await operacion(formulario);
      setAviso(mensaje);
    } catch (fallo) {
      setError(fallo.code === "SIN_STOCK"
        ? "La merma supera el stock disponible. Actualizá las existencias y revisá la cantidad."
        : fallo.message);
    } finally {
      setGuardando(false);
    }
  }

  const ingrediente = datos?.ingredientes.find((item) => item.id === ingredienteId);
  const ocupado = cargando || guardando;
  return (
    <section className="recetas-mermas" aria-labelledby="recetas-mermas-titulo">
      <h1 id="recetas-mermas-titulo">Recetas y mermas</h1>
      {error && <p role="alert">{error}</p>}
      {aviso && <p role="status">{aviso}</p>}
      <Button variant="secondary" onClick={actualizar} disabled={ocupado}>Actualizar datos</Button>
      {cargando && <p>Cargando platos, recetas e ingredientes…</p>}
      {datos && (
        <div className="recetas-mermas-formularios">
          <form onSubmit={(evento) => guardar(evento, async () => {
            const guardada = await api.guardarReceta(platoId, validarReceta(filas));
            setDatos((anterior) => ({
              ...anterior,
              recetas: [...anterior.recetas.filter((receta) => receta.platoId !== platoId), guardada],
            }));
          }, "Receta guardada.")}>
            <fieldset disabled={ocupado}>
              <legend>Receta por plato</legend>
              <label>Plato
                <select value={platoId} onChange={(evento) => elegirPlato(evento.target.value)} required>
                  <option value="">Elegí un plato</option>
                  {datos.platos.map((plato) => (
                    <option key={plato.id} value={plato.id}>{plato.nombre}{!plato.activo && " (inactivo)"}</option>
                  ))}
                </select>
              </label>
              {platoId && !datos.ingredientes.length && <p>Primero cargá un ingrediente en Stock.</p>}
              {filas.map((fila, indice) => (
                <div key={fila.clave} className="receta-fila">
                  <label>Ingrediente {indice + 1}
                    <select value={fila.ingredienteId} onChange={(evento) => cambiarFila(fila.clave, { ingredienteId: evento.target.value })} required>
                      <option value="">Elegí un ingrediente</option>
                      {datos.ingredientes.map((item) => (
                        <option key={item.id} value={item.id}>{item.nombre} ({item.unidad})</option>
                      ))}
                    </select>
                  </label>
                  <label>Cantidad
                    <input type="number" min="1" step="1" required value={fila.cantidad}
                      onChange={(evento) => cambiarFila(fila.clave, { cantidad: evento.target.value })} />
                  </label>
                  <Button variant="secondary" onClick={() => setFilas(filas.filter((item) => item.clave !== fila.clave))}>
                    Quitar ingrediente {indice + 1}
                  </Button>
                </div>
              ))}
              <Button variant="secondary" disabled={!platoId || !datos.ingredientes.length} onClick={() => {
                const nueva = { clave: proximaClave.current++, ingredienteId: "", cantidad: "" };
                setFilas((actuales) => [...actuales, nueva]);
              }}>Agregar ingrediente</Button>
              <Button type="submit" disabled={!platoId || !filas.length}>Guardar receta</Button>
            </fieldset>
          </form>

          <form onSubmit={(evento) => guardar(evento, async (formulario) => {
            const cantidad = formulario.elements.namedItem("cantidad").valueAsNumber;
            if (!ingrediente || !Number.isSafeInteger(cantidad) || cantidad < 1 || cantidad > ingrediente.disponible) {
              throw new Error("Ingresá una cantidad entera que no supere el stock disponible.");
            }
            const actualizado = await api.registrarMerma(ingrediente.id, cantidad);
            setDatos((anterior) => ({
              ...anterior,
              ingredientes: anterior.ingredientes.map((item) => item.id === actualizado.id ? actualizado : item),
            }));
            formulario.reset();
          }, "Merma registrada.")}>
            <fieldset disabled={ocupado || !datos.ingredientes.length}>
              <legend>Registrar merma</legend>
              <label>Ingrediente
                <select value={ingredienteId} onChange={(evento) => setIngredienteId(evento.target.value)} required>
                  <option value="">Elegí un ingrediente</option>
                  {datos.ingredientes.map((item) => <option key={item.id} value={item.id}>{item.nombre} ({item.unidad})</option>)}
                </select>
              </label>
              {ingrediente && <p>Disponible para descontar: {ingrediente.disponible} {ingrediente.unidad}.</p>}
              <label>Cantidad
                <input name="cantidad" type="number" min="1" max={ingrediente?.disponible} step="1" required />
              </label>
              <Button type="submit" disabled={!ingrediente?.disponible}>Registrar merma</Button>
            </fieldset>
          </form>
        </div>
      )}
    </section>
  );
}
