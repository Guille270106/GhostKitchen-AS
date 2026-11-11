package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ghostkitchen/inventario/internal/domain"
)

// Repositorio falso: anota con qué datos lo llamaron. Sirve para comprobar que los casos de uso
// validan ANTES de tocar el stock: con datos inválidos, el repositorio no se llama nunca.
type repoFalso struct {
	llamadas []string
	cantidad int64 // la que llega a MoverStock (negativa si es merma)
}

func (r *repoFalso) anotar(que string) { r.llamadas = append(r.llamadas, que) }

func (r *repoFalso) Consumir(_ context.Context, clave, reserva, pedido string) (domain.Consumo, error) {
	r.anotar("Consumir " + clave + " " + reserva + " " + pedido)
	return domain.Consumo{PedidoID: pedido, ReservaID: reserva, Estado: "CONSUMIDO"}, nil
}
func (r *repoFalso) Devolver(_ context.Context, clave, pedido string) (domain.Consumo, error) {
	r.anotar("Devolver " + clave + " " + pedido)
	return domain.Consumo{PedidoID: pedido, Estado: "DEVUELTO"}, nil
}
func (r *repoFalso) ObtenerConsumo(_ context.Context, pedido string) (domain.Consumo, error) {
	r.anotar("ObtenerConsumo " + pedido)
	return domain.Consumo{PedidoID: pedido}, nil
}
func (r *repoFalso) Crear(context.Context, domain.Ingrediente) error { r.anotar("Crear"); return nil }
func (r *repoFalso) Listar(_ context.Context, cocina string) ([]domain.Ingrediente, error) {
	r.anotar("Listar " + cocina)
	return nil, nil
}
func (r *repoFalso) Obtener(_ context.Context, cocina, id string) (domain.Ingrediente, error) {
	r.anotar("Obtener " + cocina + " " + id)
	return domain.Ingrediente{ID: id, CocinaID: cocina}, nil
}
func (r *repoFalso) MoverStock(_ context.Context, cocina, id, usuario string, cantidad int64) (domain.Ingrediente, error) {
	r.anotar("MoverStock " + cocina + " " + id + " " + usuario)
	r.cantidad = cantidad
	return domain.Ingrediente{ID: id, CocinaID: cocina}, nil
}
func (r *repoFalso) GuardarReceta(_ context.Context, receta domain.Receta) (domain.Receta, error) {
	r.anotar("GuardarReceta " + receta.PlatoID + " " + receta.CocinaID)
	return receta, nil
}
func (r *repoFalso) ObtenerReceta(_ context.Context, cocina, plato string) (domain.Receta, error) {
	r.anotar("ObtenerReceta " + cocina + " " + plato)
	return domain.Receta{PlatoID: plato}, nil
}
func (r *repoFalso) ListarRecetas(_ context.Context, cocina string) ([]domain.Receta, error) {
	r.anotar("ListarRecetas " + cocina)
	return nil, nil
}
func (r *repoFalso) CrearReserva(_ context.Context, clave string, datos domain.SolicitudReserva) (domain.Reserva, error) {
	r.anotar("CrearReserva " + clave + " " + datos.CarritoID)
	return domain.Reserva{CarritoID: datos.CarritoID}, nil
}
func (r *repoFalso) AjustarReserva(_ context.Context, clave, id string, _ []domain.PlatoSolicitado) (domain.Reserva, error) {
	r.anotar("AjustarReserva " + clave + " " + id)
	return domain.Reserva{ID: id}, nil
}
func (r *repoFalso) LiberarReserva(_ context.Context, clave, id string) (domain.Reserva, error) {
	r.anotar("LiberarReserva " + clave + " " + id)
	return domain.Reserva{ID: id}, nil
}
func (r *repoFalso) ObtenerReserva(_ context.Context, id string) (domain.Reserva, error) {
	r.anotar("ObtenerReserva " + id)
	return domain.Reserva{ID: id}, nil
}

var (
	ctx          = context.Background()
	cocina       = domain.Identidad{UsuarioID: "u1", Rol: "cocina", CocinaID: "cerro"}
	cliente      = domain.Identidad{UsuarioID: "u2", Rol: "cliente"}
	sinIdentidad = domain.Identidad{}
	claveLarga   = strings.Repeat("k", 129)
)

func noLlamo(t *testing.T, r *repoFalso) {
	t.Helper()
	if len(r.llamadas) > 0 {
		t.Fatalf("con datos inválidos no se tiene que tocar el repositorio, y se llamó: %v", r.llamadas)
	}
}

// Descontar y devolver stock (confirmar y cancelar un pedido): sin una clave de idempotencia
// válida o con identificadores raros, no se descuenta ni se devuelve nada.
func TestConsumosValidanAntesDeTocarElStock(t *testing.T) {
	casos := []struct {
		nombre string
		correr func(*Consumos) error
		quiere error
	}{
		{"consumir sin clave", func(s *Consumos) error { _, err := s.Consumir(ctx, "", "r1", "p1"); return err }, domain.ErrClaveInvalida},
		{"consumir con clave larga", func(s *Consumos) error { _, err := s.Consumir(ctx, claveLarga, "r1", "p1"); return err }, domain.ErrClaveInvalida},
		{"consumir con clave con espacios", func(s *Consumos) error { _, err := s.Consumir(ctx, "con espacio", "r1", "p1"); return err }, domain.ErrClaveInvalida},
		{"consumir sin reserva", func(s *Consumos) error { _, err := s.Consumir(ctx, "k1", "", "p1"); return err }, domain.ErrReservaInvalida},
		{"consumir con pedido inválido", func(s *Consumos) error { _, err := s.Consumir(ctx, "k1", "r1", "p/1"); return err }, domain.ErrReservaInvalida},
		{"devolver sin clave", func(s *Consumos) error { _, err := s.Devolver(ctx, "", "p1"); return err }, domain.ErrClaveInvalida},
		{"devolver con pedido inválido", func(s *Consumos) error { _, err := s.Devolver(ctx, "k1", "p 1"); return err }, domain.ErrReservaInvalida},
		{"obtener con pedido inválido", func(s *Consumos) error { _, err := s.Obtener(ctx, ""); return err }, domain.ErrReservaInvalida},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			repo := &repoFalso{}
			if err := c.correr(NuevosConsumos(repo)); !errors.Is(err, c.quiere) {
				t.Fatalf("error = %v, se esperaba %v", err, c.quiere)
			}
			noLlamo(t, repo)
		})
	}
}

func TestConsumosValidosLleganAlRepositorioConSusDatos(t *testing.T) {
	repo := &repoFalso{}
	s := NuevosConsumos(repo)
	if c, err := s.Consumir(ctx, "k1", "r1", "p1"); err != nil || c.Estado != "CONSUMIDO" {
		t.Fatalf("consumir: %v %+v", err, c)
	}
	if c, err := s.Devolver(ctx, "k2", "p1"); err != nil || c.Estado != "DEVUELTO" {
		t.Fatalf("devolver: %v %+v", err, c)
	}
	if _, err := s.Obtener(ctx, "p1"); err != nil {
		t.Fatal(err)
	}
	quiere := []string{"Consumir k1 r1 p1", "Devolver k2 p1", "ObtenerConsumo p1"}
	if strings.Join(repo.llamadas, "|") != strings.Join(quiere, "|") {
		t.Fatalf("llamadas = %v, se esperaba %v", repo.llamadas, quiere)
	}
}

// Reposición y merma: solo la cuenta Cocina, siempre sobre su cocina, con cantidades positivas.
// La merma llega al repositorio como cantidad negativa.
func TestMoverStock(t *testing.T) {
	t.Run("reposición suma", func(t *testing.T) {
		repo := &repoFalso{}
		if _, err := Nuevo(repo).MoverStock(ctx, cocina, "i1", 5, false); err != nil {
			t.Fatal(err)
		}
		if repo.cantidad != 5 || repo.llamadas[0] != "MoverStock cerro i1 u1" {
			t.Fatalf("cantidad %d, llamadas %v", repo.cantidad, repo.llamadas)
		}
	})
	t.Run("merma resta", func(t *testing.T) {
		repo := &repoFalso{}
		if _, err := Nuevo(repo).MoverStock(ctx, cocina, "i1", 3, true); err != nil {
			t.Fatal(err)
		}
		if repo.cantidad != -3 {
			t.Fatalf("la merma tiene que llegar negativa, llegó %d", repo.cantidad)
		}
	})
	invalidos := []struct {
		nombre   string
		quien    domain.Identidad
		id       string
		cantidad int64
		quiere   error
	}{
		{"sin identidad", sinIdentidad, "i1", 5, domain.ErrSinIdentidad},
		{"un cliente", cliente, "i1", 5, domain.ErrSinPermiso},
		{"cantidad cero", cocina, "i1", 0, domain.ErrDatosInvalidos},
		{"cantidad negativa", cocina, "i1", -2, domain.ErrDatosInvalidos},
		{"ingrediente inválido", cocina, "i/1", 5, domain.ErrDatosInvalidos},
	}
	for _, c := range invalidos {
		t.Run(c.nombre, func(t *testing.T) {
			repo := &repoFalso{}
			if _, err := Nuevo(repo).MoverStock(ctx, c.quien, c.id, c.cantidad, false); !errors.Is(err, c.quiere) {
				t.Fatalf("error = %v, se esperaba %v", err, c.quiere)
			}
			noLlamo(t, repo)
		})
	}
}

// Ingredientes y recetas: cada cocina trabaja solo con lo suyo (la cocina sale de la identidad,
// nunca de lo que mande el cliente), y nadie más que la cuenta Cocina puede hacerlo.
func TestIngredientesYRecetasSonDeLaCocinaDeLaCuenta(t *testing.T) {
	repo := &repoFalso{}
	ing := Nuevo(repo)
	rec := NuevasRecetas(repo)
	if i, err := ing.Crear(ctx, cocina, "Pan", "unidades", 10); err != nil || i.CocinaID != "cerro" || i.ID == "" {
		t.Fatalf("crear: %v %+v", err, i)
	}
	if _, err := ing.Listar(ctx, cocina); err != nil {
		t.Fatal(err)
	}
	if _, err := ing.Obtener(ctx, cocina, "i1"); err != nil {
		t.Fatal(err)
	}
	if r, err := rec.Guardar(ctx, cocina, "p1", []domain.RecetaItem{{IngredienteID: "i1", Cantidad: 2}}); err != nil || r.CocinaID != "cerro" {
		t.Fatalf("guardar receta: %v %+v", err, r)
	}
	if _, err := rec.Obtener(ctx, cocina, "p1"); err != nil {
		t.Fatal(err)
	}
	if _, err := rec.Listar(ctx, cocina); err != nil {
		t.Fatal(err)
	}
	quiere := "Crear|Listar cerro|Obtener cerro i1|GuardarReceta p1 cerro|ObtenerReceta cerro p1|ListarRecetas cerro"
	if got := strings.Join(repo.llamadas, "|"); got != quiere {
		t.Fatalf("llamadas = %s, se esperaba %s", got, quiere)
	}

	for _, quien := range []domain.Identidad{sinIdentidad, cliente} {
		repo := &repoFalso{}
		ing, rec := Nuevo(repo), NuevasRecetas(repo)
		errs := []error{}
		_, err := ing.Crear(ctx, quien, "Pan", "unidades", 10)
		errs = append(errs, err)
		_, err = ing.Listar(ctx, quien)
		errs = append(errs, err)
		_, err = ing.Obtener(ctx, quien, "i1")
		errs = append(errs, err)
		_, err = rec.Guardar(ctx, quien, "p1", []domain.RecetaItem{{IngredienteID: "i1", Cantidad: 2}})
		errs = append(errs, err)
		_, err = rec.Obtener(ctx, quien, "p1")
		errs = append(errs, err)
		_, err = rec.Listar(ctx, quien)
		errs = append(errs, err)
		for i, err := range errs {
			if err == nil {
				t.Fatalf("operación %d con %+v: tenía que fallar", i, quien)
			}
		}
		noLlamo(t, repo)
	}
}

func TestDatosInvalidosDeIngredientesYRecetasNoLleganAlRepositorio(t *testing.T) {
	repo := &repoFalso{}
	if _, err := Nuevo(repo).Crear(ctx, cocina, "  ", "unidades", 10); !errors.Is(err, domain.ErrDatosInvalidos) {
		t.Fatalf("ingrediente sin nombre: %v", err)
	}
	if _, err := Nuevo(repo).Crear(ctx, cocina, "Pan", "unidades", -1); !errors.Is(err, domain.ErrDatosInvalidos) {
		t.Fatalf("stock inicial negativo: %v", err)
	}
	if _, err := NuevasRecetas(repo).Guardar(ctx, cocina, "p1", nil); !errors.Is(err, domain.ErrRecetaInvalida) {
		t.Fatalf("receta vacía: %v", err)
	}
	noLlamo(t, repo)
}

// Reservas: la clave de idempotencia y los identificadores se validan antes de reservar,
// ajustar o liberar; los platos llegan ordenados y sin repetir.
func TestReservasValidanAntesDeTocarElStock(t *testing.T) {
	item := []domain.PlatoSolicitado{{PlatoID: "p1", Cantidad: 1}}
	casos := []struct {
		nombre string
		correr func(*Reservas) error
		quiere error
	}{
		{"crear sin clave", func(s *Reservas) error {
			_, err := s.Crear(ctx, "", domain.SolicitudReserva{CarritoID: "c1", CocinaID: "cerro", Items: item})
			return err
		}, domain.ErrClaveInvalida},
		{"crear sin carrito", func(s *Reservas) error {
			_, err := s.Crear(ctx, "k1", domain.SolicitudReserva{CocinaID: "cerro", Items: item})
			return err
		}, domain.ErrReservaInvalida},
		{"crear sin platos", func(s *Reservas) error {
			_, err := s.Crear(ctx, "k1", domain.SolicitudReserva{CarritoID: "c1", CocinaID: "cerro"})
			return err
		}, domain.ErrReservaInvalida},
		{"crear con plato repetido", func(s *Reservas) error {
			_, err := s.Crear(ctx, "k1", domain.SolicitudReserva{CarritoID: "c1", CocinaID: "cerro", Items: append(item, item...)})
			return err
		}, domain.ErrReservaInvalida},
		{"ajustar sin clave", func(s *Reservas) error { _, err := s.Ajustar(ctx, "", "r1", item); return err }, domain.ErrClaveInvalida},
		{"ajustar reserva inválida", func(s *Reservas) error { _, err := s.Ajustar(ctx, "k1", "", item); return err }, domain.ErrReservaInvalida},
		{"ajustar con cantidad cero", func(s *Reservas) error {
			_, err := s.Ajustar(ctx, "k1", "r1", []domain.PlatoSolicitado{{PlatoID: "p1", Cantidad: 0}})
			return err
		}, domain.ErrReservaInvalida},
		{"liberar sin clave", func(s *Reservas) error { _, err := s.Liberar(ctx, "", "r1"); return err }, domain.ErrClaveInvalida},
		{"liberar reserva inválida", func(s *Reservas) error { _, err := s.Liberar(ctx, "k1", "r 1"); return err }, domain.ErrReservaInvalida},
		{"obtener reserva inválida", func(s *Reservas) error { _, err := s.Obtener(ctx, ""); return err }, domain.ErrReservaInvalida},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			repo := &repoFalso{}
			if err := c.correr(NuevasReservas(repo)); !errors.Is(err, c.quiere) {
				t.Fatalf("error = %v, se esperaba %v", err, c.quiere)
			}
			noLlamo(t, repo)
		})
	}
}

func TestReservasValidasLleganAlRepositorio(t *testing.T) {
	repo := &repoFalso{}
	s := NuevasReservas(repo)
	item := []domain.PlatoSolicitado{{PlatoID: "p1", Cantidad: 1}}
	if _, err := s.Crear(ctx, "k1", domain.SolicitudReserva{CarritoID: "c1", CocinaID: "cerro", Items: item}); err != nil {
		t.Fatal(err)
	}
	// Ajustar a ningún plato es válido: es vaciar el carrito.
	if _, err := s.Ajustar(ctx, "k2", "r1", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Liberar(ctx, "k3", "r1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Obtener(ctx, "r1"); err != nil {
		t.Fatal(err)
	}
	quiere := "CrearReserva k1 c1|AjustarReserva k2 r1|LiberarReserva k3 r1|ObtenerReserva r1"
	if got := strings.Join(repo.llamadas, "|"); got != quiere {
		t.Fatalf("llamadas = %s, se esperaba %s", got, quiere)
	}
}
