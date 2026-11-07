package services

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"ghostkitchen/usuarios/internal/clients"
	"ghostkitchen/usuarios/internal/domain"
)

// catalogoFalso responde como Catálogo con las cocinas que tenga cargadas.
type catalogoFalso struct {
	cocinas map[string]clients.Cocina
	caido   bool
	pedidos int
}

func (c *catalogoFalso) Cocina(_ context.Context, id, _ string) (clients.Cocina, error) {
	c.pedidos++
	if c.caido {
		return clients.Cocina{}, clients.ErrCatalogoNoDisponible
	}
	if k, ok := c.cocinas[id]; ok {
		return k, nil
	}
	return clients.Cocina{}, clients.ErrCocinaNoExiste
}

var adminNC = Identidad{UsuarioID: "adm", Rol: domain.RolAdmin, Localidad: "Nueva Córdoba"}

func conCatalogo() (*Usuarios, *memoria, *catalogoFalso) {
	svc, repo := nuevo()
	cat := &catalogoFalso{cocinas: map[string]clients.Cocina{
		"nueva-cordoba": {ID: "nueva-cordoba", Localidad: "Nueva Córdoba", Activa: true},
		"cerro":         {ID: "cerro", Localidad: "Cerro de las Rosas", Activa: true},
	}}
	return svc.ConCatalogo(cat), repo, cat
}

var cuentaNC = DatosCuentaCocina{Nombre: "Cocina NC", Email: "cocina.nc@gk.dev", Password: "clave-cocina-1", CocinaID: "nueva-cordoba"}

func TestAdminCreaCuentaCocinaDeSuLocalidad(t *testing.T) {
	svc, repo, _ := conCatalogo()
	u, err := svc.CrearCuentaCocina(context.Background(), adminNC, cuentaNC, "cid")
	if err != nil {
		t.Fatal(err)
	}
	if u.Rol != domain.RolCocina || u.CocinaAsignadaID != "nueva-cordoba" || u.Localidad != "" || u.Direccion != "" {
		t.Fatalf("cuenta inesperada: %+v", u)
	}
	if bcrypt.CompareHashAndPassword([]byte(repo.datos[u.ID].PasswordHash), []byte("clave-cocina-1")) != nil {
		t.Fatal("la contraseña se guarda como hash bcrypt")
	}
	// La cuenta nueva puede entrar y su token lleva la cocina.
	sesion, err := svc.Login(context.Background(), "cocina.nc@gk.dev", "clave-cocina-1")
	if err != nil {
		t.Fatal(err)
	}
	id, _ := svc.Verificar(sesion.Token)
	if id.Rol != domain.RolCocina || id.CocinaID != "nueva-cordoba" {
		t.Fatalf("token inesperado: %+v", id)
	}
}

func TestCuentaCocinaRechazos(t *testing.T) {
	casos := []struct {
		nombre   string
		quien    Identidad
		cambiar  func(*DatosCuentaCocina)
		caido    bool
		esperado error
		consulta bool // si tenía que llegar a preguntarle a Catálogo
	}{
		{"un cliente", Identidad{UsuarioID: "c", Rol: domain.RolCliente}, nil, false, ErrSinPermiso, false},
		{"una cuenta Cocina", Identidad{UsuarioID: "k", Rol: domain.RolCocina, CocinaID: "cerro"}, nil, false, ErrSinPermiso, false},
		{"cocina de otra localidad", adminNC, func(d *DatosCuentaCocina) { d.CocinaID = "cerro" }, false, ErrOtraLocalidad, true},
		{"cocina inexistente o inactiva", adminNC, func(d *DatosCuentaCocina) { d.CocinaID = "rio-cuarto" }, false, ErrCocinaInvalida, true},
		{"Catálogo no responde", adminNC, nil, true, ErrCatalogoNoDisponible, true},
		{"sin cocina", adminNC, func(d *DatosCuentaCocina) { d.CocinaID = " " }, false, domain.ErrDatosInvalidos, false},
		{"contraseña corta", adminNC, func(d *DatosCuentaCocina) { d.Password = "123" }, false, domain.ErrDatosInvalidos, false},
		{"correo inválido", adminNC, func(d *DatosCuentaCocina) { d.Email = "x" }, false, domain.ErrDatosInvalidos, false},
	}
	for _, c := range casos {
		svc, repo, cat := conCatalogo()
		cat.caido = c.caido
		d := cuentaNC
		if c.cambiar != nil {
			c.cambiar(&d)
		}
		_, err := svc.CrearCuentaCocina(context.Background(), c.quien, d, "")
		if !errors.Is(err, c.esperado) {
			t.Errorf("%s: esperaba %v, obtuve %v", c.nombre, c.esperado, err)
		}
		if errors.Is(err, ErrCatalogoNoDisponible) && errors.Is(err, ErrCocinaInvalida) {
			t.Errorf("%s: sin respuesta de Catálogo no es \"no existe\"", c.nombre)
		}
		if (cat.pedidos > 0) != c.consulta {
			t.Errorf("%s: consultas a Catálogo = %d", c.nombre, cat.pedidos)
		}
		if len(repo.datos) != 0 {
			t.Errorf("%s: no tenía que guardar nada", c.nombre)
		}
	}
}

func TestCuentaCocinaCorreoRepetido(t *testing.T) {
	svc, _, _ := conCatalogo()
	if _, err := svc.CrearCuentaCocina(context.Background(), adminNC, cuentaNC, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CrearCuentaCocina(context.Background(), adminNC, cuentaNC, ""); !errors.Is(err, domain.ErrEmailRepetido) {
		t.Fatalf("esperaba ErrEmailRepetido: %v", err)
	}
}

func TestSembrarAdminsSePuedeRepetir(t *testing.T) {
	svc, repo := nuevo()
	admins := []AdminInicial{
		{Nombre: "Admin NC", Email: "admin.nc@gk.dev", Localidad: "Nueva Córdoba"},
		{Nombre: "Admin Cerro", Email: "admin.cerro@gk.dev", Localidad: "Cerro de las Rosas"},
	}
	if n, err := svc.SembrarAdmins(context.Background(), "clave-admin-1", admins); err != nil || n != 2 {
		t.Fatalf("primera carga: %d %v", n, err)
	}
	if n, err := svc.SembrarAdmins(context.Background(), "otra-clave-9", admins); err != nil || n != 0 {
		t.Fatalf("la segunda carga no crea nada: %d %v", n, err)
	}
	if len(repo.datos) != 2 {
		t.Fatalf("esperaba 2 admins, hay %d", len(repo.datos))
	}
	// La segunda carga no le cambió la contraseña.
	if _, err := svc.Login(context.Background(), "admin.nc@gk.dev", "clave-admin-1"); err != nil {
		t.Fatalf("el admin entra con la contraseña inicial: %v", err)
	}
	if _, err := svc.SembrarAdmins(context.Background(), "corta", admins); !errors.Is(err, domain.ErrDatosInvalidos) {
		t.Fatalf("una ADMIN_PASSWORD corta se rechaza: %v", err)
	}
}
