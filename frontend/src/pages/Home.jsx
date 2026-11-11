import { Link, useNavigate } from "react-router-dom";

import { getCocinas, getPlatosDestacados } from "../api/ghostkitchen";
import EmptyState from "../components/common/EmptyState";
import Icon from "../components/common/Icon";
import Loader from "../components/common/Loader";
import SearchBox from "../components/common/SearchBox";
import DishCard from "../components/platos/DishCard";
import { useSession } from "../context/SessionContext";
import { categorias, photos } from "../data/visual";
import useLiveQuery from "../hooks/useLiveQuery";
import { formatPrice } from "../utils/formatters";

import "../styles/home.css";

const PASOS = [
  { icon: "store", titulo: "Elegí tu sede", texto: "Escribí tu zona y te sugerimos la más cercana de nuestras 4 cocinas." },
  { icon: "bag", titulo: "Combiná restaurantes", texto: "Sumá platos de todos los restaurantes de tu cocina en un mismo pedido. Te guardamos los ingredientes 10 minutos." },
  { icon: "mail", titulo: "Recibilo todo junto", texto: "La cocina prepara todo, sale en un solo envío y te avisamos por mail cada vez que cambia el estado." },
];

function Home() {
  const navigate = useNavigate();
  const { localidadId, abrirSelectorSede } = useSession();
  const { data: cocinas, loading } = useLiveQuery(() => getCocinas({ localidadId }), [localidadId]);
  const { data: destacados } = useLiveQuery(() => getPlatosDestacados({ localidadId, limit: 8 }), [localidadId]);

  const sede = cocinas?.[0];

  const buscarCategoria = (categoria) => {
    const params = new URLSearchParams(categoria.esEtiqueta ? { etiqueta: categoria.id } : { categoria: categoria.id });
    navigate(`/buscar?${params}`);
  };

  return (
    <main>
      <section className="hero" style={{ backgroundImage: `url(${photos.feast})` }}>
        <div className="container hero-content">
          <h1>Pedí de varios restaurantes en un solo envío</h1>
          <p>Estás pidiendo en {sede?.nombre || "tu sede"}. Combiná {sede ? sede.marcas.map((m) => m.nombre).join(", ") : "sus restaurantes"} en el mismo pedido.</p>
          <SearchBox onSearch={(q) => navigate(q ? `/buscar?q=${encodeURIComponent(q)}` : "/buscar")} onCategoria={buscarCategoria} />
          <p className="hero-note">
            <Icon name="pin" size={15} /> {sede ? `${sede.nombre} · ${sede.direccion}` : "Buscando tu cocina"} ·{" "}
            <button type="button" className="link-button" onClick={abrirSelectorSede}>
              Cambiar sede
            </button>
          </p>
        </div>
      </section>

      <section className="category-band">
        <div className="container category-row">
          {categorias.map((categoria) => (
            <button key={categoria.id} className="category" onClick={() => buscarCategoria(categoria)}>
              <img className="category-art" src={categoria.foto} alt="" />
              <span>{categoria.label}</span>
            </button>
          ))}
        </div>
      </section>

      <div className="container page-space">
        <section>
          <div className="section-heading">
            <div>
              <h2>Tu cocina</h2>
              <p>Todos estos restaurantes salen de la misma cocina: armá un solo pedido con lo que quieras.</p>
            </div>
          </div>
          {loading && <Loader />}
          {!loading && !cocinas?.length && <EmptyState title="Esta sede no está disponible ahora" text="Elegí otra sede desde la barra de arriba." />}
          {cocinas?.length > 0 && (
            <div className="kitchens-grid">
              {cocinas.map((cocina) => (
                <Link className="kitchen-card" key={cocina.id} to={`/cocinas/${cocina.id}`}>
                  <img src={cocina.foto} alt="" />
                  <div>
                    <h3>{cocina.nombre}</h3>
                    <p className="kitchen-brands">{cocina.marcas.map((m) => m.nombre).join(" · ")}</p>
                    <p className="kitchen-meta">
                      <span className="open-dot">Abierto</span>
                      <span>
                        <Icon name="clock" size={14} /> {cocina.tiempo}
                      </span>
                      <span>Envío {formatPrice(cocina.envio)}</span>
                    </p>
                  </div>
                </Link>
              ))}
            </div>
          )}
        </section>

        <section className="how-it-works">
          <h2>¿Cómo funciona?</h2>
          <ol>
            {PASOS.map((paso, i) => (
              <li key={paso.titulo}>
                <span className="step-number">{i + 1}</span>
                <div>
                  <h3>
                    <Icon name={paso.icon} size={18} /> {paso.titulo}
                  </h3>
                  <p>{paso.texto}</p>
                </div>
              </li>
            ))}
          </ol>
        </section>

        {destacados?.length > 0 && (
          <section className="section-block">
            <div className="section-heading">
              <div>
                <h2>Lo más pedido</h2>
                <p>Los platos que más salen esta semana.</p>
              </div>
              <Link className="text-link" to="/buscar">
                Ver todos los platos <Icon name="chevron" size={16} />
              </Link>
            </div>
            <div className="dish-grid featured-grid">
              {destacados.map((plato) => (
                <DishCard key={plato.id} plato={plato} />
              ))}
            </div>
          </section>
        )}
      </div>
    </main>
  );
}

export default Home;
