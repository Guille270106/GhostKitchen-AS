// Package mysql implementa repositories.Usuarios sobre MySQL (base usuarios_db).
package mysql

import (
	"context"
	"database/sql"
	"errors"

	driver "github.com/go-sql-driver/mysql"

	"ghostkitchen/usuarios/internal/domain"
	"ghostkitchen/usuarios/internal/repositories"
)

// Repositorio guarda las cuentas en la tabla usuario (migrations/000002_usuarios.up.sql).
type Repositorio struct{ db *sql.DB }

var _ repositories.Usuarios = (*Repositorio)(nil)

// Nuevo crea el repositorio sobre una conexión ya abierta.
func Nuevo(db *sql.DB) *Repositorio { return &Repositorio{db: db} }

// columnas se leen siempre en este orden (ver escanear).
const columnas = `id,nombre,email,password_hash,COALESCE(direccion,''),rol,
 COALESCE(cocina_elegida_id,''),COALESCE(cocina_asignada_id,''),COALESCE(localidad,''),activo,creado_en`

// Crear inserta la cuenta. El correo único lo controla la base (uq_usuario_email), así dos
// registros con el mismo correo al mismo tiempo no pueden pasar los dos.
func (r *Repositorio) Crear(ctx context.Context, u domain.Usuario) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO usuario
 (id,nombre,email,password_hash,direccion,rol,cocina_elegida_id,cocina_asignada_id,localidad,activo)
 VALUES (?,?,?,?,?,?,?,?,?,?)`,
		u.ID, u.Nombre, u.Email, u.PasswordHash, nulo(u.Direccion), string(u.Rol),
		nulo(u.CocinaElegidaID), nulo(u.CocinaAsignadaID), nulo(u.Localidad), u.Activo)
	if esClaveRepetida(err) {
		return domain.ErrEmailRepetido
	}
	return err
}

// PorEmail busca por correo.
func (r *Repositorio) PorEmail(ctx context.Context, email string) (domain.Usuario, error) {
	return escanear(r.db.QueryRowContext(ctx, `SELECT `+columnas+` FROM usuario WHERE email=?`, email))
}

// PorID busca por id.
func (r *Repositorio) PorID(ctx context.Context, id string) (domain.Usuario, error) {
	return escanear(r.db.QueryRowContext(ctx, `SELECT `+columnas+` FROM usuario WHERE id=?`, id))
}

// Listo comprueba la conexión y que la migración 000002 esté aplicada, sin leer datos.
func (r *Repositorio) Listo(ctx context.Context) error {
	var n int
	return r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM usuario WHERE 1=0`).Scan(&n)
}

func escanear(fila *sql.Row) (domain.Usuario, error) {
	var u domain.Usuario
	var rol string
	err := fila.Scan(&u.ID, &u.Nombre, &u.Email, &u.PasswordHash, &u.Direccion, &rol,
		&u.CocinaElegidaID, &u.CocinaAsignadaID, &u.Localidad, &u.Activo, &u.CreadoEn)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Usuario{}, domain.ErrNoEncontrado
	}
	u.Rol = domain.Rol(rol)
	return u, err
}

// nulo guarda NULL en lugar de "" en las columnas opcionales, para que los CHECK por rol
// ("solo la cuenta Cocina tiene cocina asignada") funcionen.
func nulo(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// esClaveRepetida reconoce el error 1062 de MySQL (Duplicate entry). En esta tabla la única
// clave que se puede repetir desde afuera es el correo: el id lo genera el servicio.
func esClaveRepetida(err error) bool {
	var e *driver.MySQLError
	return errors.As(err, &e) && e.Number == 1062
}
