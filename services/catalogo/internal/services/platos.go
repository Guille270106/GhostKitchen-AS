package services

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"ghostkitchen/catalogo/internal/domain"
	"ghostkitchen/catalogo/internal/middleware"
	"ghostkitchen/catalogo/internal/repositories"
)

// Cocina es la cuenta Cocina que pide el cambio, según el token que verificó el gateway.
type Cocina struct {
	UsuarioID string
	Rol       string
	CocinaID  string
}

func (c Cocina) valida() error {
	if c.Rol != "cocina" || c.CocinaID == "" {
		return domain.ErrSinPermiso
	}
	return nil
}

// Recetas es lo que el servicio necesita de Inventario: saber si un plato tiene receta.
type Recetas interface {
	TieneReceta(ctx context.Context, usuarioID, cocinaID, platoID, correlationID string) (bool, error)
}

// PlatosCocina son las escrituras de la cuenta Cocina sobre sus platos (HU17).
type PlatosCocina struct {
	repo    repositories.Repositorio
	esc     repositories.EscritorPlatos
	recetas Recetas
	ahora   func() time.Time
}

// NuevoPlatosCocina crea el servicio.
func NuevoPlatosCocina(repo repositories.Repositorio, esc repositories.EscritorPlatos, recetas Recetas) *PlatosCocina {
	return &PlatosCocina{repo: repo, esc: esc, recetas: recetas, ahora: time.Now}
}

// MisPlatos devuelve todos los platos de su cocina, activos o no (para su panel).
func (s *PlatosCocina) MisPlatos(ctx context.Context, quien Cocina) ([]domain.Plato, error) {
	if err := quien.valida(); err != nil {
		return nil, err
	}
	return s.esc.ListarPlatosDeCocinaTodos(ctx, quien.CocinaID)
}

// CrearPlato da de alta un plato en un restaurante de su cocina (HU17).
//
// Un plato nuevo arranca **desactivado**: todavía no tiene receta, y un plato sin receta no
// se publica (RN18). La cocina le carga la receta en Inventario y después lo activa.
func (s *PlatosCocina) CrearPlato(ctx context.Context, quien Cocina, restauranteID string, datos domain.CambiosPlato) (domain.Plato, error) {
	if err := quien.valida(); err != nil {
		return domain.Plato{}, err
	}
	c, ok, err := s.repo.ObtenerCocina(ctx, quien.CocinaID)
	if err != nil {
		return domain.Plato{}, err
	}
	if !ok {
		return domain.Plato{}, domain.ErrCocinaNoEncontrada
	}
	restaurante, existe := buscarRestaurante(c.Restaurantes, restauranteID)
	switch {
	case !existe:
		return domain.Plato{}, domain.ErrRestauranteNoEncontrado
	case !restaurante.Activo:
		return domain.Plato{}, errors.Join(domain.ErrDatosInvalidos, errors.New("el restaurante está desactivado"))
	}
	inactivo := false
	datos.Activo = &inactivo
	p := domain.Plato{
		ID: "p-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10], CocinaID: c.ID, RestauranteID: restauranteID,
		Moneda: "ARS", Etiquetas: []string{}, Version: 1,
	}
	if err := p.Aplicar(datos); err != nil {
		return domain.Plato{}, err
	}
	p.ActualizadoEn = s.ahora().UTC()
	ev := domain.EventoPlato(domain.PlatoCreado, p, middleware.IDDesdeContexto(ctx), p.ActualizadoEn)
	if err := s.esc.CrearPlato(ctx, p, ev); err != nil {
		return domain.Plato{}, err
	}
	return p, nil
}

// ModificarPlato cambia un plato de su cocina, lo desactiva o lo vuelve a activar (HU17, RN19).
// Para activarlo, antes se le pregunta a Inventario si tiene receta (RN18).
func (s *PlatosCocina) ModificarPlato(ctx context.Context, quien Cocina, id string, cambios domain.CambiosPlato, correlationID string) (domain.Plato, error) {
	if err := quien.valida(); err != nil {
		return domain.Plato{}, err
	}
	for intento := 0; intento < reintentosConflicto; intento++ {
		p, ok, err := s.repo.ObtenerPlato(ctx, id)
		if err != nil {
			return domain.Plato{}, err
		}
		if !ok {
			return domain.Plato{}, domain.ErrPlatoNoEncontrado
		}
		if p.CocinaID != quien.CocinaID {
			return domain.Plato{}, domain.ErrOtraCocina
		}
		seActiva := cambios.Activo != nil && *cambios.Activo && !p.Activo
		anterior, estabaActivo := p.Version, p.Activo
		if err := p.Aplicar(cambios); err != nil {
			return domain.Plato{}, err
		}
		if seActiva && intento == 0 {
			if err := s.exigirReceta(ctx, quien, p.ID, correlationID); err != nil {
				return domain.Plato{}, err
			}
		}
		p.Version = anterior + 1
		p.ActualizadoEn = s.ahora().UTC()
		tipo := domain.TipoCambio(estabaActivo, p.Activo, domain.PlatoActualizado, domain.PlatoDesactivado)
		ev := domain.EventoPlato(tipo, p, correlationID, p.ActualizadoEn)
		err = s.esc.ReemplazarPlato(ctx, p, anterior, ev)
		if errors.Is(err, domain.ErrConflicto) {
			continue
		}
		if err != nil {
			return domain.Plato{}, err
		}
		return p, nil
	}
	return domain.Plato{}, domain.ErrConflicto
}

func (s *PlatosCocina) exigirReceta(ctx context.Context, quien Cocina, platoID, correlationID string) error {
	if s.recetas == nil {
		return domain.ErrInventarioNoDisponible
	}
	tiene, err := s.recetas.TieneReceta(ctx, quien.UsuarioID, quien.CocinaID, platoID, correlationID)
	if err != nil {
		return err
	}
	if !tiene {
		return domain.ErrSinReceta
	}
	return nil
}

func buscarRestaurante(rs []domain.Restaurante, id string) (domain.Restaurante, bool) {
	for _, r := range rs {
		if r.ID == id {
			return r, true
		}
	}
	return domain.Restaurante{}, false
}
