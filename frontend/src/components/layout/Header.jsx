import { useState } from "react";
import { Link, NavLink, useNavigate } from "react-router-dom";

import { getCocinas } from "../../api/ghostkitchen";
import { useCart } from "../../context/CartContext";
import { useSession } from "../../context/SessionContext";
import useLiveQuery from "../../hooks/useLiveQuery";
import { homeForRole } from "../../utils/auth";
import GhostLogo from "../common/GhostLogo";
import Icon from "../common/Icon";

const ROLE_LABELS = { CLIENTE: "Cliente", COCINA: "Cocina", ADMIN: "Admin de cocina" };

function Header() {
  const { user, logout, localidadId, tieneSede, abrirSelectorSede } = useSession();
  const { count } = useCart();
  const navigate = useNavigate();
  const [mobileOpen, setMobileOpen] = useState(false);
  const { data: cocinas } = useLiveQuery(() => (localidadId ? getCocinas({ localidadId }) : Promise.resolve([])), [localidadId]);

  const isWorkspace = user && user.rol !== "CLIENTE";
  const sede = cocinas?.[0];

  const handleLogout = () => {
    logout();
    setMobileOpen(false);
    navigate("/");
  };

  return (
    <header className="topbar">
      <div className="nav-shell">
        <Link className="logo-button" to={homeForRole(user?.rol)} aria-label="Ir al inicio">
          <GhostLogo />
        </Link>

        {!isWorkspace && (
          <nav className="desktop-nav" aria-label="Navegación principal">
            <NavLink to="/" end>
              Inicio
            </NavLink>
            {user && <NavLink to="/pedidos">Mis pedidos</NavLink>}
          </nav>
        )}

        <div className="nav-actions">
          {!isWorkspace && (
            <button type="button" className="location" onClick={abrirSelectorSede} title="Cambiar de sede">
              <Icon name="pin" size={18} />
              <span>{tieneSede ? sede?.nombre : "Elegí tu sede"}</span>
            </button>
          )}

          {user ? (
            <div className="user-chip">
              <Icon name={user.rol === "CLIENTE" ? "user" : user.rol === "COCINA" ? "store" : "box"} size={17} />
              <div>
                <strong>{user.nombre}</strong>
                <small>{ROLE_LABELS[user.rol]}</small>
              </div>
              <button onClick={handleLogout} aria-label="Cerrar sesión" title="Cerrar sesión">
                <Icon name="logout" size={18} />
              </button>
            </div>
          ) : (
            <Link className="btn btn-secondary login-link" to="/login">
              Ingresar
            </Link>
          )}

          {!isWorkspace && (
            <Link className="cart-button" to="/carrito" aria-label={`Carrito con ${count} productos`}>
              <Icon name="bag" size={18} />
              <em className="cart-label">Mi pedido</em>
              {count > 0 && <span>{count}</span>}
            </Link>
          )}

          <button className="mobile-menu" onClick={() => setMobileOpen(!mobileOpen)} aria-label="Abrir menú">
            <Icon name={mobileOpen ? "close" : "menu"} />
          </button>
        </div>
      </div>

      {mobileOpen && (
        <div className="mobile-panel" onClick={() => setMobileOpen(false)}>
          {!isWorkspace && <Link to="/">Inicio</Link>}
          {!isWorkspace && <button onClick={abrirSelectorSede}>Cambiar sede</button>}
          {user?.rol === "CLIENTE" && <Link to="/pedidos">Mis pedidos</Link>}
          {user ? <button onClick={handleLogout}>Cerrar sesión</button> : <Link to="/login">Ingresar</Link>}
        </div>
      )}
    </header>
  );
}

export default Header;
