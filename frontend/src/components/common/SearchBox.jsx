import { useId, useState } from "react";

import Icon from "./Icon";
import { autocompletePlatos } from "../../api/ghostkitchen";
import { useSession } from "../../context/SessionContext";
import useLiveQuery from "../../hooks/useLiveQuery";

function SearchBox({ initialValue = "", onSearch }) {
  const listId = useId();
  const { localidadId } = useSession();
  const [value, setValue] = useState(initialValue);

  const { data: sugerencias } = useLiveQuery(() => autocompletePlatos(value, { localidadId }), [value, localidadId]);

  const submit = (event) => {
    event.preventDefault();
    onSearch?.(value.trim());
  };

  return (
    <form className="search-box" onSubmit={submit} role="search">
      <Icon name="search" />
      <input
        list={listId}
        aria-label="Buscar comida"
        placeholder="¿Qué tenés ganas de comer?"
        value={value}
        onChange={(event) => setValue(event.target.value)}
      />
      <datalist id={listId}>{(sugerencias || []).map((nombre) => <option key={nombre} value={nombre} />)}</datalist>
      <button type="submit">Buscar</button>
    </form>
  );
}

export default SearchBox;
