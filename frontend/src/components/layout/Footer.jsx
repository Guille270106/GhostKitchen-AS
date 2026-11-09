import { Link } from "react-router-dom";

import { useSession } from "../../context/SessionContext";
import GhostLogo from "../common/GhostLogo";

function Footer() {
  const { user } = useSession();
  if (user && user.rol !== "CLIENTE") return null;

  return (
    <footer className="site-footer">
      <div className="container footer-grid">
        <div className="footer-brand">
          <GhostLogo />
          <p>Pedidos a cocinas fantasma de Córdoba. Varias marcas, una misma cocina, un solo envío.</p>
        </div>
        <div>
          <h4>Pedir</h4>
          <ul>
            <li>
              <Link to="/">Cocinas cerca tuyo</Link>
            </li>
            <li>
              <Link to="/buscar">Buscar platos</Link>
            </li>
            <li>
              <Link to={user ? "/pedidos" : "/login"}>Mis pedidos</Link>
            </li>
          </ul>
        </div>
        <div>
          <h4>Tu cuenta</h4>
          <ul>
            <li>
              <Link to="/login">Ingresar</Link>
            </li>
            <li>
              <Link to="/registro">Crear una cuenta</Link>
            </li>
          </ul>
        </div>
        <div>
          <h4>Para cocinas</h4>
          <ul>
            <li>
              <Link to="/login">Panel de la cocina</Link>
            </li>
            <li>
              <Link to="/login">Panel del administrador</Link>
            </li>
          </ul>
        </div>
      </div>
      <div className="container footer-bottom">
        <span>© 2026 GhostKitchen · Práctico Integrador de Arquitectura de Software, UCC</span>
        <span>Hecho en Córdoba</span>
      </div>
    </footer>
  );
}

export default Footer;
