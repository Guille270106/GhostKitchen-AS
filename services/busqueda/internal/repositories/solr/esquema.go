package solr

import (
	"context"
	"fmt"
)

// tipoTexto es el tipo de texto para buscar: separa palabras, pasa a minúsculas y quita
// acentos, así "CAFE" encuentra "Café". Se guarda el texto original para mostrarlo.
var tipoTexto = map[string]any{
	"name":                 "text_gk",
	"class":                "solr.TextField",
	"positionIncrementGap": "100",
	"analyzer": map[string]any{
		"tokenizer": map[string]any{"class": "solr.StandardTokenizerFactory"},
		"filters": []map[string]any{
			{"class": "solr.LowerCaseFilterFactory"},
			{"class": "solr.ASCIIFoldingFilterFactory", "preserveOriginal": "false"},
		},
	},
}

// campo arma la definición de un campo para la Schema API.
func campo(nombre, tipo string, extra ...func(map[string]any)) map[string]any {
	c := map[string]any{"name": nombre, "type": tipo, "indexed": true, "stored": true}
	for _, f := range extra {
		f(c)
	}
	return c
}

func multivaluado(c map[string]any) { c["multiValued"] = true }
func soloGuardado(c map[string]any) { c["indexed"] = false; c["docValues"] = false }
func soloIndexado(c map[string]any) { c["stored"] = false }

// Campos de cada core. El campo "id" ya viene en el esquema por defecto de Solr.
var camposCocinas = []map[string]any{
	campo("nombre", "string"),
	campo("localidad", "string"),
	campo("direccion", "string"),
	campo("ubicacion", "location"),
	campo("tiempo_min", "pint"),
	campo("tiempo_max", "pint"),
	campo("costo_envio", "pint"),
	campo("restaurantes_json", "string", soloGuardado),
	campo("activo", "boolean"),
}

var camposPlatos = []map[string]any{
	campo("nombre", "text_gk"),
	campo("descripcion", "text_gk"),
	campo("categoria", "string"),
	campo("categoria_txt", "text_gk", soloIndexado),
	campo("etiquetas", "string", multivaluado),
	campo("precio", "pint"),
	campo("moneda", "string"),
	campo("foto_url", "string", soloGuardado),
	campo("disponible", "boolean"),
	campo("activo", "boolean"),
	campo("restaurante_id", "string"),
	campo("restaurante_nombre", "text_gk"),
	campo("cocina_id", "string"),
	campo("cocina_nombre", "string"),
	campo("actualizado_en", "pdate"),
}

// PrepararEsquema crea en cada core el tipo de texto y los campos que falten.
// Se puede ejecutar en cada arranque: solo agrega lo que no existe.
func (r *Repositorio) PrepararEsquema(ctx context.Context) error {
	if err := r.prepararCore(ctx, CoreCocinas, false, camposCocinas, nil); err != nil {
		return err
	}
	copias := []map[string]any{{"source": "categoria", "dest": "categoria_txt"}}
	return r.prepararCore(ctx, CorePlatos, true, camposPlatos, copias)
}

func (r *Repositorio) prepararCore(ctx context.Context, core string, conTexto bool, campos, copias []map[string]any) error {
	if conTexto {
		tipos, err := r.cli.NombresEsquema(ctx, core, "fieldtypes")
		if err != nil {
			return fmt.Errorf("leer tipos de %s: %w", core, err)
		}
		if !tipos["text_gk"] {
			if err := r.cli.Esquema(ctx, core, map[string]any{"add-field-type": tipoTexto}); err != nil {
				return fmt.Errorf("crear tipo text_gk en %s: %w", core, err)
			}
		}
	}

	existentes, err := r.cli.NombresEsquema(ctx, core, "fields")
	if err != nil {
		return fmt.Errorf("leer campos de %s: %w", core, err)
	}
	var faltan []map[string]any
	for _, c := range campos {
		if !existentes[c["name"].(string)] {
			faltan = append(faltan, c)
		}
	}
	if len(faltan) > 0 {
		if err := r.cli.Esquema(ctx, core, map[string]any{"add-field": faltan}); err != nil {
			return fmt.Errorf("crear campos en %s: %w", core, err)
		}
	}

	if len(copias) > 0 {
		hay, err := r.cli.NombresEsquema(ctx, core, "copyfields")
		if err != nil {
			return fmt.Errorf("leer copias de %s: %w", core, err)
		}
		var nuevas []map[string]any
		for _, cp := range copias {
			if !hay[fmt.Sprint(cp["source"])+"->"+fmt.Sprint(cp["dest"])] {
				nuevas = append(nuevas, cp)
			}
		}
		if len(nuevas) > 0 {
			if err := r.cli.Esquema(ctx, core, map[string]any{"add-copy-field": nuevas}); err != nil {
				return fmt.Errorf("crear copias en %s: %w", core, err)
			}
		}
	}
	return nil
}
