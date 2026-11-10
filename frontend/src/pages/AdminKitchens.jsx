import { useState } from "react";

import { crearCocina, crearRestaurante, editarCocina, editarRestaurante, toggleCocina, toggleRestaurante } from "../api/ghostkitchen";
import Button from "../components/common/Button";
import EmptyState from "../components/common/EmptyState";
import Icon from "../components/common/Icon";
import Loader from "../components/common/Loader";
import Modal from "../components/common/Modal";
import Select from "../components/common/Select";
import { useToast } from "../context/ToastContext";
import { MAX_RESTAURANTES, zonas } from "../data/visual";
import useAdminCocina from "../hooks/useAdminCocina";
import { formatPrice, initials } from "../utils/formatters";

import "../styles/workspace.css";
import "../styles/admin.css";

const COCINA_VACIA = { nombre: "", direccion: "", zonaId: "", envio: "1200", tiempoMin: "25", tiempoMax: "40", email: "", password: "" };

// Panel del admin de cocina (HU14, HU15, HU16): las cocinas de su localidad, sus restaurantes
// y la cuenta con la que cada cocina entra a trabajar. Catálogo y Usuarios controlan que sea
// admin y que todo quede en su localidad.
function AdminKitchens() {
  const { notify } = useToast();
  const { cocinas, localidad, loading, error } = useAdminCocina();
  const [nuevaCocina, setNuevaCocina] = useState(null);
  const [nuevaMarca, setNuevaMarca] = useState(null); // { cocinaId, cocinaNombre, nombre }
  const [cocinaEditada, setCocinaEditada] = useState(null); // { id, nombre, direccion, tiempoMin, tiempoMax, envio }
  const [marcaEditada, setMarcaEditada] = useState(null); // { cocinaId, id, nombre, anterior }
  const [saving, setSaving] = useState(false);

  if (loading) return <Loader />;
  if (error && !cocinas.length) return <EmptyState title="No pudimos cargar tus cocinas" text={error.message} />;

  const guardar = async (event, action, onDone) => {
    event.preventDefault();
    setSaving(true);
    try {
      await action();
      onDone();
    } catch (err) {
      notify(err.message, "error");
    } finally {
      setSaving(false);
    }
  };

  const pausar = async (cocina) => {
    try {
      await toggleCocina(cocina);
      notify(cocina.activa ? `Pausaste ${cocina.nombre}: deja de aparecer para los clientes.` : `${cocina.nombre} vuelve a recibir pedidos.`, "success");
    } catch (err) {
      notify(err.message, "error");
    }
  };

  const pausarMarca = async (cocina, r) => {
    try {
      await toggleRestaurante(cocina.id, r);
      notify(r.activo ? `Pausaste ${r.nombre}: sus platos dejan de mostrarse.` : `${r.nombre} vuelve a mostrar sus platos.`, "success");
    } catch (err) {
      notify(err.message, "error");
    }
  };

  return (
    <>
      <div className="workspace-header">
        <div>
          <p className="eyebrow">Localidad: {localidad?.nombre}</p>
          <h1>Cocinas y restaurantes</h1>
          <p>Cada cocina tiene su propio stock y su cuenta para administrar menú, inventario y pedidos. Sus restaurantes (hasta {MAX_RESTAURANTES}) comparten ese stock.</p>
        </div>
        <Button icon="plus" onClick={() => setNuevaCocina(COCINA_VACIA)}>
          Agregar cocina
        </Button>
      </div>

      {!cocinas.length && <EmptyState title="Tu localidad todavía no tiene cocinas" text="Agregá la primera con el botón de arriba." />}

      <div className="network-grid">
        {cocinas.map((cocina) => (
          <article className={`network-card ${cocina.activa ? "" : "is-paused"}`} key={cocina.id}>
            <img src={cocina.foto} alt="" />
            <div>
              <div className="network-top">
                <span className={`stock-pill ${cocina.activa ? "ok" : "low"}`}>{cocina.activa ? "Activa" : "Pausada"}</span>
                <span>
                  <button
                    className="text-link"
                    onClick={() =>
                      setCocinaEditada({
                        id: cocina.id,
                        nombre: cocina.nombre,
                        direccion: cocina.direccion || "",
                        tiempoMin: String(cocina.tiempoMin ?? ""),
                        tiempoMax: String(cocina.tiempoMax ?? ""),
                        envio: String(cocina.envio ?? 0),
                      })
                    }
                  >
                    Editar
                  </button>{" "}
                  <button className="text-link" onClick={() => pausar(cocina)}>
                    {cocina.activa ? "Pausar" : "Reactivar"}
                  </button>
                </span>
              </div>
              <h2>{cocina.nombre}</h2>
              <p>
                <Icon name="pin" size={16} /> {cocina.direccion} · Envío {formatPrice(cocina.envio)}
              </p>
              {cocina.tiempo && <p className="muted">Entrega en {cocina.tiempo}</p>}
              <ul className="brand-list">
                {cocina.restaurantes.map((r) => (
                  <li key={r.id} className={r.activo ? "" : "is-paused"}>
                    <span className={`brand-avatar small ${r.color}`}>{initials(r.nombre)}</span>
                    <div>
                      <strong>{r.nombre}</strong>
                      <small>{r.activo ? "Activo" : "Pausado"}</small>
                    </div>
                    <button className="text-link" onClick={() => setMarcaEditada({ cocinaId: cocina.id, id: r.id, nombre: r.nombre, anterior: r.nombre })}>
                      Editar
                    </button>
                    <button className="text-link" onClick={() => pausarMarca(cocina, r)}>
                      {r.activo ? "Pausar" : "Reactivar"}
                    </button>
                  </li>
                ))}
                {!cocina.restaurantes.length && <li className="muted">Todavía no tiene restaurantes.</li>}
              </ul>
              <div className="network-bottom">
                <span>
                  <Icon name="store" size={17} /> {cocina.restaurantes.length} de {MAX_RESTAURANTES} restaurantes
                </span>
                <button
                  onClick={() => setNuevaMarca({ cocinaId: cocina.id, cocinaNombre: cocina.nombre, nombre: "" })}
                  disabled={cocina.restaurantes.length >= MAX_RESTAURANTES}
                  title={cocina.restaurantes.length >= MAX_RESTAURANTES ? `Máximo ${MAX_RESTAURANTES} restaurantes por cocina` : undefined}
                >
                  Nuevo restaurante <Icon name="plus" size={16} />
                </button>
              </div>
            </div>
          </article>
        ))}
      </div>

      {nuevaCocina && (
        <Modal title="Agregar cocina" onClose={() => setNuevaCocina(null)}>
          <form
            className="modal-form"
            onSubmit={(e) =>
              guardar(
                e,
                () => crearCocina(nuevaCocina),
                () => {
                  notify(`Creaste la cocina ${nuevaCocina.nombre}. Entra a trabajar con ${nuevaCocina.email}.`, "success");
                  setNuevaCocina(null);
                }
              )
            }
          >
            <label>
              Nombre
              <input value={nuevaCocina.nombre} maxLength={80} onChange={(e) => setNuevaCocina({ ...nuevaCocina, nombre: e.target.value })} required />
            </label>
            <label>
              Dirección
              <input value={nuevaCocina.direccion} maxLength={120} onChange={(e) => setNuevaCocina({ ...nuevaCocina, direccion: e.target.value })} required />
            </label>
            {/* div y no label: dentro de un label, el clic en una opción vuelve a abrir la lista */}
            <div className="campo">
              Zona
              <Select
                ariaLabel="Zona de la cocina"
                placeholder="Elegí la zona"
                value={nuevaCocina.zonaId}
                onChange={(zonaId) => setNuevaCocina({ ...nuevaCocina, zonaId })}
                options={zonas.map((z) => ({ value: z.id, label: z.nombre }))}
              />
            </div>
            <div className="form-row">
              <label>
                Entrega desde (min)
                <input type="number" min="1" max="240" value={nuevaCocina.tiempoMin} onChange={(e) => setNuevaCocina({ ...nuevaCocina, tiempoMin: e.target.value })} required />
              </label>
              <label>
                hasta (min)
                <input type="number" min="1" max="240" value={nuevaCocina.tiempoMax} onChange={(e) => setNuevaCocina({ ...nuevaCocina, tiempoMax: e.target.value })} required />
              </label>
            </div>
            <label>
              Costo de envío (ARS)
              <input type="number" min="0" step="100" value={nuevaCocina.envio} onChange={(e) => setNuevaCocina({ ...nuevaCocina, envio: e.target.value })} />
            </label>
            <label>
              Email de la cuenta de la cocina
              <input type="email" value={nuevaCocina.email} onChange={(e) => setNuevaCocina({ ...nuevaCocina, email: e.target.value })} required />
            </label>
            <label>
              Contraseña inicial
              <input type="password" minLength={8} autoComplete="new-password" value={nuevaCocina.password} onChange={(e) => setNuevaCocina({ ...nuevaCocina, password: e.target.value })} required />
            </label>
            <p className="modal-hint">La cocina queda en tu localidad ({localidad?.nombre}). Con esa cuenta el personal ve y prepara los pedidos de todos sus restaurantes.</p>
            <div className="modal-actions">
              <Button variant="secondary" onClick={() => setNuevaCocina(null)}>
                Cancelar
              </Button>
              <Button type="submit" loading={saving} disabled={!nuevaCocina.zonaId}>
                Crear cocina
              </Button>
            </div>
          </form>
        </Modal>
      )}

      {cocinaEditada && (
        <Modal title={`Editar ${cocinaEditada.nombre}`} onClose={() => setCocinaEditada(null)}>
          <form
            className="modal-form"
            onSubmit={(e) =>
              guardar(
                e,
                () => editarCocina(cocinaEditada.id, cocinaEditada),
                () => {
                  notify(`Guardaste los cambios de ${cocinaEditada.nombre}.`, "success");
                  setCocinaEditada(null);
                }
              )
            }
          >
            <label>
              Nombre
              <input value={cocinaEditada.nombre} maxLength={80} onChange={(e) => setCocinaEditada({ ...cocinaEditada, nombre: e.target.value })} required />
            </label>
            <label>
              Dirección
              <input value={cocinaEditada.direccion} maxLength={120} onChange={(e) => setCocinaEditada({ ...cocinaEditada, direccion: e.target.value })} required />
            </label>
            <div className="form-row">
              <label>
                Entrega desde (min)
                <input type="number" min="1" max="240" value={cocinaEditada.tiempoMin} onChange={(e) => setCocinaEditada({ ...cocinaEditada, tiempoMin: e.target.value })} required />
              </label>
              <label>
                hasta (min)
                <input type="number" min="1" max="240" value={cocinaEditada.tiempoMax} onChange={(e) => setCocinaEditada({ ...cocinaEditada, tiempoMax: e.target.value })} required />
              </label>
            </div>
            <label>
              Costo de envío (ARS)
              <input type="number" min="0" step="100" value={cocinaEditada.envio} onChange={(e) => setCocinaEditada({ ...cocinaEditada, envio: e.target.value })} />
            </label>
            <div className="modal-actions">
              <Button variant="secondary" onClick={() => setCocinaEditada(null)}>
                Cancelar
              </Button>
              <Button type="submit" loading={saving}>
                Guardar cambios
              </Button>
            </div>
          </form>
        </Modal>
      )}

      {marcaEditada && (
        <Modal title={`Editar ${marcaEditada.anterior}`} onClose={() => setMarcaEditada(null)}>
          <form
            className="modal-form"
            onSubmit={(e) =>
              guardar(
                e,
                () => editarRestaurante(marcaEditada.cocinaId, marcaEditada.id, marcaEditada.nombre),
                () => {
                  notify(`${marcaEditada.anterior} ahora se llama ${marcaEditada.nombre}.`, "success");
                  setMarcaEditada(null);
                }
              )
            }
          >
            <label>
              Nombre del restaurante
              <input value={marcaEditada.nombre} maxLength={80} onChange={(e) => setMarcaEditada({ ...marcaEditada, nombre: e.target.value })} required />
            </label>
            <div className="modal-actions">
              <Button variant="secondary" onClick={() => setMarcaEditada(null)}>
                Cancelar
              </Button>
              <Button type="submit" loading={saving}>
                Guardar
              </Button>
            </div>
          </form>
        </Modal>
      )}

      {nuevaMarca && (
        <Modal title={`Nuevo restaurante en ${nuevaMarca.cocinaNombre}`} onClose={() => setNuevaMarca(null)}>
          <form
            className="modal-form"
            onSubmit={(e) =>
              guardar(
                e,
                () => crearRestaurante(nuevaMarca.cocinaId, nuevaMarca),
                () => {
                  notify(`Agregaste ${nuevaMarca.nombre} a ${nuevaMarca.cocinaNombre}.`, "success");
                  setNuevaMarca(null);
                }
              )
            }
          >
            <label>
              Nombre del restaurante
              <input value={nuevaMarca.nombre} maxLength={80} onChange={(e) => setNuevaMarca({ ...nuevaMarca, nombre: e.target.value })} required />
            </label>
            <p className="modal-hint">El restaurante usa el stock de {nuevaMarca.cocinaNombre}. Sus pedidos los recibe la cuenta de la cocina.</p>
            <div className="modal-actions">
              <Button variant="secondary" onClick={() => setNuevaMarca(null)}>
                Cancelar
              </Button>
              <Button type="submit" loading={saving}>
                Crear restaurante
              </Button>
            </div>
          </form>
        </Modal>
      )}
    </>
  );
}

export default AdminKitchens;
