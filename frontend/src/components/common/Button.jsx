import Icon from "./Icon";

function Button({ children, variant = "primary", icon, onClick, className = "", disabled = false, type = "button", loading = false }) {
  return (
    <button type={type} className={`btn btn-${variant} ${className}`} onClick={onClick} disabled={disabled || loading}>
      {loading ? <span className="spinner" aria-hidden="true" /> : icon && <Icon name={icon} size={18} />}
      {children}
    </button>
  );
}

export default Button;
