package solr

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"ghostkitchen/busqueda/internal/domain"
)

// solrFalso responde como Solr a las rutas que usa el repositorio y guarda lo que recibió.
type solrFalso struct {
	mu        sync.Mutex
	pedidos   []*http.Request
	cuerpos   []string
	responder func(w http.ResponseWriter, r *http.Request)
}

func nuevoSolrFalso(t *testing.T, f func(w http.ResponseWriter, r *http.Request)) (*Repositorio, *solrFalso) {
	t.Helper()
	sf := &solrFalso{responder: f}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		sf.mu.Lock()
		sf.pedidos = append(sf.pedidos, r)
		sf.cuerpos = append(sf.cuerpos, string(b))
		sf.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		sf.responder(w, r)
	}))
	t.Cleanup(srv.Close)
	return NuevoRepositorio(NuevoCliente(srv.URL+"/solr", time.Second)), sf
}

const docCerro = `{"id":"cerro","nombre":"Cocina Cerro","localidad":"Cerro de las Rosas","direccion":"Rafael Núñez 4520",
"ubicacion":"-31.3682,-64.2318","tiempo_min":30,"tiempo_max":40,"costo_envio":1300,
"restaurantes_json":"[{\"id\":\"r5\",\"nombre\":\"Johnny B. Good\"},{\"id\":\"r6\",\"nombre\":\"Il Gatto\"}]","activo":true,"_version_":1}`

func TestSedeMasCercanaArmaLaConsultaGeoespacial(t *testing.T) {
	repo, sf := nuevoSolrFalso(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"response":{"numFound":4,"docs":[`+docCerro+`]}}`)
	})
	sede, ok, err := repo.SedeMasCercana(context.Background(), domain.Ubicacion{Lat: -31.333, Lng: -64.255})
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	q := sf.pedidos[0].URL.Query()
	if sf.pedidos[0].URL.Path != "/solr/cocinas/select" || q.Get("sfield") != "ubicacion" ||
		q.Get("pt") != "-31.333,-64.255" || q.Get("sort") != "geodist() asc,id asc" || q.Get("fq") != "activo:true" {
		t.Fatalf("consulta inesperada: %s", sf.pedidos[0].URL)
	}
	if sede.ID != "cerro" || sede.Ubicacion.Lat != -31.3682 || len(sede.Restaurantes) != 2 || sede.Restaurantes[1].Nombre != "Il Gatto" {
		t.Fatalf("%+v", sede)
	}
}

func TestSinSedesActivas(t *testing.T) {
	repo, _ := nuevoSolrFalso(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"response":{"numFound":0,"docs":[]}}`)
	})
	_, ok, err := repo.SedeMasCercana(context.Background(), domain.Ubicacion{})
	if err != nil || ok {
		t.Fatal(ok, err)
	}
}

func TestObtenerPlatoInexistenteODesactivado(t *testing.T) {
	repo, _ := nuevoSolrFalso(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id") == "p64" {
			_, _ = io.WriteString(w, `{"doc":{"id":"p64","nombre":"Licuado","activo":false,"actualizado_en":"2026-10-16T12:00:00Z"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"doc":null}`)
	})
	for _, id := range []string{"p999", "p64"} {
		if _, ok, err := repo.ObtenerPlato(context.Background(), id); ok || err != nil {
			t.Fatalf("%s: ok=%v err=%v", id, ok, err)
		}
	}
}

func TestErrorDeSolrEsNoDisponible(t *testing.T) {
	repo, _ := nuevoSolrFalso(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = io.WriteString(w, `{"error":{"msg":"se rompió"}}`)
	})
	if _, err := repo.ListarSedes(context.Background()); !errors.Is(err, domain.ErrNoDisponible) {
		t.Fatalf("esperaba ErrNoDisponible, obtuve %v", err)
	}
}

func TestSolrApagadoEsNoDisponible(t *testing.T) {
	repo := NuevoRepositorio(NuevoCliente("http://127.0.0.1:1/solr", 200*time.Millisecond))
	if err := repo.Ping(context.Background()); !errors.Is(err, domain.ErrNoDisponible) {
		t.Fatalf("esperaba ErrNoDisponible, obtuve %v", err)
	}
}

func TestParametrosBusqueda(t *testing.T) {
	p := ParametrosBusqueda(domain.FiltroPlatos{
		SedeID: "nueva-cordoba", Texto: `burger (doble)`, Categoria: "Hamburguesas", Etiqueta: "Sin TACC",
		RestauranteID: "r1", PrecioMax: 15000, SoloDisponibles: true, Orden: domain.OrdenPrecioDesc,
		Pagina: 3, TamPagina: 10,
	})
	if p.Get("defType") != "edismax" || p.Get("q") != `burger \(doble\)` || p.Get("mm") != "100%" {
		t.Fatalf("texto mal armado: %v", p)
	}
	fq := strings.Join(p["fq"], " | ")
	for _, esperado := range []string{`activo:true`, `cocina_id:"nueva-cordoba"`, `disponible:true`,
		`categoria:"Hamburguesas"`, `etiquetas:"Sin TACC"`, `restaurante_id:"r1"`, `precio:[* TO 15000]`} {
		if !strings.Contains(fq, esperado) {
			t.Errorf("falta el filtro %s en %s", esperado, fq)
		}
	}
	if p.Get("sort") != "precio desc,id asc" || p.Get("start") != "20" || p.Get("rows") != "10" {
		t.Fatalf("orden o página mal armados: %v", p)
	}
}

func TestParametrosBusquedaSinTextoNiFiltros(t *testing.T) {
	p := ParametrosBusqueda(domain.FiltroPlatos{SedeID: "cerro", Pagina: 1, TamPagina: 20})
	if p.Get("q") != "*:*" || p.Get("defType") != "" || p.Get("sort") != "score desc,id asc" || len(p["fq"]) != 2 {
		t.Fatalf("%v", p)
	}
}

func TestFiltroEscapaComillas(t *testing.T) {
	p := ParametrosBusqueda(domain.FiltroPlatos{SedeID: "x", Categoria: `a"b\c`, Pagina: 1, TamPagina: 1})
	if !strings.Contains(strings.Join(p["fq"], " "), `categoria:"a\"b\\c"`) {
		t.Fatalf("%v", p["fq"])
	}
}

func TestPrepararEsquemaSoloAgregaLoQueFalta(t *testing.T) {
	repo, sf := nuevoSolrFalso(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/schema/fieldtypes"):
			_, _ = io.WriteString(w, `{"fieldTypes":[{"name":"string"},{"name":"text_gk"}]}`)
		case strings.HasSuffix(r.URL.Path, "/schema/fields"):
			_, _ = io.WriteString(w, `{"fields":[{"name":"id"},{"name":"nombre"},{"name":"activo"}]}`)
		case strings.HasSuffix(r.URL.Path, "/schema/copyfields"):
			_, _ = io.WriteString(w, `{"copyFields":[{"source":"categoria","dest":"categoria_txt"}]}`)
		default:
			_, _ = io.WriteString(w, `{"responseHeader":{"status":0}}`)
		}
	})
	if err := repo.PrepararEsquema(context.Background()); err != nil {
		t.Fatal(err)
	}
	var posts []string
	for i, r := range sf.pedidos {
		if r.Method == http.MethodPost {
			posts = append(posts, r.URL.Path+" "+sf.cuerpos[i])
		}
	}
	// Un POST por core con los campos faltantes; no crea text_gk ni la copia porque ya existen.
	if len(posts) != 2 {
		t.Fatalf("esperaba 2 POST, hubo %d: %v", len(posts), posts)
	}
	for _, p := range posts {
		if strings.Contains(p, `"name":"nombre"`) || strings.Contains(p, "add-field-type") || strings.Contains(p, "add-copy-field") {
			t.Fatalf("intentó crear algo que ya existía: %s", p)
		}
	}
	var cuerpo map[string][]map[string]any
	_ = json.Unmarshal([]byte(strings.SplitN(posts[1], " ", 2)[1]), &cuerpo)
	if len(cuerpo["add-field"]) != len(camposPlatos)-2 { // nombre y activo ya existían
		t.Fatalf("campos agregados en platos: %d", len(cuerpo["add-field"]))
	}
}

func TestGuardarPlatosArmaLosDocumentos(t *testing.T) {
	repo, sf := nuevoSolrFalso(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"responseHeader":{"status":0}}`)
	})
	err := repo.GuardarPlatos(context.Background(), []domain.Plato{{
		ID: "p1", Nombre: "Mega doble", Precio: 12900, Moneda: "ARS", Activo: true, Disponible: true,
		Restaurante: domain.Referencia{ID: "r1", Nombre: "Mostaza"}, Sede: domain.Referencia{ID: "nueva-cordoba", Nombre: "Cocina Nueva Córdoba"},
		ActualizadoEn: time.Date(2026, 10, 16, 15, 0, 0, 0, time.UTC), Etiquetas: []string{},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if sf.pedidos[0].URL.Path != "/solr/platos/update" || sf.pedidos[0].URL.Query().Get("commit") != "true" {
		t.Fatalf("%s", sf.pedidos[0].URL)
	}
	if !strings.Contains(sf.cuerpos[0], `"cocina_id":"nueva-cordoba"`) || !strings.Contains(sf.cuerpos[0], `"actualizado_en":"2026-10-16T15:00:00Z"`) {
		t.Fatalf("%s", sf.cuerpos[0])
	}
}
