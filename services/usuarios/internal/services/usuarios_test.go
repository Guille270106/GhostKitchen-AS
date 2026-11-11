package services

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"ghostkitchen/usuarios/internal/domain"
)

// memoria es un repositorio en memoria con el mismo contrato que el de MySQL: el correo no
// se repite.
type memoria struct {
	mu    sync.Mutex
	datos map[string]domain.Usuario
}

func nuevaMemoria() *memoria { return &memoria{datos: map[string]domain.Usuario{}} }

func (m *memoria) Crear(_ context.Context, u domain.Usuario) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, otro := range m.datos {
		if otro.Email == u.Email {
			return domain.ErrEmailRepetido
		}
	}
	m.datos[u.ID] = u
	return nil
}
func (m *memoria) PorEmail(_ context.Context, email string) (domain.Usuario, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.datos {
		if u.Email == email {
			return u, nil
		}
	}
	return domain.Usuario{}, domain.ErrNoEncontrado
}
func (m *memoria) PorID(_ context.Context, id string) (domain.Usuario, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if u, ok := m.datos[id]; ok {
		return u, nil
	}
	return domain.Usuario{}, domain.ErrNoEncontrado
}
func (m *memoria) Listo(context.Context) error { return nil }

func nuevo() (*Usuarios, *memoria) {
	m := nuevaMemoria()
	tk, _ := NuevosTokens(secretoDePrueba, time.Hour)
	return Nuevo(m, tk).ConCostoBcrypt(bcrypt.MinCost), m
}

const secretoDePrueba = "un-secreto-de-prueba-de-32-bytes!!"

var valido = DatosRegistro{Nombre: " Ana Pérez ", Email: " Ana@Mail.com ", Password: "clave-segura", Direccion: " Obispo Trejo 323 "}

func TestRegistrarCreaUnClienteConHash(t *testing.T) {
	svc, repo := nuevo()
	u, err := svc.Registrar(context.Background(), valido)
	if err != nil {
		t.Fatal(err)
	}
	if u.Rol != domain.RolCliente || u.Email != "ana@mail.com" || u.Nombre != "Ana Pérez" || u.Direccion != "Obispo Trejo 323" || !u.Activo || u.CreadoEn.IsZero() {
		t.Fatalf("usuario inesperado: %+v", u)
	}
	guardado, _ := repo.PorID(context.Background(), u.ID)
	if guardado.PasswordHash == "" || strings.Contains(guardado.PasswordHash, "clave-segura") {
		t.Fatal("la contraseña no puede guardarse tal cual")
	}
	if bcrypt.CompareHashAndPassword([]byte(guardado.PasswordHash), []byte("clave-segura")) != nil {
		t.Fatal("el hash guardado tiene que ser el bcrypt de la contraseña")
	}
	if guardado.Localidad != "" || guardado.CocinaAsignadaID != "" {
		t.Fatal("un cliente no tiene localidad ni cocina asignada")
	}
}

func TestRegistrarCorreoRepetidoAunqueCambienMayusculas(t *testing.T) {
	svc, _ := nuevo()
	if _, err := svc.Registrar(context.Background(), valido); err != nil {
		t.Fatal(err)
	}
	otro := valido
	otro.Email = "ANA@mail.COM"
	if _, err := svc.Registrar(context.Background(), otro); !errors.Is(err, domain.ErrEmailRepetido) {
		t.Fatalf("esperaba ErrEmailRepetido: %v", err)
	}
}

func TestRegistrarFaltaUnDato(t *testing.T) {
	svc, repo := nuevo()
	casos := map[string]func(*DatosRegistro){
		"sin nombre":           func(d *DatosRegistro) { d.Nombre = "  " },
		"sin dirección":        func(d *DatosRegistro) { d.Direccion = "" },
		"correo inválido":      func(d *DatosRegistro) { d.Email = "ana" },
		"contraseña corta":     func(d *DatosRegistro) { d.Password = "1234" },
		"contraseña muy larga": func(d *DatosRegistro) { d.Password = strings.Repeat("a", 73) },
	}
	for nombre, cambiar := range casos {
		d := valido
		cambiar(&d)
		if _, err := svc.Registrar(context.Background(), d); !errors.Is(err, domain.ErrDatosInvalidos) {
			t.Errorf("%s: esperaba ErrDatosInvalidos, obtuve %v", nombre, err)
		}
	}
	if len(repo.datos) != 0 {
		t.Fatal("con un dato mal no se guarda nada")
	}
}

func TestLogin(t *testing.T) {
	svc, repo := nuevo()
	u, err := svc.Registrar(context.Background(), valido)
	if err != nil {
		t.Fatal(err)
	}
	sesion, err := svc.Login(context.Background(), "ANA@mail.com ", "clave-segura")
	if err != nil {
		t.Fatal(err)
	}
	if sesion.Token == "" || sesion.Tipo != "Bearer" || sesion.Usuario.ID != u.ID || sesion.VenceEn.Before(time.Now()) {
		t.Fatalf("sesión inesperada: %+v", sesion)
	}
	id, err := svc.Verificar(sesion.Token)
	if err != nil || id.UsuarioID != u.ID || id.Rol != domain.RolCliente {
		t.Fatalf("el token del login tiene que verificar: %+v %v", id, err)
	}

	// Mismo error si el correo no existe o si la contraseña está mal.
	if _, err := svc.Login(context.Background(), "ana@mail.com", "otra-clave"); !errors.Is(err, ErrCredencialesInvalidas) {
		t.Fatalf("contraseña mal: %v", err)
	}
	if _, err := svc.Login(context.Background(), "nadie@mail.com", "clave-segura"); !errors.Is(err, ErrCredencialesInvalidas) {
		t.Fatalf("correo inexistente: %v", err)
	}

	// Una cuenta desactivada no entra.
	inactivo := repo.datos[u.ID]
	inactivo.Activo = false
	repo.datos[u.ID] = inactivo
	if _, err := svc.Login(context.Background(), "ana@mail.com", "clave-segura"); !errors.Is(err, ErrCredencialesInvalidas) {
		t.Fatalf("cuenta inactiva: %v", err)
	}
}
