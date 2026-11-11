function Loader({ text = "Cargando…" }) {
  return (
    <div className="loader" role="status">
      <span className="spinner dark" aria-hidden="true" />
      {text}
    </div>
  );
}

export default Loader;
