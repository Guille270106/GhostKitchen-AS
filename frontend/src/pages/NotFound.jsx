import { Link } from "react-router-dom";

import EmptyState from "../components/common/EmptyState";

function NotFound() {
  return (
    <main className="container page-space">
      <EmptyState title="Esta página se esfumó" text="Puede que el enlace esté mal o que la página ya no exista.">
        <Link className="btn btn-primary" to="/">
          Volver al inicio
        </Link>
      </EmptyState>
    </main>
  );
}

export default NotFound;
