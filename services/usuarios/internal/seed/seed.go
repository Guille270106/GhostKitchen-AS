// Package seed tiene las cuentas de admin que se cargan al iniciar el sistema: un admin por
// localidad, la misma que tienen las cocinas en los datos iniciales de Catálogo. Los admins no
// se pueden registrar solos (README de Usuarios).
//
// El archivo no tiene contraseñas: la contraseña inicial viene de ADMIN_PASSWORD (.env).
package seed

import (
	_ "embed"
	"encoding/json"
)

//go:embed admins.json
var adminsJSON []byte

// Admin es una cuenta de admin inicial.
type Admin struct {
	Nombre    string `json:"nombre"`
	Email     string `json:"email"`
	Localidad string `json:"localidad"`
}

// Admins devuelve las cuentas iniciales.
func Admins() ([]Admin, error) {
	var a []Admin
	err := json.Unmarshal(adminsJSON, &a)
	return a, err
}
