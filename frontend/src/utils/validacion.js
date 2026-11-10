// Mensajes de validación con el estilo de la página, para TODOS los formularios (sin tocar cada
// uno). Los formularios siguen usando lo que ya tienen: required, type="email", min, max,
// minLength, step. Acá se reemplaza el cartelito del navegador ("Completa este campo", distinto
// en cada navegador) por un mensaje propio debajo del campo, en castellano y con el campo
// marcado en rojo.
//
// Además, los mails se controlan mejor que con type="email" solo: el navegador acepta
// "ana@gmail" (sin ".com"), y acá se pide que el dominio tenga un punto.

const MAIL = /^[^\s@]+@[^\s@.]+(\.[^\s@.]+)*\.[^\s@.]{2,}$/;

export function mailValido(texto) {
  return MAIL.test(String(texto).trim());
}

// Qué decir según qué falló. Se arma con los datos del propio campo (mínimo, máximo, largo).
export function mensajePara(campo) {
  const v = campo.validity;
  if (v.valueMissing) {
    return campo.tagName === "SELECT" ? "Elegí una opción." : "Completá este campo.";
  }
  if (v.customError) return campo.validationMessage;
  if (v.typeMismatch && campo.type === "email") return "Escribí un mail válido, por ejemplo nombre@gmail.com.";
  if (v.typeMismatch) return "El formato no es válido.";
  if (v.tooShort) return `Tiene que tener al menos ${campo.minLength} caracteres (tiene ${campo.value.length}).`;
  if (v.tooLong) return `Puede tener hasta ${campo.maxLength} caracteres.`;
  if (v.rangeUnderflow) return `Tiene que ser ${campo.min} o más.`;
  if (v.rangeOverflow) return `Tiene que ser ${campo.max} o menos.`;
  if (v.stepMismatch) return "Usá un número entero.";
  if (v.badInput) return "Escribí un número.";
  if (v.patternMismatch) return campo.title || "El formato no es válido.";
  return "Revisá este campo.";
}

// Los mails se revisan con la regla de arriba además de la del navegador.
function revisarMail(campo) {
  if (campo.type !== "email") return;
  const valor = campo.value.trim();
  campo.setCustomValidity(valor && !mailValido(valor) ? "Escribí un mail válido, por ejemplo nombre@gmail.com." : "");
}

let siguienteId = 0;

function mostrar(campo) {
  let aviso = campo.nextElementSibling;
  if (!aviso || !aviso.classList.contains("campo-error")) {
    aviso = document.createElement("span");
    aviso.className = "campo-error";
    aviso.id = `campo-error-${++siguienteId}`;
    aviso.setAttribute("role", "alert");
    campo.insertAdjacentElement("afterend", aviso);
  }
  aviso.textContent = mensajePara(campo);
  campo.classList.add("campo-invalido");
  campo.setAttribute("aria-invalid", "true");
  campo.setAttribute("aria-describedby", aviso.id);
}

function ocultar(campo) {
  const aviso = campo.nextElementSibling;
  if (aviso && aviso.classList.contains("campo-error")) aviso.remove();
  campo.classList.remove("campo-invalido");
  campo.removeAttribute("aria-invalid");
  campo.removeAttribute("aria-describedby");
}

let enfocado = false;

function alFallar(evento) {
  const campo = evento.target;
  if (!(campo instanceof HTMLElement) || !campo.form) return;
  evento.preventDefault(); // sin el cartel del navegador
  mostrar(campo);
  // Solo el primer campo con error de cada envío recibe el foco y queda a la vista.
  if (!enfocado) {
    enfocado = true;
    campo.focus({ preventScroll: true });
    campo.scrollIntoView({ block: "center", behavior: "smooth" });
    // setTimeout y no una microtarea: el navegador avisa cada campo inválido en un evento
    // aparte, y entre uno y otro corren las microtareas.
    setTimeout(() => {
      enfocado = false;
    }, 0);
  }
}

function alEscribir(evento) {
  const campo = evento.target;
  if (!(campo instanceof HTMLInputElement || campo instanceof HTMLTextAreaElement || campo instanceof HTMLSelectElement)) return;
  revisarMail(campo);
  // El mensaje se actualiza mientras se corrige y desaparece cuando el campo ya está bien.
  if (campo.classList.contains("campo-invalido")) {
    if (campo.checkValidity()) ocultar(campo);
    else mostrar(campo);
  }
}

// Antes de enviar se revisan los mails (por si se completaron con el autocompletado del
// navegador, que no siempre dispara "input").
function antesDeEnviar(evento) {
  const form = evento.target;
  if (!(form instanceof HTMLFormElement)) return;
  form.querySelectorAll('input[type="email"]').forEach(revisarMail);
}

let activa = false;

// Se llama una vez, en main.jsx.
export function activarValidacion() {
  if (activa || typeof document === "undefined") return;
  activa = true;
  document.addEventListener("invalid", alFallar, true);
  document.addEventListener("input", alEscribir, true);
  document.addEventListener("change", alEscribir, true);
  document.addEventListener("click", (evento) => {
    const boton = evento.target instanceof Element ? evento.target.closest('button[type="submit"], input[type="submit"]') : null;
    if (boton?.form) antesDeEnviar({ target: boton.form });
  }, true);
  document.addEventListener("keydown", (evento) => {
    if (evento.key === "Enter" && evento.target instanceof HTMLInputElement && evento.target.form) antesDeEnviar({ target: evento.target.form });
  }, true);
}
