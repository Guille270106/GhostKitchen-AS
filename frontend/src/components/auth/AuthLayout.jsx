import GhostLogo from "../common/GhostLogo";
import { photos } from "../../data/visual";

function AuthLayout({ children }) {
  return (
    <main className="auth-page">
      <section className="auth-visual">
        <img src={photos.feast} alt="" />
        <div>
          <GhostLogo compact />
          <h1>
            Todo lo que te gusta,
            <br />
            en un solo pedido.
          </h1>
          <p>Distintas marcas, una misma cocina.</p>
        </div>
      </section>
      <section className="auth-form">
        <div className="auth-box">{children}</div>
      </section>
    </main>
  );
}

export default AuthLayout;
