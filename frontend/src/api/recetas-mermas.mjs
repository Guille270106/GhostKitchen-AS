export function crearApiRecetasMermas(pedir) {
  return {
    async cargar() {
      const [platos, ingredientes, recetas] = await Promise.all([
        pedir("GET", "/cocina/platos"),
        pedir("GET", "/cocina/ingredientes"),
        pedir("GET", "/cocina/recetas"),
      ]);
      return { platos: platos.items, ingredientes: ingredientes.items, recetas: recetas.items };
    },
    guardarReceta(id, items) {
      return pedir("PUT", `/cocina/platos/${encodeURIComponent(id)}/receta`, { cuerpo: { items } });
    },
    registrarMerma(id, cantidad) {
      return pedir("POST", `/cocina/ingredientes/${encodeURIComponent(id)}/mermas`, { cuerpo: { cantidad } });
    },
  };
}

export function validarReceta(filas) {
  const ids = new Set();
  if (!filas.length) throw new Error("Agregá al menos un ingrediente.");
  return filas.map(({ ingredienteId, cantidad }) => {
    const unidades = Number(cantidad);
    if (!ingredienteId || !Number.isSafeInteger(unidades) || unidades < 1) {
      throw new Error("Elegí un ingrediente y una cantidad entera positiva en cada fila.");
    }
    if (ids.has(ingredienteId)) throw new Error("Cada ingrediente puede aparecer una sola vez.");
    ids.add(ingredienteId);
    return { ingredienteId, cantidad: unidades };
  });
}
