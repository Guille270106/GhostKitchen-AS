import { NavLink } from "react-router-dom";

import { useSession } from "../../context/SessionContext";
import Icon from "../common/Icon";

// Navegacion inferior para celulares (solo cliente y visitantes).
function BottomNav() {
  const { user } = useSession();
  if (user && user.rol !== "CLIENTE") return null;

  return (
    <nav className="bottom-nav" aria-label="Navegación inferior">
      <NavLink to="/" end>
        <Icon name="home" />
        <span>Inicio</span>
      </NavLink>
      <NavLink to="/buscar">
        <Icon name="search" />
        <span>Buscar</span>
      </NavLink>
      <NavLink to={user ? "/pedidos" : "/login"}>
        <Icon name="receipt" />
        <span>Pedidos</span>
      </NavLink>
      <NavLink to="/carrito">
        <Icon name="bag" />
        <span>Carrito</span>
      </NavLink>
    </nav>
  );
}

export default BottomNav;
