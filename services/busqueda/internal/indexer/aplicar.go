package indexer

import (
	"context"
	"fmt"

	"ghostkitchen/busqueda/internal/clients"
	"ghostkitchen/busqueda/internal/domain"
	"ghostkitchen/busqueda/internal/repositories"
)

// Aplicador guarda en el índice un cambio de Catálogo o de Inventario sin pisar datos más
// nuevos. Lo usan el lector de mensajes y la reconstrucción, que pueden correr a la vez:
// cada cambio se hace sobre lo último guardado (repositories.Actualizador).
//
// Reglas (ADR-005, README de Búsqueda):
//   - Cada fuente tiene su versión y cambia solo sus campos: Catálogo los datos comerciales
//     (versión de la cocina o del plato), Inventario la disponibilidad (su contador por plato).
//   - Un cambio con una versión menor que la guardada es viejo y se ignora. Con la misma
//     versión se vuelve a escribir lo mismo: un mensaje repetido no cambia nada.
//   - Un plato se muestra solo si él, su restaurante y su cocina están activos.
type Aplicador struct {
	idx repositories.Actualizador
}

// NuevoAplicador crea el aplicador.
func NuevoAplicador(idx repositories.Actualizador) *Aplicador { return &Aplicador{idx: idx} }

// Cocina guarda una cocina de Catálogo. Si cambió, recalcula sus platos: el nombre de la
// cocina y de cada restaurante va copiado en cada plato, y si se desactivó la cocina o un
// restaurante, sus platos dejan de mostrarse.
func (a *Aplicador) Cocina(ctx context.Context, c clients.CocinaCatalogo) error {
	var sede domain.Sede
	cambio := false
	err := a.idx.CambiarSede(ctx, c.ID, func(actual domain.Sede, existe bool) (domain.Sede, bool, error) {
		if existe && c.Version < actual.VersionCatalogo {
			return actual, false, nil // mensaje viejo
		}
		sede, cambio = sedeDeCatalogo(c), true
		return sede, true, nil
	})
	if err != nil || !cambio {
		return err
	}
	ids, err := a.idx.IDsPlatosDeSede(ctx, c.ID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		err := a.idx.CambiarPlato(ctx, id, func(p domain.Plato, existe bool) (domain.Plato, bool, error) {
			if !existe || p.Sede.ID != sede.ID {
				return p, false, nil // se borró o se movió en el medio
			}
			return conSede(p, sede), true, nil
		})
		if err != nil {
			return fmt.Errorf("plato %s: %w", id, err)
		}
	}
	return nil
}

// Plato guarda los datos comerciales de un plato de Catálogo. La disponibilidad no se toca:
// es de Inventario. Si el plato no estaba, entra como no disponible (sin receta) hasta que
// llegue la disponibilidad.
//
// Si su cocina todavía no está en el índice, el plato se guarda igual, inactivo, con el id de
// la cocina: cuando llegue la cocina, Cocina recalcula sus platos y lo completa. No se
// reintenta el mensaje porque, devuelto a la cola, podría quedar delante del de la cocina y
// trabarla.
func (a *Aplicador) Plato(ctx context.Context, p clients.PlatoCatalogo) error {
	return a.PlatoConDisponibilidad(ctx, p, nil)
}

// PlatoConDisponibilidad es Plato, pero si el plato no estaba en el índice usa inicial para
// decir si se puede pedir. La reconstrucción lo usa cuando no hay Inventario configurado
// (tabla provisional de los datos de ejemplo).
func (a *Aplicador) PlatoConDisponibilidad(ctx context.Context, p clients.PlatoCatalogo, inicial func(string) bool) error {
	sede, existe, err := a.sede(ctx, p.CocinaID)
	if err != nil {
		return err
	}
	if !existe {
		sede = domain.Sede{ID: p.CocinaID} // inactiva y sin restaurantes: el plato no se muestra
	}
	return a.idx.CambiarPlato(ctx, p.ID, func(actual domain.Plato, existe bool) (domain.Plato, bool, error) {
		if existe && p.Version < actual.VersionCatalogo {
			return actual, false, nil
		}
		nuevo := actual // conserva Disponible, TieneReceta y VersionInventario
		if !existe && inicial != nil {
			nuevo.Disponible = inicial(p.ID)
			nuevo.TieneReceta = nuevo.Disponible
		}
		etq := p.Etiquetas
		if etq == nil {
			etq = []string{}
		}
		moneda := p.Moneda
		if moneda == "" { // el contrato siempre publica una moneda; hoy todo es en pesos
			moneda = "ARS"
		}
		nuevo.ID, nuevo.Nombre, nuevo.Descripcion, nuevo.Categoria = p.ID, p.Nombre, p.Descripcion, p.Categoria
		nuevo.Etiquetas, nuevo.Precio, nuevo.Moneda, nuevo.FotoURL = etq, p.Precio, moneda, p.FotoURL
		nuevo.ActualizadoEn, nuevo.PlatoActivo, nuevo.VersionCatalogo = p.ActualizadoEn, p.Activo, p.Version
		nuevo.Restaurante = domain.Referencia{ID: p.RestauranteID}
		return conSede(nuevo, sede), true, nil
	})
}

// Disponibilidad guarda si un plato se puede pedir, según Inventario. Los datos comerciales
// no se tocan. Si el plato todavía no estaba (Inventario avisó antes que Catálogo), se guarda
// un plato inactivo con solo la disponibilidad, que se completa cuando llega el de Catálogo.
//
// Secuencia 0 es la lista de Inventario sin contador (reconstrucción): no se puede saber si
// es más nueva que lo guardado, así que solo se usa para platos que nunca recibieron un
// mensaje de Inventario.
func (a *Aplicador) Disponibilidad(ctx context.Context, d clients.DisponibilidadInventario) error {
	return a.idx.CambiarPlato(ctx, d.PlatoID, func(actual domain.Plato, existe bool) (domain.Plato, bool, error) {
		switch {
		case existe && d.Secuencia == 0 && actual.VersionInventario > 0:
			return actual, false, nil // ya tiene un dato con contador: el de la lista puede ser más viejo
		case existe && d.Secuencia < actual.VersionInventario:
			return actual, false, nil // mensaje viejo
		}
		nuevo := actual
		if !existe {
			nuevo = domain.Plato{
				ID: d.PlatoID, Etiquetas: []string{}, Moneda: "ARS",
				Sede: domain.Referencia{ID: d.CocinaID},
			}
		}
		nuevo.Disponible = d.Disponible && d.TieneReceta // sin receta, nunca disponible
		nuevo.TieneReceta, nuevo.VersionInventario = d.TieneReceta, d.Secuencia
		return nuevo, true, nil
	})
}

// sede lee una sede del índice, activa o no.
func (a *Aplicador) sede(ctx context.Context, id string) (domain.Sede, bool, error) {
	var s domain.Sede
	var existe bool
	// CambiarSede sin guardar es una lectura de lo último escrito (real-time get).
	err := a.idx.CambiarSede(ctx, id, func(actual domain.Sede, ok bool) (domain.Sede, bool, error) {
		s, existe = actual, ok
		return actual, false, nil
	})
	return s, existe, err
}

// sedeDeCatalogo arma la sede: los restaurantes activos se publican; los inactivos se guardan
// aparte, para saber de quién es un plato que llega después.
func sedeDeCatalogo(c clients.CocinaCatalogo) domain.Sede {
	s := domain.Sede{
		ID: c.ID, Nombre: c.Nombre, Localidad: c.Localidad, Direccion: c.Direccion,
		Ubicacion: c.Ubicacion, TiempoEntregaMin: c.TiempoEntregaMin, TiempoEntregaMax: c.TiempoEntregaMax,
		CostoEnvio: c.CostoEnvio, Restaurantes: []domain.Referencia{}, Activa: c.Activa,
		VersionCatalogo: c.Version,
	}
	for _, r := range c.Restaurantes {
		ref := domain.Referencia{ID: r.ID, Nombre: r.Nombre}
		if r.Activo {
			s.Restaurantes = append(s.Restaurantes, ref)
		} else {
			s.RestaurantesInactivos = append(s.RestaurantesInactivos, ref)
		}
	}
	return s
}

// conSede copia en el plato el nombre de su cocina y de su restaurante, y decide si se
// muestra: el plato, su restaurante y su cocina tienen que estar activos. Un
// restaurante que la cocina no tiene cuenta como inactivo.
func conSede(p domain.Plato, s domain.Sede) domain.Plato {
	p.Sede = s.Ref()
	restActivo := false
	for _, r := range s.Restaurantes {
		if r.ID == p.Restaurante.ID {
			p.Restaurante.Nombre, restActivo = r.Nombre, true
		}
	}
	for _, r := range s.RestaurantesInactivos {
		if r.ID == p.Restaurante.ID {
			p.Restaurante.Nombre = r.Nombre
		}
	}
	p.Activo = p.PlatoActivo && restActivo && s.Activa
	return p
}
