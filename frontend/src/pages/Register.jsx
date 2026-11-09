import { useState } from "react";
import { Link, Navigate, useNavigate } from "react-router-dom";

import AuthLayout from "../components/auth/AuthLayout";
import Button from "../components/common/Button";
import { useSession } from "../context/SessionContext";
import { useToast } from "../context/ToastContext";
import { homeForRole } from "../utils/auth";

import "../styles/auth.css";

function Register() {
  const { user, register } = useSession();
  const { notify } = useToast();
  const navigate = useNavigate();
  const [form, setForm] = useState({ nombre: "", email: "", password: "", direccion: "" });
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  if (user) return <Navigate to={homeForRole(user.rol)} replace />;

  const change = (field) => (event) => setForm({ ...form, [field]: event.target.value });

  const submit = async (event) => {
    event.preventDefault();
    setError("");
    setLoading(true);
    try {
      await register(form);
      notify("¡Bienvenido/a! Ya podés armar tu primer pedido.", "success");
      navigate("/", { replace: true });
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <AuthLayout>
      <h2>Empezá a pedir</h2>
      <p>Creá tu cuenta y elegí desde qué cocina pedir cada vez: podés cambiarla cuando quieras.</p>
      <form onSubmit={submit}>
        <label>
          Nombre
          <input autoComplete="name" placeholder="Tu nombre" value={form.nombre} onChange={change("nombre")} required />
        </label>
        <label>
          Email
          <input type="email" autoComplete="email" placeholder="vos@email.com" value={form.email} onChange={change("email")} required />
        </label>
        <label>
          Contraseña
          <input type="password" autoComplete="new-password" placeholder="Mínimo 8 caracteres" minLength={8} value={form.password} onChange={change("password")} required />
        </label>
        <label>
          Dirección
          <input autoComplete="street-address" placeholder="Calle y número, para la entrega" value={form.direccion} onChange={change("direccion")} required />
        </label>
        {error && <p className="form-error">{error}</p>}
        <Button type="submit" className="full-button" loading={loading}>
          Crear cuenta
        </Button>
      </form>
      <p className="auth-toggle">
        ¿Ya tenés cuenta? <Link to="/login">Ingresá</Link>
      </p>
    </AuthLayout>
  );
}

export default Register;
