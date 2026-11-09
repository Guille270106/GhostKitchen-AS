import Select from "../common/Select";

function CocinaSelector({ cocinas, cocinaId, onChange }) {
  if (cocinas.length <= 1) return null;
  return (
    <Select
      className="cocina-selector"
      icon="store"
      ariaLabel="Elegir cocina"
      value={cocinaId}
      onChange={onChange}
      options={cocinas.map((c) => ({ value: c.id, label: c.nombre }))}
    />
  );
}

export default CocinaSelector;
