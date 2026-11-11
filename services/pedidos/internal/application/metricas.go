package application

import (
	"context"
	"errors"
	"time"

	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/ports"
)

// MetricasService arma las métricas del negocio para el admin de cocina.
type MetricasService struct {
	Cocinas ports.Cocinas
	Repo    ports.MetricasRepo
	Ahora   func() time.Time // en los tests, una hora fija
}

// Obtener devuelve cuántos pedidos tuvo cada cocina de la localidad del admin en los últimos
// 7 días y el detalle de una (cocinaID, o la primera si viene vacío). Solo para admins, y solo
// de su localidad: la lista de cocinas sale de Catálogo, nunca de lo que mande el cliente.
func (s MetricasService) Obtener(ctx context.Context, actor domain.Actor, cocinaID string) (domain.Metricas, error) {
	if err := actor.Validar(); err != nil {
		return domain.Metricas{}, err
	}
	if actor.Rol != domain.RolAdmin {
		return domain.Metricas{}, errors.Join(domain.ErrSinPermiso, errors.New("solo el admin de cocina ve las métricas"))
	}
	if actor.Localidad == "" {
		return domain.Metricas{}, errors.Join(domain.ErrSinPermiso, errors.New("el admin no tiene localidad"))
	}
	ahora := time.Now
	if s.Ahora != nil {
		ahora = s.Ahora
	}
	desde := domain.InicioMetricas(ahora())
	res := domain.Metricas{Localidad: actor.Localidad, Desde: desde, Cocinas: []domain.ResumenCocina{}}

	cocinas, err := s.Cocinas.CocinasDeLocalidad(ctx, actor.Localidad)
	if err != nil {
		return domain.Metricas{}, err
	}
	if len(cocinas) == 0 {
		return res, nil
	}
	ids := make([]string, 0, len(cocinas))
	elegida := cocinas[0]
	encontrada := cocinaID == ""
	for _, c := range cocinas {
		ids = append(ids, c.ID)
		if c.ID == cocinaID {
			elegida, encontrada = c, true
		}
	}
	if !encontrada {
		// Una cocina de otra localidad (o que no existe): no se dice cuál de las dos.
		return domain.Metricas{}, errors.Join(domain.ErrSinPermiso, errors.New("esa cocina no es de tu localidad"))
	}

	conteo, err := s.Repo.PedidosPorCocina(ctx, ids, desde)
	if err != nil {
		return domain.Metricas{}, err
	}
	for _, c := range cocinas {
		res.Cocinas = append(res.Cocinas, domain.ResumenCocina{CocinaID: c.ID, Nombre: c.Nombre, Pedidos: conteo[c.ID]})
	}

	detalle, porDia, err := s.Repo.MetricasDeCocina(ctx, elegida.ID, desde)
	if err != nil {
		return domain.Metricas{}, err
	}
	detalle.CocinaID, detalle.Nombre = elegida.ID, elegida.Nombre
	detalle.Completar(desde, porDia)
	res.Detalle = &detalle
	return res, nil
}
