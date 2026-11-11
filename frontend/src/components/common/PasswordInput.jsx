import { useState } from "react";

import Icon from "./Icon";

// Campo de contraseña con un botón para verla mientras se escribe. Recibe las mismas props que
// un <input> (value, onChange, autoComplete, minLength, required...). El botón no envía el
// formulario y dice qué hace para los lectores de pantalla.
function PasswordInput(props) {
  const [visible, setVisible] = useState(false);
  return (
    <span className="password-field" data-campo>
      <input {...props} type={visible ? "text" : "password"} />
      <button
        type="button"
        className="password-toggle"
        onClick={() => setVisible((v) => !v)}
        aria-label={visible ? "Ocultar contraseña" : "Mostrar contraseña"}
        aria-pressed={visible}
        title={visible ? "Ocultar contraseña" : "Mostrar contraseña"}
      >
        <Icon name={visible ? "eyeOff" : "eye"} size={18} />
      </button>
    </span>
  );
}

export default PasswordInput;
