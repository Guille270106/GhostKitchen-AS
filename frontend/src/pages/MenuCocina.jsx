import { useEffect, useState } from "react";
import { pedir } from "../api/ghostkitchen";
import Button from "../components/common/Button";
import { getCurrentUser } from "../utils/auth";
import { formatPrice } from "../utils/formatters";

import "../styles/inventario.css";

// Menú de la cocina: crear, editar, pausar y reactivar los platos de sus restaurantes.
// Todo lo controla Catálogo: la cocina sale del token (nunca de la pantalla), un plato nuevo
// arranca pausado y solo se activa si ya tiene receta en Inventario.
const ETIQUETAS = ["Vegano", "Vegetariano", "Sin TACC", "Picante"];

const api = {
  async platos() {
    const { items } = await pedir("GET", "/cocina/platos");
    return items || [];
  },
  async restaurantes(cocinaId) {
    const cocina = await pedir("GET", `/cocinas/${encodeURIComponent(cocinaId)}`);
    return (cocina.restaurantes || []).filter((r) => r.activo !== false);
  },
  crear(restauranteId, cuerpo) {
    return pedir("POST", `/cocina/restaurantes/${encodeURIComponent(restauranteId)}/platos`, { cuerpo });
  },
  cambiar(platoId, cuerpo) {
    return pedir("PATCH", `/cocina/platos/${encodeURIComponent(platoId)}`, { cuerpo });
  },
};

// leerPlato arma el cuerpo con lo que cargó la cocina en el formulario.
function leerPlato(campos) {
  const nombre = campos.namedItem("nombre").value.trim();
  const categoria = campos.namedItem("categoria").value.trim();
  const precio = campos.namedItem("precio").valueAsNumber;
  if (!nombre || !categoria) throw new Error("Completá el nombre y la categoría.");
  if (!Number.isSafeInteger(precio) || precio <= 0) throw new Error("El precio tiene que ser un entero mayor a 0.");
  return {
    nombre,
    categoria,
    precio,
    descripcion: campos.namedItem("descripcion").value.trim(),
    etiquetas: ETIQUETAS.filter((e) => campos.namedItem(`etiqueta-${e}`)?.checked),
  };
}

export default function MenuCocina() {
  const [platos, setPlatos] = useState(null);
  const [restaurantes, setRestaurantes] = useState([]);
  const [error, setError] = useState("");
  const [aviso, setAviso] = useState("");
  const [cargando, setCargando] = useState(true);
  const [guardando, setGuardando] = useState(false);
  const [revision, setRevision] = useState(0);
  const [editandoId, setEditandoId] = useState("");

  useEffect(() => {
    let activo = true;
    const cocinaId = getCurrentUser()?.cocinaId;
    Promise.all([api.platos(), cocinaId ? api.restaurantes(cocinaId) : Promise.resolve([])])
      .then(([lista, marcas]) => {
        if (!activo) return;
        setPlatos(lista);
        setRestaurantes(marcas);
      })
      .catch((fallo) => { if (activo) setError(fallo.message); })
      .finally(() => { if (activo) setCargando(false); });
    return () => { activo = false; };
  }, [revision]);

  function actualizar() {
    setError("");
    setCargando(true);
    setRevision((valor) => valor + 1);
  }

  async function hacer(operacion, mensaje) {
    setError("");
    setAviso("");
    setGuardando(true);
    try {
      await operacion();
      setAviso(mensaje);
      actualizar();
      return true;
    } catch (fallo) {
      setError(fallo.message);
      return false;
    } finally {
      setGuardando(false);
    }
  }

  async function crear(evento) {
    evento.preventDefault();
    const form = evento.currentTarget;
    const ok = await hacer(() => {
      const restauranteId = form.elements.namedItem("restaurante").value;
      if (!restauranteId) throw new Error("Elegí el restaurante del plato.");
      return api.crear(restauranteId, leerPlato(form.elements));
    }, "Plato creado. Arranca pausado: cargale la receta en \"Platos y recetas\" y después activalo.");
    if (ok) form.reset();
  }

  async function editar(evento) {
    evento.preventDefault();
    const form = evento.currentTarget;
    const ok = await hacer(() => api.cambiar(editandoId, leerPlato(form.elements)), "Cambios guardados.");
    if (ok) setEditandoId("");
  }

  function pausarOActivar(plato) {
    hacer(() => api.cambiar(plato.id, { activo: !plato.activo }),
      plato.activo ? `"${plato.nombre}" quedó pausado: ya no se muestra a los clientes.` : `"${plato.nombre}" está activo.`);
  }

  const nombreRestaurante = Object.fromEntries(restaurantes.map((r) => [r.id, r.nombre]));
  const editando = platos?.find((p) => p.id === editandoId);
  const ocupado = cargando || guardando;

  return (
    <section className="stock-ingredientes" aria-labelledby="menu-titulo">
      <header>
        <h1 id="menu-titulo">Menú de la cocina</h1>
        <p>Creá, editá, pausá y reactivá los platos de tus restaurantes.</p>
      </header>
      {error && <p role="alert">{error}</p>}
      {aviso && <p role="status">{aviso}</p>}

      <div aria-busy={cargando}>
        <Button variant="secondary" onClick={actualizar} disabled={ocupado}>Actualizar menú</Button>
        {cargando && <p>Cargando menú…</p>}
        {platos?.length === 0 && <p>Todavía no hay platos cargados.</p>}
        {platos?.length > 0 && (
          <div className="stock-tabla">
            <table>
              <caption>Platos de tu cocina</caption>
              <thead><tr>
                <th scope="col">Plato</th><th scope="col">Restaurante</th><th scope="col">Categoría</th>
                <th scope="col">Precio</th><th scope="col">Estado</th><th scope="col">Acciones</th>
              </tr></thead>
              <tbody>
                {platos.map((plato) => (
                  <tr key={plato.id}>
                    <th scope="row">{plato.nombre}</th>
                    <td>{nombreRestaurante[plato.restauranteId] || plato.restauranteId}</td>
                    <td>{plato.categoria}</td>
                    <td>{formatPrice(plato.precio)}</td>
                    <td>{plato.activo ? "Activo" : "Pausado"}</td>
                    <td>
                      <Button variant="secondary" onClick={() => setEditandoId(plato.id)} disabled={ocupado}>Editar</Button>{" "}
                      <Button variant="secondary" onClick={() => pausarOActivar(plato)} disabled={ocupado}>
                        {plato.activo ? "Pausar" : "Activar"}
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <div className="stock-formularios">
        {editando ? (
          <form key={editando.id} onSubmit={editar}>
            <fieldset disabled={ocupado}>
              <legend>Editar "{editando.nombre}"</legend>
              <CamposPlato plato={editando} />
              <Button type="submit">Guardar cambios</Button>{" "}
              <Button type="button" variant="secondary" onClick={() => setEditandoId("")}>Cancelar</Button>
            </fieldset>
          </form>
        ) : (
          <form onSubmit={crear}>
            <fieldset disabled={ocupado || restaurantes.length === 0}>
              <legend>Nuevo plato</legend>
              <label>Restaurante
                <select name="restaurante" required defaultValue="">
                  <option value="">Elegí un restaurante</option>
                  {restaurantes.map((r) => <option key={r.id} value={r.id}>{r.nombre}</option>)}
                </select>
              </label>
              <CamposPlato />
              <Button type="submit">Crear plato</Button>
            </fieldset>
          </form>
        )}
      </div>
    </section>
  );
}

// CamposPlato son los campos comunes de crear y editar. Al editar, arrancan con lo que ya tiene.
function CamposPlato({ plato }) {
  return (
    <>
      <label>Nombre <input name="nombre" required maxLength={80} defaultValue={plato?.nombre} /></label>
      <label>Descripción <textarea name="descripcion" maxLength={300} rows={2} defaultValue={plato?.descripcion} /></label>
      <label>Categoría <input name="categoria" required maxLength={40} placeholder="Hamburguesas" defaultValue={plato?.categoria} /></label>
      <label>Precio ($) <input name="precio" type="number" required min="1" step="1" defaultValue={plato?.precio} /></label>
      <fieldset>
        <legend>Etiquetas</legend>
        {ETIQUETAS.map((e) => (
          <label key={e}>
            <input type="checkbox" name={`etiqueta-${e}`} defaultChecked={plato?.etiquetas?.includes(e)} /> {e}
          </label>
        ))}
      </fieldset>
    </>
  );
}
