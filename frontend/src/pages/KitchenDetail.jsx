import { useState } from "react";
import { Link, useParams } from "react-router-dom";

import { getCocinaById } from "../api/ghostkitchen";
import EmptyState from "../components/common/EmptyState";
import Icon from "../components/common/Icon";
import Loader from "../components/common/Loader";
import DishCard from "../components/platos/DishCard";
import useLiveQuery from "../hooks/useLiveQuery";
import { formatPrice } from "../utils/formatters";

import "../styles/kitchen.css";

function KitchenDetail() {
  const { id } = useParams();
  const [marca, setMarca] = useState("todas");
  const { data, loading, error, reload } = useLiveQuery(() => getCocinaById(id), [id]);

  if (loading && !data) return <Loader />;
  if (error || !data) {
    return (
      <main className="container page-space">
        <EmptyState title="No encontramos esa cocina" text={error?.message}>
          <Link className="text-link" to="/">
            Volver al inicio
          </Link>
        </EmptyState>
      </main>
    );
  }

  const { cocina, marcas, platos } = data;
  const visibles = marca === "todas" ? marcas : marcas.filter((m) => m.id === marca);

  return (
    <main>
      <section className="kitchen-hero">
        <img src={cocina.foto} alt="" />
        <div className="container kitchen-title">
          <span className="open-pill">Abierto ahora</span>
          <h1>{cocina.nombre}</h1>
          <p>
            <Icon name="pin" size={17} /> {cocina.direccion} · {cocina.tiempo} · Envío {formatPrice(cocina.envio)}
          </p>
        </div>
      </section>
      <div className="container">
        <div className="mix-notice">
          <div className="mix-icon">
            <Icon name="bag" />
          </div>
          <div>
            <strong>Mezclá marcas en un mismo pedido</strong>
            <p>Todos estos platos salen de esta cocina y comparten ingredientes. Pagás un solo envío.</p>
          </div>
          <Icon name="check" />
        </div>
        <div className="brand-tabs" role="tablist">
          <button className={marca === "todas" ? "active" : ""} onClick={() => setMarca("todas")}>
            Todas las marcas
          </button>
          {marcas.map((m) => (
            <button key={m.id} className={marca === m.id ? "active" : ""} onClick={() => setMarca(m.id)}>
              {m.nombre}
            </button>
          ))}
        </div>
        {data.stockSinConfirmar && (
          <p className="search-aviso" role="alert">
            No pudimos confirmar el stock de los platos: se controla cuando los agregás al carrito.{" "}
            <button className="text-link" onClick={reload}>
              Reintentar
            </button>
          </p>
        )}
        {visibles.map((m, index) => {
          const propios = platos.filter((p) => p.restauranteId === m.id);
          return (
            <section className={`section-block ${index === 0 ? "first" : ""}`} key={m.id}>
              <div className="section-heading">
                <div>
                  <p className="eyebrow red">{m.nombre}</p>
                  <h2>{[...new Set(propios.map((p) => p.categoria))].join(" · ") || "Menú"}</h2>
                </div>
                <span className="availability">
                  {data.stockSinConfirmar
                    ? `${propios.length} platos · stock sin confirmar`
                    : `${propios.filter((p) => !p.sinStock && !p.stockSinConfirmar).length} de ${propios.length} disponibles`}
                </span>
              </div>
              {propios.length ? (
                <div className="dish-grid">
                  {propios.map((p) => (
                    <DishCard key={p.id} plato={p} />
                  ))}
                </div>
              ) : (
                <p className="muted">Esta marca todavía no cargó platos.</p>
              )}
            </section>
          );
        })}
        <div className="last-section" />
      </div>
    </main>
  );
}

export default KitchenDetail;
