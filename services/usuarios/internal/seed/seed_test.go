package seed

import "testing"

// Las localidades tienen que ser las de las cocinas del seed de Catálogo: si no, ningún
// admin podría crear cuentas Cocina.
func TestUnAdminPorLocalidadDeCatalogo(t *testing.T) {
	admins, err := Admins()
	if err != nil {
		t.Fatal(err)
	}
	esperadas := map[string]bool{"Barrio Jardín": true, "Cerro de las Rosas": true, "Nueva Córdoba": true, "Villa Allende": true}
	for _, a := range admins {
		if !esperadas[a.Localidad] || a.Email == "" || a.Nombre == "" {
			t.Errorf("admin inesperado: %+v", a)
		}
		delete(esperadas, a.Localidad)
	}
	if len(esperadas) != 0 {
		t.Fatalf("faltan admins para: %v", esperadas)
	}
}
