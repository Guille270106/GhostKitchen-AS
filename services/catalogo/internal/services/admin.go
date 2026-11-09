package services

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"ghostkitchen/catalogo/internal/domain"
	"ghostkitchen/catalogo/internal/repositories"
)

// Admin es quien pide el cambio: lo que el gateway verificó en su token (G9).
type Admin struct {
	UsuarioID string
	Rol       string
	Localidad string
}

// Admin usa estas escrituras. Se separa de Catalogo para que las lecturas no necesiten un
// repositorio que escriba.
type AdminCatalogo struct {
	repo  repositories.Repositorio
	esc   repositories.Escritor
	ahora func() time.Time
}

// NuevoAdmin crea el servicio de escrituras del admin.
func NuevoAdmin(repo repositories.Repositorio, esc repositories.Escritor) *AdminCatalogo {
	return &AdminCatalogo{repo: repo, esc: esc, ahora: time.Now}
}

// Reintentos cuando otra solicitud cambió la cocina al mismo tiempo. Con 3 alcanza: los
// cambios de un admin son de a uno, y la colisión es rara.
const reintentosConflicto = 3

func (a Admin) valido() error {
	if a.Rol != "admin" || a.Localidad == "" {
		return domain.ErrSinPermiso
	}
	return nil
}

// mismaLocalidad compara sin mayúsculas ni acentos.
func mismaLocalidad(a, b string) bool { return domain.Clave(a) == domain.Clave(b) }

// CocinasDelAdmin devuelve todas las cocinas de la localidad del admin, activas o no, con
// todos sus restaurantes (HU14). Las de otra localidad no se ven.
func (s *AdminCatalogo) CocinasDelAdmin(ctx context.Context, admin Admin) ([]domain.Cocina, error) {
	if err := admin.valido(); err != nil {
		return nil, err
	}
	todas, err := s.repo.ListarCocinas(ctx, false)
	if err != nil {
		return nil, err
	}
	suyas := []domain.Cocina{}
	for _, c := range todas {
		if mismaLocalidad(c.Localidad, admin.Localidad) {
			suyas = append(suyas, c)
		}
	}
	return suyas, nil
}

// CrearCocina da de alta una cocina en la localidad del admin (HU14). La localidad no viene
// del pedido: es la del admin, así nadie crea cocinas en otra localidad.
func (s *AdminCatalogo) CrearCocina(ctx context.Context, admin Admin, datos domain.CambiosCocina) (domain.Cocina, error) {
	if err := admin.valido(); err != nil {
		return domain.Cocina{}, err
	}
	activa := true
	datos.Activa = &activa // una cocina nueva arranca activa
	c := domain.Cocina{Localidad: admin.Localidad, Restaurantes: []domain.Restaurante{}, Version: 1}
	if err := c.Aplicar(datos); err != nil {
		return domain.Cocina{}, err
	}
	c.ID = domain.IDDesdeNombre(c.Nombre)
	if c.ID == "" {
		return domain.Cocina{}, errors.Join(domain.ErrDatosInvalidos, errors.New("el nombre tiene que tener letras o números"))
	}
	c.ActualizadaEn = s.ahora().UTC()
	if err := s.esc.CrearCocina(ctx, c); err != nil {
		return domain.Cocina{}, err
	}
	return c, nil
}

// ModificarCocina cambia datos de una cocina de su localidad, o la desactiva (HU14, RN19).
func (s *AdminCatalogo) ModificarCocina(ctx context.Context, admin Admin, id string, cambios domain.CambiosCocina) (domain.Cocina, error) {
	return s.cambiar(ctx, admin, id, func(c *domain.Cocina) error { return c.Aplicar(cambios) })
}

// AgregarRestaurante suma una marca a una cocina de su localidad (HU16, RN20).
func (s *AdminCatalogo) AgregarRestaurante(ctx context.Context, admin Admin, cocinaID, nombre string) (domain.Restaurante, error) {
	r := domain.Restaurante{ID: "r-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10], Nombre: nombre, Activo: true}
	c, err := s.cambiar(ctx, admin, cocinaID, func(c *domain.Cocina) error { return c.AgregarRestaurante(r) })
	if err != nil {
		return domain.Restaurante{}, err
	}
	return c.Restaurantes[len(c.Restaurantes)-1], nil
}

// ModificarRestaurante renombra, desactiva o reactiva una marca (HU16, RN19).
func (s *AdminCatalogo) ModificarRestaurante(ctx context.Context, admin Admin, cocinaID, id string, cambios domain.CambiosRestaurante) (domain.Restaurante, error) {
	var r domain.Restaurante
	_, err := s.cambiar(ctx, admin, cocinaID, func(c *domain.Cocina) error {
		var err error
		r, err = c.CambiarRestaurante(id, cambios)
		return err
	})
	return r, err
}

// cambiar es el ciclo de concurrencia optimista: leer la cocina, controlar que sea de la
// localidad del admin, aplicar el cambio y guardar solo si nadie la cambió en el medio. Si
// alguien la cambió, se vuelve a leer y se aplica de nuevo sobre la versión nueva: las reglas
// (4 restaurantes, sin repetir) se controlan siempre sobre lo último guardado.
func (s *AdminCatalogo) cambiar(ctx context.Context, admin Admin, id string, cambio func(*domain.Cocina) error) (domain.Cocina, error) {
	if err := admin.valido(); err != nil {
		return domain.Cocina{}, err
	}
	for intento := 0; intento < reintentosConflicto; intento++ {
		c, ok, err := s.repo.ObtenerCocina(ctx, id)
		if err != nil {
			return domain.Cocina{}, err
		}
		if !ok {
			return domain.Cocina{}, domain.ErrCocinaNoEncontrada
		}
		if !mismaLocalidad(c.Localidad, admin.Localidad) {
			return domain.Cocina{}, domain.ErrOtraLocalidad
		}
		anterior := c.Version
		if err := cambio(&c); err != nil {
			return domain.Cocina{}, err
		}
		c.Version = anterior + 1
		c.ActualizadaEn = s.ahora().UTC()
		err = s.esc.ReemplazarCocina(ctx, c, anterior)
		if errors.Is(err, domain.ErrConflicto) {
			continue
		}
		if err != nil {
			return domain.Cocina{}, err
		}
		return c, nil
	}
	return domain.Cocina{}, domain.ErrConflicto
}
