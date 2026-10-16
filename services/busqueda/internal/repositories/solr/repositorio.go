package solr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ghostkitchen/busqueda/internal/domain"
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
}

// ---------- Lectura ----------

func (r *Repositorio) ListarSedes(ctx context.Context) ([]domain.Sede, error) {
	p := url.Values{}
	p.Set("q", "*:*")
	p.Set("fq", "activo:true")
	p.Set("sort", "id asc")
	p.Set("rows", "100")
	resp, err := r.cli.Select(ctx, CoreCocinas, p)
	if err != nil {
		return nil, err
	}
	return decodificarSedes(resp.Response.Docs)
}

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

func (r *Repositorio) Ping(ctx context.Context) error {
	p := url.Values{}
	p.Set("q", "*:*")
	p.Set("rows", "0")
	_, err := r.cli.Select(ctx, CoreCocinas, p)
	return err
}

// ParametrosBusqueda traduce el filtro a parámetros de Solr (exportada para testearla).
func ParametrosBusqueda(f domain.FiltroPlatos) url.Values {
	p := url.Values{}
	if t := strings.TrimSpace(f.Texto); t != "" {
		p.Set("defType", "edismax")
		p.Set("q", escaparTexto(t))
		p.Set("qf", "nombre^3 restaurante_nombre^2 categoria_txt^2 descripcion")
		p.Set("mm", "100%") // todas las palabras tienen que aparecer
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

func (r *Repositorio) GuardarSedes(ctx context.Context, sedes []domain.Sede) error {
	docs := make([]docCocina, 0, len(sedes))
	for _, s := range sedes {
		rest, err := json.Marshal(s.Restaurantes)
		if err != nil {
			return err
		}
		docs = append(docs, docCocina{
			ID: s.ID, Nombre: s.Nombre, Localidad: s.Localidad, Direccion: s.Direccion,
			Ubicacion: formatearPunto(s.Ubicacion), TiempoMin: s.TiempoEntregaMin,
			TiempoMax: s.TiempoEntregaMax, CostoEnvio: s.CostoEnvio,
			RestaurantesJSON: string(rest), Activo: s.Activa,
		})
	}
	return r.cli.Actualizar(ctx, CoreCocinas, docs)
}

func (r *Repositorio) GuardarPlatos(ctx context.Context, platos []domain.Plato) error {
	docs := make([]docPlato, 0, len(platos))
	for _, p := range platos {
		docs = append(docs, docPlato{
			ID: p.ID, Nombre: p.Nombre, Descripcion: p.Descripcion, Categoria: p.Categoria,
			Etiquetas: p.Etiquetas, Precio: p.Precio, Moneda: p.Moneda, FotoURL: p.FotoURL,
			Disponible: p.Disponible, Activo: p.Activo,
			RestauranteID: p.Restaurante.ID, RestauranteNombre: p.Restaurante.Nombre,
			CocinaID: p.Sede.ID, CocinaNombre: p.Sede.Nombre,
			ActualizadoEn: p.ActualizadoEn.UTC().Format(time.RFC3339),
		})
	}
	return r.cli.Actualizar(ctx, CorePlatos, docs)
}

// ---------- Conversión ----------

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
	return domain.Sede{
		ID: d.ID, Nombre: d.Nombre, Localidad: d.Localidad, Direccion: d.Direccion,
		Ubicacion: ub, TiempoEntregaMin: d.TiempoMin, TiempoEntregaMax: d.TiempoMax,
		CostoEnvio: d.CostoEnvio, Restaurantes: rest, Activa: d.Activo,
	}, nil
}

func decodificarPlato(raw json.RawMessage) (domain.Plato, error) {
	var d docPlato
	if err := json.Unmarshal(raw, &d); err != nil {
		return domain.Plato{}, fmt.Errorf("%w: plato inválido: %v", domain.ErrNoDisponible, err)
	}
	act, _ := time.Parse(time.RFC3339, d.ActualizadoEn)
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

// frase arma un valor exacto entre comillas para un filtro (fq).
func frase(v string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(v) + `"`
}

// escaparTexto escapa los caracteres especiales de la sintaxis de Solr en el texto del usuario,
// para que se busquen como texto y no se interpreten como operadores.
func escaparTexto(s string) string {
	var b strings.Builder
	for _, ch := range s {
		if strings.ContainsRune(`\+-!():^[]"{}~*?|&/`, ch) {
			b.WriteRune('\\')
		}
		b.WriteRune(ch)
	}
	return b.String()
}
