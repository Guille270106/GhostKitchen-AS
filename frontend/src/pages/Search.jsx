import { useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";

import { searchPlatos } from "../api/ghostkitchen";
import EmptyState from "../components/common/EmptyState";
import Icon from "../components/common/Icon";
import Loader from "../components/common/Loader";
import SearchBox from "../components/common/SearchBox";
import Select from "../components/common/Select";
import DishCard from "../components/platos/DishCard";
import { useSession } from "../context/SessionContext";
import { ETIQUETAS } from "../data/visual";
import useLiveQuery from "../hooks/useLiveQuery";
import { formatPrice } from "../utils/formatters";

import "../styles/search.css";


function Search() {
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const { localidadId } = useSession();
  const [showFilters, setShowFilters] = useState(false);

  const filtros = {
    q: params.get("q") || "",
    cocinaId: params.get("cocina") || "",
    restauranteIds: params.get("marcas") ? params.get("marcas").split(",") : [],
    categoria: params.get("categoria") || "",
    etiqueta: params.get("etiqueta") || "",
    precioMax: params.get("precioMax") || "",
    soloDisponibles: params.get("disponibles") === "1",
    orden: params.get("orden") || "relevancia",
    page: Number(params.get("page")) || 1,
  };

  const { data, loading, error, reload } = useLiveQuery(() => searchPlatos({ ...filtros, localidadId }), [params.toString(), localidadId]);

  const update = (changes) => {
    const next = new URLSearchParams(params);
    Object.entries(changes).forEach(([key, value]) => {
      if (value === "" || value === null || value === undefined || value === false) next.delete(key);
      else next.set(key, value === true ? "1" : value);
    });
    if (!("page" in changes)) next.delete("page");
    setParams(next);
  };

  const toggleMarca = (id) => {
    const set = new Set(filtros.restauranteIds);
    if (set.has(id)) set.delete(id);
    else set.add(id);
    update({ marcas: [...set].join(",") });
  };

  const titulo = filtros.q ? `Resultados para "${filtros.q}"` : filtros.categoria || filtros.etiqueta || "Todos los platos";
  const facetas = data?.facetas;
  const precioTope = facetas?.precioMaximo || 20000;

  return (
    <main className="container page-space results-page">
      {data?.toleranciaAplicada && <p>Mostramos coincidencias aproximadas porque no hubo resultados con el texto original.</p>}
      <div className="results-top">
        <div>
          <button className="back-link" onClick={() => navigate(-1)}>
            ← Volver
          </button>
          <h1>{titulo}</h1>
          <p>
            {error && !data
              ? "La búsqueda no está disponible"
              : data
                ? `${data.total} resultado${data.total === 1 ? "" : "s"} en tu sede`
                : "Buscando…"}
          </p>
        </div>
        <SearchBox
          key={filtros.q}
          initialValue={filtros.q}
          onSearch={(q) => update({ q })}
          onCategoria={(c) => update({ q: "", categoria: c.esEtiqueta ? "" : c.id, etiqueta: c.esEtiqueta ? c.id : "" })}
        />
      </div>

      <div className="results-layout">
        <aside className={`filters ${showFilters ? "open" : ""}`}>
          <div className="filter-title">
            <h3>Filtros</h3>
            <button onClick={() => setParams(filtros.q ? { q: filtros.q } : {})}>Limpiar</button>
          </div>
          <label className="switch-row">
            <span>Solo disponibles</span>
            <input type="checkbox" checked={filtros.soloDisponibles} onChange={(e) => update({ disponibles: e.target.checked })} />
            <i />
          </label>

          {facetas?.cocinas.length > 1 && (
            <div className="filter-section">
              <h4>Cocina</h4>
              <Select
                ariaLabel="Filtrar por cocina"
                value={filtros.cocinaId}
                onChange={(value) => update({ cocina: value })}
                options={[{ value: "", label: "Todas" }, ...facetas.cocinas.map((c) => ({ value: c.id, label: c.nombre }))]}
              />
            </div>
          )}

          <div className="filter-section">
            <h4>Marca</h4>
            {(facetas?.restaurantes || []).map((r) => (
              <label className="check-row" key={r.id}>
                <input type="checkbox" checked={filtros.restauranteIds.includes(r.id)} onChange={() => toggleMarca(r.id)} />
                <span>{r.nombre}</span>
                <small>{r.cantidad}</small>
              </label>
            ))}
          </div>

          <div className="filter-section">
            <h4>Categoría</h4>
            <label className="check-row">
              <input type="radio" name="categoria" checked={!filtros.categoria} onChange={() => update({ categoria: "" })} />
              <span>Todas</span>
            </label>
            {(facetas?.categorias || []).map((c) => (
              <label className="check-row" key={c}>
                <input type="radio" name="categoria" checked={filtros.categoria === c} onChange={() => update({ categoria: c })} />
                <span>{c}</span>
              </label>
            ))}
          </div>

          <div className="filter-section">
            <h4>Etiquetas</h4>
            {ETIQUETAS.map((e) => (
              <label className="check-row" key={e}>
                <input type="checkbox" checked={filtros.etiqueta === e} onChange={() => update({ etiqueta: filtros.etiqueta === e ? "" : e })} />
                <span>{e}</span>
              </label>
            ))}
          </div>

          <div className="filter-section">
            <h4>Precio máximo</h4>
            <input
              className="range"
              type="range"
              min="5000"
              max={Math.max(precioTope, 5000)}
              step="500"
              value={filtros.precioMax || Math.max(precioTope, 5000)}
              onChange={(e) => update({ precioMax: Number(e.target.value) >= precioTope ? "" : e.target.value })}
            />
            <div className="price-inputs">
              <span>Hasta {formatPrice(filtros.precioMax || precioTope)}</span>
            </div>
          </div>
        </aside>

        <section>
          <div className="sort-row">
            <button className="mobile-filter" onClick={() => setShowFilters(!showFilters)}>
              <Icon name="filter" /> {showFilters ? "Ocultar filtros" : "Filtros"}
            </button>
            <span>Ordenar por</span>
            <Select
              className="sort-select"
              ariaLabel="Ordenar resultados"
              value={filtros.orden}
              onChange={(value) => update({ orden: value })}
              options={[
                { value: "relevancia", label: "Más relevantes" },
                { value: "precio-asc", label: "Menor precio" },
                { value: "precio-desc", label: "Mayor precio" },
              ]}
            />
          </div>

          {loading && !data && !error && <Loader />}
          {error && !data && (
            <EmptyState title="La búsqueda no está disponible" text={`${error.message} Probá de nuevo en unos segundos.`}>
              <button className="text-link" onClick={reload}>
                Reintentar
              </button>
            </EmptyState>
          )}
          {error && data && (
            <p className="search-aviso" role="alert">
              No pudimos actualizar la búsqueda: estos son los resultados anteriores.{" "}
              <button className="text-link" onClick={reload}>
                Reintentar
              </button>
            </p>
          )}
          {data && !data.items.length && (
            <EmptyState title="No encontramos platos con esos filtros" text="Probá con otra palabra o sacá algún filtro.">
              <button className="text-link" onClick={() => setParams({})}>
                Ver todos los platos
              </button>
            </EmptyState>
          )}
          {data?.items.length > 0 && (
            <>
              <div className="dish-grid results-grid">
                {data.items.map((plato) => (
                  <DishCard key={plato.id} plato={plato} />
                ))}
              </div>
              {data.pages > 1 && (
                <nav className="pagination" aria-label="Paginación">
                  <button disabled={data.page <= 1} onClick={() => update({ page: data.page - 1 })}>
                    <Icon name="back" size={16} /> Anterior
                  </button>
                  <span>
                    Página {data.page} de {data.pages}
                  </span>
                  <button disabled={data.page >= data.pages} onClick={() => update({ page: data.page + 1 })}>
                    Siguiente <Icon name="chevron" size={16} />
                  </button>
                </nav>
              )}
            </>
          )}
        </section>
      </div>
    </main>
  );
}

export default Search;
