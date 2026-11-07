package mysql

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"

	"ghostkitchen/usuarios/internal/domain"
)

// Estas pruebas usan un MySQL real con la migración aplicada. Se omiten salvo con TEST_MYSQL=1
// y DB_HOST, DB_USER, DB_PASSWORD y DB_NAME de una base descartable (nunca la del proyecto).
func conectar(t *testing.T) *sql.DB {
	t.Helper()
	if os.Getenv("TEST_MYSQL") != "1" {
		t.Skip("requiere MySQL descartable (TEST_MYSQL=1)")
	}
	cfg := driver.NewConfig()
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(os.Getenv("DB_HOST"), "3306")
	cfg.User, cfg.Passwd, cfg.DBName = os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD"), os.Getenv("DB_NAME")
	cfg.ParseTime = true
	conn, err := driver.NewConnector(cfg)
	if err != nil {
		t.Fatal(err)
	}
	db := sql.OpenDB(conn)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCrearYBuscarContraMySQL(t *testing.T) {
	repo := Nuevo(conectar(t))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := repo.Listo(ctx); err != nil {
		t.Fatalf("la tabla usuario no está: %v", err)
	}
	email := uuid.NewString()[:8] + "@prueba.com"
	u := domain.Usuario{ID: uuid.NewString(), Nombre: "Ana", Email: email,
		PasswordHash: "$2a$10$abcdefghijklmnopqrstuuabcdefghijklmnopqrstuvwxyz01234",
		Rol:          domain.RolCliente, Activo: true}
	if err := repo.Crear(ctx, u); err != nil {
		t.Fatal(err)
	}
	otro := u
	otro.ID = uuid.NewString()
	if err := repo.Crear(ctx, otro); !errors.Is(err, domain.ErrEmailRepetido) {
		t.Fatalf("el mismo correo dos veces tenía que dar ErrEmailRepetido: %v", err)
	}
	got, err := repo.PorEmail(ctx, email)
	if err != nil || got.ID != u.ID || got.Rol != domain.RolCliente || got.CocinaAsignadaID != "" {
		t.Fatalf("PorEmail: %+v, %v", got, err)
	}
	if _, err := repo.PorID(ctx, uuid.NewString()); !errors.Is(err, domain.ErrNoEncontrado) {
		t.Fatalf("un id inexistente tenía que dar ErrNoEncontrado: %v", err)
	}
	// El CHECK de la base también frena una cuenta Cocina sin cocina.
	mala := domain.Usuario{ID: uuid.NewString(), Nombre: "X", Email: "x" + email, PasswordHash: u.PasswordHash, Rol: domain.RolCocina, Activo: true}
	if err := repo.Crear(ctx, mala); err == nil {
		t.Fatal("la base tenía que rechazar una cuenta Cocina sin cocina asignada")
	}
}
