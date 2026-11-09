package domain

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Errores de las escrituras del admin (HU14, HU16).
var (
	ErrDatosInvalidos          = errors.New("datos inválidos")
	ErrSinPermiso              = errors.New("solo un admin puede administrar cocinas")
	ErrOtraLocalidad           = errors.New("la cocina es de otra localidad")
	ErrYaExiste                = errors.New("ya existe")
	ErrCocinaLlena             = errors.New("la cocina ya tiene 4 restaurantes")
	ErrRestauranteRepetido     = errors.New("ya hay un restaurante con ese nombre en la cocina")
	ErrRestauranteNoEncontrado = errors.New("restaurante no encontrado")
	// ErrConflicto: otra solicitud cambió la cocina al mismo tiempo (la versión no coincide).
	// El servicio vuelve a leerla y reintenta; nunca pisa el cambio del otro.
	ErrConflicto = errors.New("la cocina cambió mientras se modificaba")
)

func invalido(motivo string) error { return fmt.Errorf("%w: %s", ErrDatosInvalidos, motivo) }

// CambiosCocina son los datos que el admin puede cambiar de una cocina. Un campo nil no se
// toca. La localidad no se puede cambiar: es la del admin que la creó.
type CambiosCocina struct {
	Nombre           *string    `json:"nombre"`
	Direccion        *string    `json:"direccion"`
	Ubicacion        *Ubicacion `json:"ubicacion"`
	TiempoEntregaMin *int       `json:"tiempoEntregaMin"`
	TiempoEntregaMax *int       `json:"tiempoEntregaMax"`
	CostoEnvio       *int       `json:"costoEnvio"`
	Activa           *bool      `json:"activa"`
}

// Aplicar cambia los campos indicados y valida cómo queda la cocina.
func (c *Cocina) Aplicar(cambios CambiosCocina) error {
	if cambios.Nombre != nil {
		c.Nombre = strings.TrimSpace(*cambios.Nombre)
	}
	if cambios.Direccion != nil {
		c.Direccion = strings.TrimSpace(*cambios.Direccion)
	}
	if cambios.Ubicacion != nil {
		c.Ubicacion = *cambios.Ubicacion
	}
	if cambios.TiempoEntregaMin != nil {
		c.TiempoEntregaMin = *cambios.TiempoEntregaMin
	}
	if cambios.TiempoEntregaMax != nil {
		c.TiempoEntregaMax = *cambios.TiempoEntregaMax
	}
	if cambios.CostoEnvio != nil {
		c.CostoEnvio = *cambios.CostoEnvio
	}
	if cambios.Activa != nil {
		c.Activa = *cambios.Activa
	}
	return c.Validar()
}

// Validar revisa los datos de la cocina que maneja el admin.
func (c Cocina) Validar() error {
	switch {
	case c.Nombre == "" || utf8.RuneCountInString(c.Nombre) > 80:
		return invalido("el nombre es obligatorio y tiene hasta 80 caracteres")
	case c.Direccion == "" || utf8.RuneCountInString(c.Direccion) > 120:
		return invalido("la dirección es obligatoria y tiene hasta 120 caracteres")
	case c.Ubicacion.Lat < -90 || c.Ubicacion.Lat > 90 || c.Ubicacion.Lng < -180 || c.Ubicacion.Lng > 180 ||
		(c.Ubicacion.Lat == 0 && c.Ubicacion.Lng == 0):
		// 0,0 es el valor que queda si no se manda la ubicación: está en el océano Atlántico.
		return invalido("la ubicación no es válida")
	case c.TiempoEntregaMin < 1 || c.TiempoEntregaMax > 240 || c.TiempoEntregaMin > c.TiempoEntregaMax:
		return invalido("el tiempo de entrega va de 1 a 240 minutos, y el mínimo no puede superar al máximo")
	case c.CostoEnvio < 0 || c.CostoEnvio > 1_000_000:
		return invalido("el costo de envío no es válido")
	}
	return nil
}

// AgregarRestaurante suma una marca a la cocina (HU16). Rechaza la quinta y un nombre que
// ya esté en la cocina, aunque cambien mayúsculas o acentos (RN20). Los desactivados también
// cuentan: no se borran (RN19), así que siguen ocupando su lugar y su nombre.
func (c *Cocina) AgregarRestaurante(r Restaurante) error {
	r.Nombre = strings.TrimSpace(r.Nombre)
	if err := validarNombreRestaurante(r.Nombre); err != nil {
		return err
	}
	// Primero el nombre repetido: es el motivo más preciso si pasan las dos cosas.
	if c.nombreOcupado(r.Nombre, "") {
		return ErrRestauranteRepetido
	}
	if len(c.Restaurantes) >= MaxRestaurantes {
		return ErrCocinaLlena
	}
	c.Restaurantes = append(c.Restaurantes, r)
	return nil
}

// CambiosRestaurante son los datos que se pueden cambiar de un restaurante.
type CambiosRestaurante struct {
	Nombre *string `json:"nombre"`
	Activo *bool   `json:"activo"`
}

// CambiarRestaurante renombra, desactiva o reactiva una marca de la cocina.
func (c *Cocina) CambiarRestaurante(id string, cambios CambiosRestaurante) (Restaurante, error) {
	for i := range c.Restaurantes {
		if c.Restaurantes[i].ID != id {
			continue
		}
		if cambios.Nombre != nil {
			nombre := strings.TrimSpace(*cambios.Nombre)
			if err := validarNombreRestaurante(nombre); err != nil {
				return Restaurante{}, err
			}
			if c.nombreOcupado(nombre, id) {
				return Restaurante{}, ErrRestauranteRepetido
			}
			c.Restaurantes[i].Nombre = nombre
		}
		if cambios.Activo != nil {
			c.Restaurantes[i].Activo = *cambios.Activo
		}
		return c.Restaurantes[i], nil
	}
	return Restaurante{}, ErrRestauranteNoEncontrado
}

func validarNombreRestaurante(nombre string) error {
	if nombre == "" || utf8.RuneCountInString(nombre) > 60 {
		return invalido("el nombre del restaurante es obligatorio y tiene hasta 60 caracteres")
	}
	return nil
}

// nombreOcupado dice si otro restaurante de la cocina (distinto de excepto) ya usa ese nombre.
func (c *Cocina) nombreOcupado(nombre, excepto string) bool {
	clave := Clave(nombre)
	for _, r := range c.Restaurantes {
		if r.ID != excepto && Clave(r.Nombre) == clave {
			return true
		}
	}
	return false
}

// Clave normaliza un nombre para compararlo: minúsculas, sin acentos y con los espacios
// juntos. "Café  Martínez" y "cafe martinez" tienen la misma clave.
func Clave(s string) string {
	return strings.Join(strings.Fields(sinAcentos.Replace(strings.ToLower(s))), " ")
}

var sinAcentos = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ä", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ñ", "n", "ç", "c",
)

// IDDesdeNombre arma el id de una cocina nueva a partir de su nombre, igual que los de los
// datos iniciales: "Cocina Alta Córdoba" queda "alta-cordoba".
func IDDesdeNombre(nombre string) string {
	clave := strings.TrimPrefix(Clave(nombre), "cocina ")
	var b strings.Builder
	guion := false
	for _, r := range clave {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			guion = false
		} else if !guion && b.Len() > 0 {
			b.WriteByte('-')
			guion = true
		}
	}
	id := strings.TrimSuffix(b.String(), "-")
	if len(id) > 40 {
		id = strings.TrimSuffix(id[:40], "-")
	}
	return id
}
