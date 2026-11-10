import { NavLink, Outlet } from "react-router-dom";

import { useSession } from "../../context/SessionContext";
import { initials } from "../../utils/formatters";
import Icon from "../common/Icon";

import "../../styles/workspace.css";

const MENUS = {
  ADMIN: [
    { to: "/admin/cocinas", icon: "store", label: "Cocinas y marcas" },
    { to: "/admin/metricas", icon: "chart", label: "Métricas" },
  ],
  COCINA: [
    { to: "/cocina", icon: "receipt", label: "Pedidos", end: true },
    { to: "/cocina/menu", icon: "utensils", label: "Menú" },
    { to: "/cocina/platos", icon: "recipe", label: "Platos y recetas" },
    { to: "/cocina/inventario", icon: "box", label: "Inventario" },
  ],
};

// Estructura comun de los paneles de la cocina y del admin: barra lateral + contenido.
function WorkspaceLayout() {
  const { user } = useSession();
  const items = MENUS[user?.rol] || [];

  return (
    <main className="workspace">
      <aside className="workspace-sidebar">
        <div className="sidebar-brand">
          <span>{initials(user?.nombre)}</span>
          <div>
            <strong>{user?.nombre}</strong>
            <small>{user?.rol === "ADMIN" ? "Administrador" : "Cocina"}</small>
          </div>
        </div>
        <nav>
          {items.map((item) => (
            // Con la pantalla angosta el menú muestra solo los íconos: el nombre aparece en un
            // cartel al pasar el mouse o al llegar con Tab (data-nombre, ver layout.css), y
            // aria-label lo da a los lectores de pantalla aunque el texto esté oculto.
            <NavLink key={item.to} to={item.to} end={item.end} aria-label={item.label} data-nombre={item.label}>
              <Icon name={item.icon} />
              <span>{item.label}</span>
            </NavLink>
          ))}
        </nav>
      </aside>
      <div className="workspace-main">
        <Outlet />
      </div>
    </main>
  );
}

export default WorkspaceLayout;
