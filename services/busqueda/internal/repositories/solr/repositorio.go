package solr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"ghostkitchen/busqueda/internal/domain"
	"ghostkitchen/busqueda/internal/texto"
)

// Repositorio implementa repositories.Lector y repositories.Indexador sobre Solr.
type Repositorio struct {
	cli *Cliente
}

// NuevoRepositorio crea el repositorio.
func NuevoRepositorio(cli *Cliente) *Repositorio { return &Repositorio{cli: cli} }

// docCocina es una sede tal como se guarda en el core "cocinas".
type docCocina struct {
	ID               string `json:"id"`
	Nombre           string `json:"nombre"`
	Localidad        string `json:"localidad"`
	Direccion        string `json:"direccion"`
	Ubicacion        string `json:"ubicacion"` // "lat,lng"
	TiempoMin        int    `json:"tiempo_min"`
	TiempoMax        int    `json:"tiempo_max"`
	CostoEnvio       int    `json:"costo_envio"`
	RestaurantesJSON string `json:"restaurantes_json"`
	Activo           bool   `json:"activo"`
	// G15
	InactivosJSON   string `json:"restaurantes_inactivos_json,omitempty"`
	VersionCatalogo int64  `json:"version_catalogo"`
	Version         int64  `json:"_version_,omitempty"` // concurrencia optimista de Solr
}

// docPlato es un plato tal como se guarda en el core "platos".
type docPlato struct {
	ID                string   `json:"id"`
	Nombre            string   `json:"nombre"`
	Descripcion       string   `json:"descripcion,omitempty"`
	Categoria         string   `json:"categoria"`
	Etiquetas         []string `json:"etiquetas,omitempty"`
	Precio            int      `json:"precio"`
	Moneda            string   `json:"moneda"`
	FotoURL           string   `json:"foto_url,omitempty"`
	Disponible        bool     `json:"disponible"`
	Activo            bool     `json:"activo"`
	RestauranteID     string   `json:"restaurante_id"`
	RestauranteNombre string   `json:"restaurante_nombre"`
	CocinaID          string   `json:"cocina_id"`
	CocinaNombre      string   `json:"cocina_nombre"`
	ActualizadoEn     string   `json:"actualizado_en"`
	// G15. PlatoActivo es puntero porque los documentos de antes de G15 no lo tienen: en
	// ese caso vale lo mismo que Activo.
	PlatoActivo       *bool `json:"plato_activo,omitempty"`
	TieneReceta       bool  `json:"tiene_receta"`
	VersionCatalogo   int64 `json:"version_catalogo"`
	VersionInventario int64 `json:"version_inventario"`
	Version           int64 `json:"_version_,omitempty"` // concurrencia optimista de Solr
}

// ---------- Lectura ----------

// ListarSedes devuelve las sedes activas ordenadas por id.
func (r *Repositorio) ListarSedes(ctx context.Context) ([]domain.Sede, error) {
	p := url.Values{}
	p.Set("q", "*:*")
	p.Set("fq", "activo:true")
	p.Set("sort", "id asc")
	p.Set("rows", "100") // son 4 cocinas: 100 alcanza de sobra
	resp, err := r.cli.Select(ctx, CoreCocinas, p)
	if err != nil {
		return nil, err
	}
	return decodificarSedes(resp.Response.Docs)
}

// ObtenerSede busca por id con el real-time get, que no depende del último commit.
func (r *Repositorio) ObtenerSede(ctx context.Context, id string) (domain.Sede, bool, error) {
	raw, err := r.cli.Get(ctx, CoreCocinas, id)
	if err != nil || raw == nil {
		return domain.Sede{}, false, err
	}
	s, err := decodificarSede(raw)
	if err != nil {
		return domain.Sede{}, false, err
	}
	return s, s.Activa, nil
}

// SedeMasCercana le pide a Solr que ordene las cocinas activas por geodist(): la distancia
// desde pt (el punto del cliente) hasta el campo sfield (ubicacion) de cada una, y se queda
// con la primera. No filtra por radio: la regla de los 25 km la aplica services (RN24), que
// así puede distinguir "no hay sedes" de "la más cercana queda lejos".
func (r *Repositorio) SedeMasCercana(ctx context.Context, punto domain.Ubicacion) (domain.Sede, bool, error) {
	p := url.Values{}
	p.Set("q", "*:*")
	p.Set("fq", "activo:true")
	p.Set("sfield", "ubicacion")
	p.Set("pt", formatearPunto(punto))
	p.Set("sort", "geodist() asc,id asc") // empates por id
	p.Set("rows", "1")
	resp, err := r.cli.Select(ctx, CoreCocinas, p)
	if err != nil {
		return domain.Sede{}, false, err
	}
	sedes, err := decodificarSedes(resp.Response.Docs)
	if err != nil || len(sedes) == 0 {
		return domain.Sede{}, false, err
	}
	return sedes[0], true, nil
}

// BuscarPlatos devuelve una página de platos y el total (numFound) para calcular las páginas.
func (r *Repositorio) BuscarPlatos(ctx context.Context, f domain.FiltroPlatos) ([]domain.Plato, int, error) {
	resp, err := r.cli.Select(ctx, CorePlatos, ParametrosBusqueda(f))
	if err != nil {
		return nil, 0, err
	}
	platos := make([]domain.Plato, 0, len(resp.Response.Docs))
	for _, raw := range resp.Response.Docs {
		pl, err := decodificarPlato(raw)
		if err != nil {
			return nil, 0, err
		}
		platos = append(platos, pl)
	}
	return platos, resp.Response.NumFound, nil
}

// ObtenerPlato busca por id con el real-time get; un plato inactivo cuenta como no encontrado.
func (r *Repositorio) ObtenerPlato(ctx context.Context, id string) (domain.Plato, bool, error) {
	raw, err := r.cli.Get(ctx, CorePlatos, id)
	if err != nil || raw == nil {
		return domain.Plato{}, false, err
	}
	pl, err := decodificarPlato(raw)
	if err != nil {
		return domain.Plato{}, false, err
	}
	return pl, pl.Activo, nil
}

// Ping hace la consulta más barata posible (rows=0) en los dos cores, para ver si Solr
// responde. Revisa los dos (G22): si se borró uno, Búsqueda no puede atender aunque el otro ande.
func (r *Repositorio) Ping(ctx context.Context) error {
	p := url.Values{}
	p.Set("q", "*:*")
	p.Set("rows", "0")
	for _, core := range []string{CoreCocinas, CorePlatos} {
		if _, err := r.cli.Select(ctx, core, p); err != nil {
			return err
		}
	}
	return nil
}

// Confirmar hace commit en los dos cores: todo lo escrito queda visible en las búsquedas.
func (r *Repositorio) Confirmar(ctx context.Context) error {
	for _, core := range []string{CoreCocinas, CorePlatos} {
		if err := r.cli.Confirmar(ctx, core); err != nil {
			return err
		}
	}
	return nil
}

// ParametrosBusqueda traduce el filtro a parámetros de Solr (exportada para testearla).
//
// El texto va en q con edismax, que busca en varios campos a la vez con distinto peso (qf):
// que la palabra esté en el nombre pesa 3 veces más que en la descripción. Todo lo demás va en
// fq (filtros): no cambia el puntaje de relevancia y Solr guarda cada filtro en caché, así
// "activo:true" o "cocina_id:..." se reusan entre búsquedas (ADR-006).
func ParametrosBusqueda(f domain.FiltroPlatos) url.Values {
	p := url.Values{}
	if t := strings.TrimSpace(f.Texto); t != "" {
		p.Set("defType", "edismax")
		p.Set("q", consultaTexto(t))
		p.Set("qf", "nombre^3 restaurante_nombre^2 categoria_txt^2 descripcion")
		// Todas las palabras son obligatorias porque consultaTexto les pone "+". mm=1 es para el
		// grupo de la última palabra, "(hambur OR hambur*)": con mm=100% Solr también le
		// exigiría las dos partes al grupo, y "hamburguesa*" nunca coincide con "hamburgues".
		p.Set("mm", "1")
	} else {
		p.Set("q", "*:*")
	}
	p.Add("fq", "activo:true")
	p.Add("fq", "cocina_id:"+frase(f.SedeID))
	if f.SoloDisponibles {
		p.Add("fq", "disponible:true")
	}
	if f.Categoria != "" {
		p.Add("fq", "categoria:"+frase(f.Categoria))
	}
	if f.Etiqueta != "" {
		p.Add("fq", "etiquetas:"+frase(f.Etiqueta))
	}
	if f.RestauranteID != "" {
		p.Add("fq", "restaurante_id:"+frase(f.RestauranteID))
	}
	if f.PrecioMax > 0 {
		p.Add("fq", fmt.Sprintf("precio:[* TO %d]", f.PrecioMax))
	}
	// Siempre se desempata por id: sin un orden total, un plato podría aparecer en dos
	// páginas o en ninguna al pasar de una a la otra.
	switch f.Orden {
	case domain.OrdenPrecioAsc:
		p.Set("sort", "precio asc,id asc")
	case domain.OrdenPrecioDesc:
		p.Set("sort", "precio desc,id asc")
	default:
		p.Set("sort", "score desc,id asc")
	}
	p.Set("start", strconv.Itoa((f.Pagina-1)*f.TamPagina))
	p.Set("rows", strconv.Itoa(f.TamPagina))
	return p
}

// ---------- Escritura ----------

// GuardarSedes agrega o reemplaza sedes por id. La ubicación se guarda como "lat,lng",
// el formato del tipo location de Solr.
func (r *Repositorio) GuardarSedes(ctx context.Context, sedes []domain.Sede) error {
	docs := make([]docCocina, 0, len(sedes))
	for _, s := range sedes {
		d, err := docDeSede(s)
		if err != nil {
			return err
		}
		docs = append(docs, d)
	}
	return r.cli.Escribir(ctx, CoreCocinas, docs)
}

// GuardarPlatos agrega o reemplaza platos por id. La fecha va en UTC y RFC 3339, el formato
// que acepta el tipo pdate.
func (r *Repositorio) GuardarPlatos(ctx context.Context, platos []domain.Plato) error {
	docs := make([]docPlato, 0, len(platos))
	for _, p := range platos {
		docs = append(docs, docDePlato(p))
	}
	return r.cli.Escribir(ctx, CorePlatos, docs)
}

// intentosConflicto es cuántas veces se vuelve a leer y escribir un documento si otro lo
// cambió en el medio. Con un solo lector de mensajes y una reconstrucción, alcanza y sobra.
const intentosConflicto = 5

// CambiarSede lee la sede con el real-time get (ve lo último aunque no haya commit), llama a
// cambiar y la guarda con el _version_ leído (repositories.Actualizador).
func (r *Repositorio) CambiarSede(ctx context.Context, id string, cambiar func(domain.Sede, bool) (domain.Sede, bool, error)) error {
	for intento := 1; ; intento++ {
		raw, err := r.cli.Get(ctx, CoreCocinas, id)
		if err != nil {
			return err
		}
		var actual domain.Sede
		version := int64(-1) // -1: Solr solo la guarda si sigue sin existir
		if raw != nil {
			if actual, err = decodificarSede(raw); err != nil {
				return err
			}
			if version, err = leerVersion(raw); err != nil {
				return err
			}
		}
		nueva, guardar, err := cambiar(actual, raw != nil)
		if err != nil || !guardar {
			return err
		}
		doc, err := docDeSede(nueva)
		if err != nil {
			return err
		}
		doc.Version = version
		err = r.cli.Escribir(ctx, CoreCocinas, []docCocina{doc})
		if !errors.Is(err, ErrConflicto) || intento == intentosConflicto {
			return err
		}
	}
}

// CambiarPlato es lo mismo que CambiarSede, para un plato.
func (r *Repositorio) CambiarPlato(ctx context.Context, id string, cambiar func(domain.Plato, bool) (domain.Plato, bool, error)) error {
	for intento := 1; ; intento++ {
		raw, err := r.cli.Get(ctx, CorePlatos, id)
		if err != nil {
			return err
		}
		var actual domain.Plato
		version := int64(-1)
		if raw != nil {
			if actual, err = decodificarPlato(raw); err != nil {
				return err
			}
			if version, err = leerVersion(raw); err != nil {
				return err
			}
		}
		nuevo, guardar, err := cambiar(actual, raw != nil)
		if err != nil || !guardar {
			return err
		}
		doc := docDePlato(nuevo)
		doc.Version = version
		err = r.cli.Escribir(ctx, CorePlatos, []docPlato{doc})
		if !errors.Is(err, ErrConflicto) || intento == intentosConflicto {
			return err
		}
	}
}

// IDsPlatosDeSede devuelve los ids de todos los platos de la sede, activos o no. Antes hace
// un commit para que /select vea también los platos recién escritos por mensajes (que con
// commitWithin tardan hasta 1 s en aparecer). Lo usa el mensaje de una cocina, que es raro.
func (r *Repositorio) IDsPlatosDeSede(ctx context.Context, sedeID string) ([]string, error) {
	if err := r.cli.Confirmar(ctx, CorePlatos); err != nil {
		return nil, err
	}
	p := url.Values{}
	p.Set("q", "*:*")
	p.Set("fq", "cocina_id:"+frase(sedeID))
	p.Set("fl", "id")
	p.Set("sort", "id asc")
	p.Set("rows", "1000") // una cocina tiene a lo sumo 4 restaurantes con sus platos
	resp, err := r.cli.Select(ctx, CorePlatos, p)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(resp.Response.Docs))
	for _, raw := range resp.Response.Docs {
		var d struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &d); err != nil {
			return nil, fmt.Errorf("%w: id inválido: %v", domain.ErrNoDisponible, err)
		}
		ids = append(ids, d.ID)
	}
	return ids, nil
}

// ---------- Conversión ----------

// leerVersion saca el _version_ de un documento. Se decodifica como int64 (y no como número
// genérico) porque es un número de 19 cifras que un float64 redondearía.
func leerVersion(raw json.RawMessage) (int64, error) {
	var d struct {
		Version int64 `json:"_version_"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return 0, fmt.Errorf("%w: _version_ inválido: %v", domain.ErrNoDisponible, err)
	}
	return d.Version, nil
}

// docDeSede arma el documento de una sede. La ubicación se guarda como "lat,lng", el formato
// del tipo location de Solr.
func docDeSede(s domain.Sede) (docCocina, error) {
	rest := s.Restaurantes
	if rest == nil {
		rest = []domain.Referencia{}
	}
	activos, err := json.Marshal(rest)
	if err != nil {
		return docCocina{}, err
	}
	d := docCocina{
		ID: s.ID, Nombre: s.Nombre, Localidad: s.Localidad, Direccion: s.Direccion,
		Ubicacion: formatearPunto(s.Ubicacion), TiempoMin: s.TiempoEntregaMin,
		TiempoMax: s.TiempoEntregaMax, CostoEnvio: s.CostoEnvio,
		RestaurantesJSON: string(activos), Activo: s.Activa, VersionCatalogo: s.VersionCatalogo,
	}
	if len(s.RestaurantesInactivos) > 0 {
		inactivos, err := json.Marshal(s.RestaurantesInactivos)
		if err != nil {
			return docCocina{}, err
		}
		d.InactivosJSON = string(inactivos)
	}
	return d, nil
}

// docDePlato arma el documento de un plato. La fecha va en UTC y RFC 3339, el formato que
// acepta el tipo pdate.
func docDePlato(p domain.Plato) docPlato {
	propio := p.PlatoActivo
	return docPlato{
		ID: p.ID, Nombre: p.Nombre, Descripcion: p.Descripcion, Categoria: p.Categoria,
		Etiquetas: p.Etiquetas, Precio: p.Precio, Moneda: p.Moneda, FotoURL: p.FotoURL,
		Disponible: p.Disponible, Activo: p.Activo,
		RestauranteID: p.Restaurante.ID, RestauranteNombre: p.Restaurante.Nombre,
		CocinaID: p.Sede.ID, CocinaNombre: p.Sede.Nombre,
		ActualizadoEn: p.ActualizadoEn.UTC().Format(time.RFC3339),
		PlatoActivo:   &propio, TieneReceta: p.TieneReceta,
		VersionCatalogo: p.VersionCatalogo, VersionInventario: p.VersionInventario,
	}
}

func decodificarSedes(raws []json.RawMessage) ([]domain.Sede, error) {
	sedes := make([]domain.Sede, 0, len(raws))
	for _, raw := range raws {
		s, err := decodificarSede(raw)
		if err != nil {
			return nil, err
		}
		sedes = append(sedes, s)
	}
	return sedes, nil
}

func decodificarSede(raw json.RawMessage) (domain.Sede, error) {
	var d docCocina
	if err := json.Unmarshal(raw, &d); err != nil {
		return domain.Sede{}, fmt.Errorf("%w: cocina inválida: %v", domain.ErrNoDisponible, err)
	}
	ub, err := leerPunto(d.Ubicacion)
	if err != nil {
		return domain.Sede{}, fmt.Errorf("%w: cocina %s: %v", domain.ErrNoDisponible, d.ID, err)
	}
	rest := []domain.Referencia{}
	if d.RestaurantesJSON != "" {
		if err := json.Unmarshal([]byte(d.RestaurantesJSON), &rest); err != nil {
			return domain.Sede{}, fmt.Errorf("%w: restaurantes de %s: %v", domain.ErrNoDisponible, d.ID, err)
		}
	}
	var inactivos []domain.Referencia
	if d.InactivosJSON != "" {
		if err := json.Unmarshal([]byte(d.InactivosJSON), &inactivos); err != nil {
			return domain.Sede{}, fmt.Errorf("%w: restaurantes inactivos de %s: %v", domain.ErrNoDisponible, d.ID, err)
		}
	}
	return domain.Sede{
		ID: d.ID, Nombre: d.Nombre, Localidad: d.Localidad, Direccion: d.Direccion,
		Ubicacion: ub, TiempoEntregaMin: d.TiempoMin, TiempoEntregaMax: d.TiempoMax,
		CostoEnvio: d.CostoEnvio, Restaurantes: rest, Activa: d.Activo,
		RestaurantesInactivos: inactivos, VersionCatalogo: d.VersionCatalogo,
	}, nil
}

func decodificarPlato(raw json.RawMessage) (domain.Plato, error) {
	var d docPlato
	if err := json.Unmarshal(raw, &d); err != nil {
		return domain.Plato{}, fmt.Errorf("%w: plato inválido: %v", domain.ErrNoDisponible, err)
	}
	act, _ := time.Parse(time.RFC3339, d.ActualizadoEn)
	propio := d.Activo // documento de antes de G15
	if d.PlatoActivo != nil {
		propio = *d.PlatoActivo
	}
	etq := d.Etiquetas
	if etq == nil {
		etq = []string{}
	}
	return domain.Plato{
		ID: d.ID, Nombre: d.Nombre, Descripcion: d.Descripcion, Categoria: d.Categoria,
		Etiquetas: etq, Precio: d.Precio, Moneda: d.Moneda, FotoURL: d.FotoURL,
		Disponible: d.Disponible, Activo: d.Activo,
		Restaurante:   domain.Referencia{ID: d.RestauranteID, Nombre: d.RestauranteNombre},
		Sede:          domain.Referencia{ID: d.CocinaID, Nombre: d.CocinaNombre},
		ActualizadoEn: act.UTC(),
		PlatoActivo:   propio, TieneReceta: d.TieneReceta,
		VersionCatalogo: d.VersionCatalogo, VersionInventario: d.VersionInventario,
	}, nil
}

func formatearPunto(u domain.Ubicacion) string {
	return strconv.FormatFloat(u.Lat, 'f', -1, 64) + "," + strconv.FormatFloat(u.Lng, 'f', -1, 64)
}

func leerPunto(s string) (domain.Ubicacion, error) {
	partes := strings.Split(s, ",")
	if len(partes) != 2 {
		return domain.Ubicacion{}, fmt.Errorf("ubicación %q inválida", s)
	}
	lat, err1 := strconv.ParseFloat(strings.TrimSpace(partes[0]), 64)
	lng, err2 := strconv.ParseFloat(strings.TrimSpace(partes[1]), 64)
	if err1 != nil || err2 != nil {
		return domain.Ubicacion{}, fmt.Errorf("ubicación %q inválida", s)
	}
	return domain.Ubicacion{Lat: lat, Lng: lng}, nil
}

// consultaTexto arma la consulta del texto que escribió la persona. Se queda solo con las
// palabras (letras y números): los signos los descarta igual el análisis de text_gk, y así no
// pueden armar sintaxis de Solr. Van en minúsculas, para que "AND", "OR" o "NOT" se busquen
// como palabras y no como operadores. Cada una lleva "+": tienen que aparecer todas.
//
// La última, si tiene al menos 3 letras, se busca también como comienzo de palabra: así
// "hambur" encuentra "Hamburguesa" mientras se escribe (búsqueda y sugerencias del frontend,
// G18). Va "hambur OR hambur*" y no solo "hambur*" porque el comodín no pasa por la raíz en
// español: "hamburguesas*" no encontraría "hamburgues", que es lo que guarda el índice. La
// palabra entera sí.
func consultaTexto(t string) string {
	palabras := strings.FieldsFunc(strings.ToLower(t), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(palabras) == 0 {
		return "*:*" // solo signos: como sin texto
	}
	partes := make([]string, len(palabras))
	for i, w := range palabras {
		partes[i] = "+" + w
	}
	if ultima := palabras[len(palabras)-1]; len([]rune(ultima)) >= texto.MinPrefijo {
		partes[len(partes)-1] = "+(" + ultima + " OR " + ultima + "*)"
	}
	return strings.Join(partes, " ")
}

// frase arma un valor exacto entre comillas para un filtro (fq). Escapar \ y " impide que un
// valor que manda el consumidor cierre las comillas y agregue condiciones propias al filtro.
func frase(v string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(v) + `"`
}
