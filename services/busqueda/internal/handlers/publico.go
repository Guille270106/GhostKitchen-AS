package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"ghostkitchen/busqueda/internal/domain"
	"ghostkitchen/busqueda/internal/services"
)

// MaxCuerpoProximidad es el tamaño máximo del cuerpo de POST /v1/proximidad (64 KiB).
// El gateway ya lo controla; el servicio lo vuelve a controlar por si lo llaman directo.
const MaxCuerpoProximidad = 64 << 10

// patronSedeID es el formato de id de sede del contrato. Validarlo antes evita mandarle
// a Solr valores raros y responde 400 en vez de un 404 confuso.
var patronSedeID = regexp.MustCompile(`^[a-z0-9-]+$`)

// Publico atiende la capacidad publicada (/v1). La API key y el límite de uso
// los controla el gateway antes de llegar acá.
type Publico struct {
	svc *services.Busqueda
}

// NuevoPublico crea los handlers de /v1.
func NuevoPublico(svc *services.Busqueda) *Publico { return &Publico{svc: svc} }

// Registrar agrega las rutas de /v1.
func (h *Publico) Registrar(r gin.IRoutes) {
	r.GET("/v1/sedes", h.listarSedes)
	r.GET("/v1/sedes/cercana", h.sedeCercana)
	r.GET("/v1/sedes/:sedeId/platos", h.buscarPlatos)
	r.GET("/v1/platos/:platoId", h.obtenerPlato)
	r.POST("/v1/proximidad", h.proximidad)
}

// GET /v1/sedes
func (h *Publico) listarSedes(c *gin.Context) {
	sedes, err := h.svc.ListarSedes(c.Request.Context())
	if err != nil {
		responderError(c, err, "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": sedes})
}

// GET /v1/sedes/cercana?lat=&lng=
func (h *Publico) sedeCercana(c *gin.Context) {
	var errs []ErrorParametro
	lat := leerCoordenada(c, "lat", 90, &errs)
	lng := leerCoordenada(c, "lng", 180, &errs)
	if len(errs) > 0 {
		parametrosInvalidos(c, errs)
		return
	}
	sede, dist, err := h.svc.SedeCercana(c.Request.Context(), domain.Ubicacion{Lat: lat, Lng: lng})
	if err != nil {
		responderError(c, err, "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"sede": sede, "distanciaKm": dist})
}

// GET /v1/sedes/:sedeId/platos
func (h *Publico) buscarPlatos(c *gin.Context) {
	f, errs := leerFiltro(c)
	if len(errs) > 0 {
		parametrosInvalidos(c, errs)
		return
	}
	res, err := h.svc.BuscarPlatos(c.Request.Context(), f)
	if err != nil {
		responderError(c, err, fmt.Sprintf("No existe la sede '%s' o no está activa.", f.SedeID))
		return
	}
	c.JSON(http.StatusOK, res)
}

// GET /v1/platos/:platoId
func (h *Publico) obtenerPlato(c *gin.Context) {
	id := c.Param("platoId")
	p, err := h.svc.ObtenerPlato(c.Request.Context(), id)
	if err != nil {
		responderError(c, err, fmt.Sprintf("El plato '%s' no existe o fue desactivado.", id))
		return
	}
	c.JSON(http.StatusOK, p)
}

// POST /v1/proximidad
func (h *Publico) proximidad(c *gin.Context) {
	q, errs, muyGrande := leerConsultaProximidad(c.Writer, c.Request)
	if muyGrande {
		responderProblema(c, http.StatusRequestEntityTooLarge, CodigoSolicitudMuyGrande,
			"El cuerpo JSON no puede superar 64 KiB.")
		return
	}
	if len(errs) > 0 {
		parametrosInvalidos(c, errs)
		return
	}
	res, err := h.svc.Proximidad(c.Request.Context(), q)
	if err != nil {
		responderError(c, err, "")
		return
	}
	c.JSON(http.StatusOK, res)
}

// ---------- Lectura y validación de parámetros ----------
//
// Las funciones de lectura juntan todos los errores en lugar de cortar en el primero, así
// el consumidor ve en una sola respuesta 400 todo lo que tiene que corregir.

// leerCoordenada lee lat o lng de la query. Rechaza "NaN" e "Inf" porque ParseFloat los
// acepta como números, pero no son coordenadas.
func leerCoordenada(c *gin.Context, nombre string, limite float64, errs *[]ErrorParametro) float64 {
	v, ok := c.GetQuery(nombre)
	if !ok || strings.TrimSpace(v) == "" {
		*errs = append(*errs, ErrorParametro{nombre, "Es obligatorio."})
		return 0
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		*errs = append(*errs, ErrorParametro{nombre, "Debe ser un número."})
		return 0
	}
	if n < -limite || n > limite {
		*errs = append(*errs, ErrorParametro{nombre, fmt.Sprintf("Debe estar entre -%g y %g.", limite, limite)})
	}
	return n
}

// leerFiltro arma el filtro de búsqueda de platos con los valores por defecto del contrato:
// solo disponibles, orden por relevancia, página 1 de 20. pageSize tiene un máximo de 50
// para que nadie pida respuestas enormes.
func leerFiltro(c *gin.Context) (domain.FiltroPlatos, []ErrorParametro) {
	var errs []ErrorParametro
	f := domain.FiltroPlatos{
		SedeID:          c.Param("sedeId"),
		Texto:           strings.TrimSpace(c.Query("q")),
		Categoria:       strings.TrimSpace(c.Query("categoria")),
		RestauranteID:   strings.TrimSpace(c.Query("restauranteId")),
		SoloDisponibles: true,
		Orden:           domain.OrdenRelevancia,
		Pagina:          1,
		TamPagina:       20,
	}
	if !patronSedeID.MatchString(f.SedeID) {
		errs = append(errs, ErrorParametro{"sedeId", "Solo minúsculas, números y guiones."})
	}
	if utf8.RuneCountInString(c.Query("q")) > 100 {
		errs = append(errs, ErrorParametro{"q", "No puede superar 100 caracteres."})
	}
	if v, ok := c.GetQuery("etiqueta"); ok {
		if !domain.EtiquetasValidas[v] {
			errs = append(errs, ErrorParametro{"etiqueta", "Debe ser Vegano, Vegetariano, Sin TACC o Picante."})
		}
		f.Etiqueta = v
	}
	if v, ok := c.GetQuery("precioMax"); ok {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			errs = append(errs, ErrorParametro{"precioMax", "Debe ser un entero mayor o igual a 1."})
		}
		f.PrecioMax = n
	}
	if v, ok := c.GetQuery("soloDisponibles"); ok {
		switch v {
		case "true":
		case "false":
			f.SoloDisponibles = false
		default:
			errs = append(errs, ErrorParametro{"soloDisponibles", "Debe ser true o false."})
		}
	}
	if v, ok := c.GetQuery("orden"); ok {
		switch domain.Orden(v) {
		case domain.OrdenRelevancia, domain.OrdenPrecioAsc, domain.OrdenPrecioDesc:
			f.Orden = domain.Orden(v)
		default:
			errs = append(errs, ErrorParametro{"orden", "Debe ser relevancia, precio_asc o precio_desc."})
		}
	}
	if v, ok := c.GetQuery("page"); ok {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			errs = append(errs, ErrorParametro{"page", "Debe ser un entero mayor o igual a 1."})
		}
		f.Pagina = n
	}
	if v, ok := c.GetQuery("pageSize"); ok {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 50 {
			errs = append(errs, ErrorParametro{"pageSize", "Debe ser un entero entre 1 y 50."})
		}
		f.TamPagina = n
	}
	return f, errs
}

// Formato de entrada de POST /v1/proximidad. Los punteros permiten distinguir
// un campo ausente (o null) de un cero.
type coordenadasEntrada struct {
	Lat *float64 `json:"lat"`
	Lng *float64 `json:"lng"`
}

type consultaEntrada struct {
	UbicacionCliente   *coordenadasEntrada `json:"ubicacionCliente"`
	UbicacionSede      *coordenadasEntrada `json:"ubicacionSede"`
	RadioKm            *float64            `json:"radioKm"`
	IncluirDarkKitchen *bool               `json:"incluirDarkKitchen"`
}

// leerConsultaProximidad valida el cuerpo según RN26: exactamente un objeto JSON,
// sin campos extra, sin listas y con tipos correctos (un booleano como texto es error).
func leerConsultaProximidad(w http.ResponseWriter, r *http.Request) (services.ConsultaProximidad, []ErrorParametro, bool) {
	cuerpo := http.MaxBytesReader(w, r.Body, MaxCuerpoProximidad)
	dec := json.NewDecoder(cuerpo)
	dec.DisallowUnknownFields()

	var in consultaEntrada
	if err := dec.Decode(&in); err != nil {
		var grande *http.MaxBytesError
		if errors.As(err, &grande) {
			return services.ConsultaProximidad{}, nil, true
		}
		return services.ConsultaProximidad{}, []ErrorParametro{errorJSON(err)}, false
	}
	// Un segundo Decode tiene que dar EOF: así se rechaza un cuerpo con dos objetos
	// seguidos o con basura después del primero.
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		var grande *http.MaxBytesError
		if errors.As(err, &grande) {
			return services.ConsultaProximidad{}, nil, true
		}
		return services.ConsultaProximidad{}, []ErrorParametro{{"cuerpo", "Se espera un único objeto JSON."}}, false
	}

	var errs []ErrorParametro
	cliente := validarCoordenadas("ubicacionCliente", in.UbicacionCliente, &errs)
	sede := validarCoordenadas("ubicacionSede", in.UbicacionSede, &errs)
	radio := 0.0
	switch {
	case in.RadioKm == nil:
		errs = append(errs, ErrorParametro{"radioKm", "Es obligatorio."})
	case *in.RadioKm <= 0 || *in.RadioKm > 500:
		errs = append(errs, ErrorParametro{"radioKm", "Debe ser mayor que 0 y como máximo 500."})
	default:
		radio = *in.RadioKm
	}
	incluir := in.IncluirDarkKitchen != nil && *in.IncluirDarkKitchen // ausente = false
	return services.ConsultaProximidad{Cliente: cliente, Sede: sede, RadioKm: radio, IncluirDarkKitchen: incluir}, errs, false
}

// validarCoordenadas revisa un objeto {lat, lng} del cuerpo. Los mensajes nombran el campo
// completo (ej. ubicacionCliente.lat) para que el consumidor sepa cuál corregir.
func validarCoordenadas(nombre string, c *coordenadasEntrada, errs *[]ErrorParametro) domain.Ubicacion {
	if c == nil {
		*errs = append(*errs, ErrorParametro{nombre, "Es obligatorio."})
		return domain.Ubicacion{}
	}
	var u domain.Ubicacion
	if c.Lat == nil {
		*errs = append(*errs, ErrorParametro{nombre + ".lat", "Es obligatorio."})
	} else if *c.Lat < -90 || *c.Lat > 90 {
		*errs = append(*errs, ErrorParametro{nombre + ".lat", "Debe estar entre -90 y 90."})
	} else {
		u.Lat = *c.Lat
	}
	if c.Lng == nil {
		*errs = append(*errs, ErrorParametro{nombre + ".lng", "Es obligatorio."})
	} else if *c.Lng < -180 || *c.Lng > 180 {
		*errs = append(*errs, ErrorParametro{nombre + ".lng", "Debe estar entre -180 y 180."})
	} else {
		u.Lng = *c.Lng
	}
	return u
}

// errorJSON traduce un error de decodificación a un mensaje para el consumidor.
func errorJSON(err error) ErrorParametro {
	var tipo *json.UnmarshalTypeError
	var sintaxis *json.SyntaxError
	switch {
	case errors.As(err, &tipo):
		if tipo.Field == "" {
			return ErrorParametro{"cuerpo", "Se espera un objeto JSON (no se aceptan listas)."}
		}
		return ErrorParametro{tipo.Field, "Tipo inválido: se esperaba " + nombreTipo(tipo.Type.String()) + "."}
	case errors.As(err, &sintaxis), errors.Is(err, io.ErrUnexpectedEOF):
		return ErrorParametro{"cuerpo", "JSON mal formado."}
	case errors.Is(err, io.EOF):
		return ErrorParametro{"cuerpo", "El cuerpo es obligatorio."}
	// encoding/json no tiene un tipo de error para los campos desconocidos: solo se
	// pueden reconocer por el texto del mensaje.
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		campo := strings.Trim(strings.TrimPrefix(err.Error(), "json: unknown field "), `"`)
		return ErrorParametro{campo, "Campo no permitido."}
	default:
		return ErrorParametro{"cuerpo", "JSON inválido."}
	}
}

// nombreTipo traduce un tipo de Go a palabras que entiende alguien que no conoce Go.
func nombreTipo(t string) string {
	switch strings.TrimPrefix(t, "*") {
	case "bool":
		return "un booleano (true o false)"
	case "float64":
		return "un número"
	default:
		return "un objeto"
	}
}
