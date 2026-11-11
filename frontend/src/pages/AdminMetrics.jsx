import { getMetricas } from "../api/ghostkitchen";
import CocinaSelector from "../components/admin/CocinaSelector";
import EmptyState from "../components/common/EmptyState";
import Loader from "../components/common/Loader";
import Stat from "../components/common/Stat";
import useAdminCocina from "../hooks/useAdminCocina";
import useLiveQuery from "../hooks/useLiveQuery";
import { formatPrice } from "../utils/formatters";

import "../styles/workspace.css";
import "../styles/admin.css";

// Métricas del negocio de una cocina de la localidad del admin, últimos 7 días. Las
// calcula Pedidos con los pedidos guardados.
function AdminMetrics() {
  const { cocinas, cocinaId, cocina, setCocinaId, loading: loadingCocinas } = useAdminCocina();
  const { data, loading, error } = useLiveQuery(() => (cocinaId ? getMetricas(cocinaId) : Promise.resolve(null)), [cocinaId]);

  if (loadingCocinas) return <Loader />;
  if (!cocina) return <EmptyState title="Tu localidad no tiene cocinas" />;
  if (error && !data) return <EmptyState title="No pudimos cargar las métricas" text={error.message} />;
  if (loading || !data) return <Loader />;

  const maximo = Math.max(1, ...data.dias.map((d) => d.cantidad));
  const promedio = data.dias.length ? data.dias.reduce((acc, d) => acc + d.cantidad, 0) / data.dias.length : 0;

  return (
    <>
      <div className="workspace-header">
        <div>
          <h1>Métricas del negocio</h1>
          <p>{cocina.nombre} · Últimos 7 días</p>
        </div>
        <CocinaSelector cocinas={cocinas} cocinaId={cocinaId} onChange={setCocinaId} />
      </div>
      <div className="quick-stats four">
        <Stat label="Pedidos" value={data.pedidos} detail="Sin contar cancelados" />
        <Stat label="Ventas de platos" value={formatPrice(data.facturacion)} detail="Sin envíos ni cancelados" />
        <Stat label="Ticket promedio" value={formatPrice(data.ticketPromedio)} detail="Platos por pedido, sin envío" />
        <Stat label="Cancelados" value={data.cancelados} detail="Pedidos cancelados en la semana" tone="warn" />
      </div>
      <div className="metrics-grid">
        <section className="chart-card">
          <div>
            <h2>Pedidos por día</h2>
            <p>Promedio de {promedio.toFixed(1)} pedidos diarios</p>
          </div>
          <div className="bar-chart">
            {data.dias.map((dia) => (
              <div className="bar-wrap" key={dia.fecha} title={`${dia.fecha}: ${dia.cantidad} pedidos`}>
                <span>{dia.cantidad}</span>
                <i style={{ height: `${(dia.cantidad / maximo) * 100}%` }} />
                <small>{dia.label}</small>
              </div>
            ))}
          </div>
        </section>
        <section className="ranking-card">
          <h2>Platos más vendidos</h2>
          {data.topPlatos.map((plato, i) => (
            <div className="rank-row" key={plato.id}>
              <span>{i + 1}</span>
              {plato.foto ? <img src={plato.foto} alt="" /> : <i />}
              <div>
                <strong>{plato.nombre}</strong>
                <p>{plato.restauranteNombre}</p>
              </div>
              <strong>{plato.vendidos}</strong>
            </div>
          ))}
          {!data.topPlatos.length && <p className="muted">Todavía no hay ventas.</p>}
        </section>
        <section className="brand-metrics">
          <h2>Pedidos por marca</h2>
          {data.porMarca.map((marca) => (
            <div className="brand-bar" key={marca.id}>
              <div>
                <strong>{marca.nombre}</strong>
                <span>{marca.porcentaje}%</span>
              </div>
              <i>
                <span style={{ width: `${marca.porcentaje}%` }} />
              </i>
            </div>
          ))}
          {!data.porMarca.length && <p className="muted">Todavía no hay pedidos.</p>}
          {data.minutosPromedio > 0 && <p className="muted metric-note">Tiempo promedio de entrega: {data.minutosPromedio} min</p>}
        </section>
      </div>
    </>
  );
}

export default AdminMetrics;
