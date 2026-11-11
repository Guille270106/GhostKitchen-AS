import { useEffect, useId, useRef, useState } from "react";

import Icon from "./Icon";

// Desplegable propio: reemplaza al <select> nativo para que se vea igual en todos
// los navegadores. Se puede usar con mouse o con teclado (flechas, Enter, Escape).
function Select({ value, onChange, options, placeholder = "Elegí una opción", ariaLabel, icon, variant = "field", className = "", disabled = false }) {
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(-1);
  const rootRef = useRef(null);
  const listRef = useRef(null);
  const listId = useId();

  const selected = options.find((o) => String(o.value) === String(value));

  useEffect(() => {
    if (!open) return undefined;
    const onClick = (event) => {
      if (!rootRef.current?.contains(event.target)) setOpen(false);
    };
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [open]);

  useEffect(() => {
    if (open && active >= 0) {
      listRef.current?.children[active]?.scrollIntoView({ block: "nearest" });
    }
  }, [open, active]);

  const openList = () => {
    if (disabled) return;
    setActive(Math.max(0, options.findIndex((o) => String(o.value) === String(value))));
    setOpen(true);
  };

  const choose = (option) => {
    if (option.disabled) return;
    onChange(option.value);
    setOpen(false);
  };

  const move = (step) => {
    let next = active;
    for (let i = 0; i < options.length; i += 1) {
      next = (next + step + options.length) % options.length;
      if (!options[next].disabled) break;
    }
    setActive(next);
  };

  const onKeyDown = (event) => {
    if (!open && ["ArrowDown", "ArrowUp", "Enter", " "].includes(event.key)) {
      event.preventDefault();
      openList();
      return;
    }
    if (!open) return;
    if (event.key === "ArrowDown") {
      event.preventDefault();
      move(1);
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      move(-1);
    } else if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      if (options[active]) choose(options[active]);
    } else if (event.key === "Escape" || event.key === "Tab") {
      setOpen(false);
    }
  };

  return (
    <div className={`gk-select gk-select-${variant} ${open ? "is-open" : ""} ${disabled ? "is-disabled" : ""} ${className}`} ref={rootRef}>
      <button
        type="button"
        className="gk-select-trigger"
        onClick={() => (open ? setOpen(false) : openList())}
        onKeyDown={onKeyDown}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={listId}
        aria-label={ariaLabel}
        disabled={disabled}
      >
        {icon && <Icon name={icon} size={17} />}
        <span className={selected ? "" : "is-placeholder"}>{selected ? selected.label : placeholder}</span>
        <svg className="gk-select-caret" width="12" height="12" viewBox="0 0 12 12" aria-hidden="true">
          <path d="M2.5 4.5 6 8l3.5-3.5" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      </button>
      {open && (
        <ul className="gk-select-list" role="listbox" id={listId} ref={listRef} aria-label={ariaLabel}>
          {options.map((option, index) => {
            const isSelected = String(option.value) === String(value);
            return (
              <li
                key={String(option.value)}
                role="option"
                aria-selected={isSelected}
                aria-disabled={option.disabled || undefined}
                className={`${index === active ? "is-active" : ""} ${isSelected ? "is-selected" : ""} ${option.disabled ? "is-disabled" : ""}`}
                onMouseEnter={() => setActive(index)}
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => choose(option)}
              >
                <span>{option.label}</span>
                {isSelected && <Icon name="check" size={15} />}
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

export default Select;
