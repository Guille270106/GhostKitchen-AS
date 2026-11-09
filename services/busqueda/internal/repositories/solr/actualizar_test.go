package solr

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"ghostkitchen/busqueda/internal/domain"
)

// Si otro escribió el plato entre la lectura y la escritura, Solr responde 409: se vuelve a
// leer y se aplica el cambio sobre lo nuevo, con el _version_ exacto (19 cifras, sin redondear).
func TestCambiarPlatoReintentaSiHayConflicto(t *testing.T) {
	versiones := []string{"1789000000000000001", "1789000000000000002"}
	lecturas, escrituras := 0, 0
	repo, sf := nuevoSolrFalso(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/solr/platos/get":
			v := versiones[min(lecturas, 1)]
			lecturas++
			_, _ = io.WriteString(w, `{"doc":{"id":"p1","nombre":"Doble","precio":9000,"activo":true,
				"version_catalogo":2,"version_inventario":5,"disponible":true,"_version_":`+v+`}}`)
		case "/solr/platos/update":
			escrituras++
			if escrituras == 1 {
				w.WriteHeader(http.StatusConflict)
				_, _ = io.WriteString(w, `{"error":{"msg":"version conflict","code":409}}`)
				return
			}
			_, _ = io.WriteString(w, `{}`)
		}
	})
	llamadas := 0
	err := repo.CambiarPlato(context.Background(), "p1", func(p domain.Plato, existe bool) (domain.Plato, bool, error) {
		llamadas++
		if !existe || p.VersionInventario != 5 || !p.PlatoActivo {
			t.Errorf("lectura: %v %+v", existe, p)
		}
		p.Precio = 9500
		return p, true, nil
	})
	if err != nil || llamadas != 2 || escrituras != 2 {
		t.Fatalf("err=%v llamadas=%d escrituras=%d", err, llamadas, escrituras)
	}
	ultimo := sf.cuerpos[len(sf.cuerpos)-1]
	if !strings.Contains(ultimo, `"_version_":1789000000000000002`) || !strings.Contains(ultimo, `"precio":9500`) {
		t.Fatalf("segunda escritura: %s", ultimo)
	}
	if q := sf.pedidos[len(sf.pedidos)-1].URL.Query(); q.Get("commitWithin") != "1000" || q.Get("commit") != "" {
		t.Fatalf("los mensajes no esperan el commit: %s", sf.pedidos[len(sf.pedidos)-1].URL)
	}
}

// Un plato que no existe se escribe con _version_ -1: si otro lo crea en el medio, hay 409.
func TestCambiarPlatoNuevoExigeQueNoExista(t *testing.T) {
	repo, sf := nuevoSolrFalso(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/solr/platos/get" {
			_, _ = io.WriteString(w, `{"doc":null}`)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	})
	err := repo.CambiarPlato(context.Background(), "p9", func(p domain.Plato, existe bool) (domain.Plato, bool, error) {
		if existe {
			t.Error("no existía")
		}
		return domain.Plato{ID: "p9", Disponible: true, VersionInventario: 1}, true, nil
	})
	if err != nil || !strings.Contains(sf.cuerpos[1], `"_version_":-1`) {
		t.Fatalf("%v %s", err, sf.cuerpos)
	}
}

// Si cambiar decide no guardar (mensaje viejo), no se escribe nada.
func TestCambiarPlatoSinCambiosNoEscribe(t *testing.T) {
	repo, sf := nuevoSolrFalso(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"doc":{"id":"p1","activo":true,"_version_":7}}`)
	})
	err := repo.CambiarPlato(context.Background(), "p1", func(p domain.Plato, _ bool) (domain.Plato, bool, error) {
		return p, false, nil
	})
	if err != nil || len(sf.pedidos) != 1 {
		t.Fatalf("%v, %d pedidos", err, len(sf.pedidos))
	}
}

func TestIDsPlatosDeSedeHaceCommitYFiltra(t *testing.T) {
	repo, sf := nuevoSolrFalso(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/select") {
			_, _ = io.WriteString(w, `{"response":{"numFound":2,"docs":[{"id":"p1"},{"id":"p2"}]}}`)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	})
	ids, err := repo.IDsPlatosDeSede(context.Background(), "nueva-cordoba")
	if err != nil || len(ids) != 2 || ids[1] != "p2" {
		t.Fatal(ids, err)
	}
	if sf.pedidos[0].URL.Query().Get("commit") != "true" || sf.pedidos[1].URL.Query().Get("fq") != `cocina_id:"nueva-cordoba"` {
		t.Fatalf("%s %s", sf.pedidos[0].URL, sf.pedidos[1].URL)
	}
}
