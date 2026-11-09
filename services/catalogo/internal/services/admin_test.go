package services

import (
	"context"
	"errors"
	"sync"
	"testing"

	"ghostkitchen/catalogo/internal/domain"
	"ghostkitchen/catalogo/internal/repositories/memoria"
)

func ptr[T any](v T) *T { return &v }

var adminNC = Admin{UsuarioID: "adm", Rol: "admin", Localidad: "Nueva Córdoba"}

func entornoAdmin(t *testing.T) (*AdminCatalogo, *memoria.Repositorio) {
	t.Helper()
	repo := memoria.Nuevo()
	nc := domain.Cocina{ID: "nueva-cordoba", Nombre: "Cocina Nueva Córdoba", Localidad: "Nueva Córdoba", Direccion: "Obispo Trejo 1050",
		Ubicacion: domain.Ubicacion{Lat: -31.42, Lng: -64.18}, TiempoEntregaMin: 25, TiempoEntregaMax: 35, CostoEnvio: 1200, Activa: true, Version: 1,
		Restaurantes: []domain.Restaurante{{ID: "r1", Nombre: "Mostaza", Activo: true}, {ID: "r2", Nombre: "Sushi Pop", Activo: true}}}
	cerro := nc
	cerro.ID, cerro.Nombre, cerro.Localidad, cerro.Restaurantes = "cerro", "Cocina Cerro", "Cerro de las Rosas", nil
	repo.Guardar(&nc, nil)
	repo.Guardar(&cerro, nil)
	return NuevoAdmin(repo, repo), repo
}

var datosNueva = domain.CambiosCocina{
	Nombre: ptr("Cocina Güemes"), Direccion: ptr("Belgrano 800"), Ubicacion: &domain.Ubicacion{Lat: -31.43, Lng: -64.19},
	TiempoEntregaMin: ptr(20), TiempoEntregaMax: ptr(30), CostoEnvio: ptr(1000),
}

func TestAdminVeSoloSuLocalidad(t *testing.T) {
	svc, _ := entornoAdmin(t)
	cocinas, err := svc.CocinasDelAdmin(context.Background(), adminNC)
	if err != nil || len(cocinas) != 1 || cocinas[0].ID != "nueva-cordoba" {
		t.Fatalf("%v %v", cocinas, err)
	}
}

func TestCrearCocinaEnSuLocalidad(t *testing.T) {
	svc, repo := entornoAdmin(t)
	c, err := svc.CrearCocina(context.Background(), adminNC, datosNueva)
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != "guemes" || c.Localidad != "Nueva Córdoba" || !c.Activa || c.Version != 1 || c.Restaurantes == nil {
		t.Fatalf("cocina inesperada: %+v", c)
	}
	if _, ok, _ := repo.ObtenerCocina(context.Background(), "guemes"); !ok {
		t.Fatal("no se guardó")
	}
	if _, err := svc.CrearCocina(context.Background(), adminNC, datosNueva); !errors.Is(err, domain.ErrYaExiste) {
		t.Fatalf("mismo nombre dos veces: %v", err)
	}
	sinUbicacion := datosNueva
	sinUbicacion.Nombre, sinUbicacion.Ubicacion = ptr("Cocina Otra"), nil
	if _, err := svc.CrearCocina(context.Background(), adminNC, sinUbicacion); !errors.Is(err, domain.ErrDatosInvalidos) {
		t.Fatalf("sin ubicación: %v", err)
	}
}

func TestSoloAdminYSoloSuLocalidad(t *testing.T) {
	svc, _ := entornoAdmin(t)
	cocina := Admin{UsuarioID: "k", Rol: "cocina"}
	if _, err := svc.CrearCocina(context.Background(), cocina, datosNueva); !errors.Is(err, domain.ErrSinPermiso) {
		t.Fatalf("una cuenta Cocina no crea cocinas: %v", err)
	}
	if _, err := svc.ModificarCocina(context.Background(), adminNC, "cerro", domain.CambiosCocina{Activa: ptr(false)}); !errors.Is(err, domain.ErrOtraLocalidad) {
		t.Fatalf("otra localidad: %v", err)
	}
	if _, err := svc.AgregarRestaurante(context.Background(), adminNC, "cerro", "Mostaza"); !errors.Is(err, domain.ErrOtraLocalidad) {
		t.Fatalf("restaurante en otra localidad: %v", err)
	}
	if _, err := svc.ModificarCocina(context.Background(), adminNC, "rio-cuarto", domain.CambiosCocina{}); !errors.Is(err, domain.ErrCocinaNoEncontrada) {
		t.Fatalf("inexistente: %v", err)
	}
}

func TestModificarYDesactivarSubeLaVersion(t *testing.T) {
	svc, repo := entornoAdmin(t)
	c, err := svc.ModificarCocina(context.Background(), adminNC, "nueva-cordoba", domain.CambiosCocina{CostoEnvio: ptr(1500), Activa: ptr(false)})
	if err != nil || c.CostoEnvio != 1500 || c.Activa || c.Version != 2 {
		t.Fatalf("%+v %v", c, err)
	}
	guardada, _, _ := repo.ObtenerCocina(context.Background(), "nueva-cordoba")
	if guardada.Version != 2 || guardada.Activa {
		t.Fatalf("no se guardó: %+v", guardada)
	}
}

func TestRestaurantes(t *testing.T) {
	svc, _ := entornoAdmin(t)
	r, err := svc.AgregarRestaurante(context.Background(), adminNC, "nueva-cordoba", "Café Martínez")
	if err != nil || r.ID == "" || !r.Activo || r.Nombre != "Café Martínez" {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err := svc.AgregarRestaurante(context.Background(), adminNC, "nueva-cordoba", "mostaza"); !errors.Is(err, domain.ErrRestauranteRepetido) {
		t.Fatalf("repetido: %v", err)
	}
	if _, err := svc.AgregarRestaurante(context.Background(), adminNC, "nueva-cordoba", "La Cabrera"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AgregarRestaurante(context.Background(), adminNC, "nueva-cordoba", "Quinto"); !errors.Is(err, domain.ErrCocinaLlena) {
		t.Fatalf("quinto: %v", err)
	}
	r, err = svc.ModificarRestaurante(context.Background(), adminNC, "nueva-cordoba", "r1", domain.CambiosRestaurante{Activo: ptr(false)})
	if err != nil || r.Activo {
		t.Fatalf("desactivar: %+v %v", r, err)
	}
}

// Si otra solicitud cambia la cocina entre la lectura y la escritura, la escritura no pisa
// nada: se vuelve a leer y las reglas se controlan sobre lo último guardado.
func TestConflictoSeReintentaSobreLaVersionNueva(t *testing.T) {
	svc, repo := entornoAdmin(t)
	// Mientras se agrega "Café Martínez", "otra solicitud" agrega "La Cabrera".
	repo.AntesDeReemplazar = func() {
		c, _, _ := repo.ObtenerCocina(context.Background(), "nueva-cordoba")
		_ = c.AgregarRestaurante(domain.Restaurante{ID: "r9", Nombre: "La Cabrera", Activo: true})
		c.Version++
		repo.Guardar(&c, nil)
	}
	if _, err := svc.AgregarRestaurante(context.Background(), adminNC, "nueva-cordoba", "Café Martínez"); err != nil {
		t.Fatal(err)
	}
	c, _, _ := repo.ObtenerCocina(context.Background(), "nueva-cordoba")
	if len(c.Restaurantes) != 4 || c.Version != 3 {
		t.Fatalf("tienen que estar los dos cambios: %d restaurantes, versión %d", len(c.Restaurantes), c.Version)
	}
}

// 10 admins agregan a la vez un restaurante a una cocina con 2: solo entran 2 (RN20).
func TestNoSePasaDeCuatroConSolicitudesALaVez(t *testing.T) {
	svc, repo := entornoAdmin(t)
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, llena, conflicto := 0, 0, 0
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := svc.AgregarRestaurante(context.Background(), adminNC, "nueva-cordoba", "Marca "+string(rune('A'+i)))
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errors.Is(err, domain.ErrCocinaLlena):
				llena++
			case errors.Is(err, domain.ErrConflicto):
				conflicto++
			default:
				t.Errorf("error inesperado: %v", err)
			}
		}(i)
	}
	wg.Wait()
	c, _, _ := repo.ObtenerCocina(context.Background(), "nueva-cordoba")
	if ok != 2 || len(c.Restaurantes) != 4 {
		t.Fatalf("entraron %d (quedaron %d restaurantes); llena %d, conflicto %d", ok, len(c.Restaurantes), llena, conflicto)
	}
}
