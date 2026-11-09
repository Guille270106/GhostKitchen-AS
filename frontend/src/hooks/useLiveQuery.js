import { useCallback, useEffect, useRef, useState } from "react";

import { subscribe } from "../api/ghostkitchen";

// Ejecuta una consulta a la capa api y la vuelve a ejecutar cuando cambian
// los datos (por ejemplo, cuando otra pantalla reserva stock o cambia un pedido).
export default function useLiveQuery(query, deps = []) {
  const [state, setState] = useState({ data: null, loading: true, error: null });
  const queryRef = useRef(query);

  useEffect(() => {
    queryRef.current = query;
  });

  const run = useCallback(async () => {
    try {
      const data = await queryRef.current();
      setState({ data, loading: false, error: null });
    } catch (error) {
      setState((prev) => ({ data: prev.data, loading: false, error }));
    }
  }, []);

  useEffect(() => {
    let active = true;
    const execute = async () => {
      try {
        const data = await queryRef.current();
        if (active) setState({ data, loading: false, error: null });
      } catch (error) {
        if (active) setState((prev) => ({ data: prev.data, loading: false, error }));
      }
    };
    execute();
    const unsubscribe = subscribe(execute);
    return () => {
      active = false;
      unsubscribe();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  return { ...state, reload: run };
}
