import { useState } from "react";
import { Link, Navigate, useLocation, useNavigate } from "react-router-dom";

import AuthLayout from "../components/auth/AuthLayout";
import Button from "../components/common/Button";
import { useSession } from "../context/SessionContext";
import { CUENTAS_DEMO } from "../data/visual";
import { homeForRole } from "../utils/auth";

import "../styles/auth.css";

function Login() {
  const { user, login } = useSession();
  const navigate = useNavigate();
  const location = useLocation();
  const [form, setForm] = useState({ email: "", password: "" });
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  if (user) return <Navigate to={homeForRole(user.rol)} replace />;

  const submit = async (event) => {
    event.preventDefault();
    setError("");
    setLoading(true);
    try {
      const { usuario } = await login(form);
      const from = location.state?.from?.pathname;
      navigate(from && usuario.rol === "CLIENTE" ? from : homeForRole(usuario.rol), { replace: true });
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <AuthLayout>
      <h2>Ingresá a GhostKitchen</h2>
      <p>La misma pantalla sirve para clientes, cocinas y administradores.</p>
      <form onSubmit={submit}>
        <label>
          Email
          <input type="email" autoComplete="email" placeholder="vos@email.com" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} required />
        </label>
        <label>
          Contraseña
          <input type="password" autoComplete="current-password" placeholder="••••••••" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} required />
        </label>
        {error && <p className="form-error">{error}</p>}
        <Button type="submit" className="full-button" loading={loading}>
          Ingresar
        </Button>
      </form>
      <p className="auth-toggle">
        ¿Todavía no tenés cuenta? <Link to="/registro">Registrate</Link>
      </p>

      <div className="demo-accounts">
        <strong>Cuentas de demostración</strong>
        <small>Completa el email; la contraseña de los admins es la de ADMIN_PASSWORD del .env.</small>
        {CUENTAS_DEMO.map((cuenta) => (
          <button key={cuenta.email} type="button" onClick={() => setForm({ ...form, email: cuenta.email })}>
            <span>{cuenta.rol}</span>
            <small>{cuenta.email}</small>
          </button>
        ))}
      </div>
    </AuthLayout>
  );
}

export default Login;
