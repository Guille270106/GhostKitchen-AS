package mysql_test

import (
	"context"
	"errors"
	"testing"
	"time"

	adapter "ghostkitchen/pedidos/internal/adapters/mysql"
	"ghostkitchen/pedidos/internal/domain"
	"github.com/google/uuid"
)

func TestMySQLCarritoGuardarYLeer(t *testing.T) {
	db := conectar(t)
	repo := adapter.CarritosMySQL{DB: db}
	ctx := context.Background()
	cliente := "u-" + uuid.NewString()[:8]
	t.Cleanup(func() {
		db.Exec("DELETE FROM carrito_item WHERE cliente_id=?", cliente)
		db.Exec("DELETE FROM carrito WHERE cliente_id=?", cliente)
	})

	// 1. Un cliente sin carrito recibe uno vacío con versión 0.
	c, err := repo.Obtener(ctx, cliente)
	if err != nil || c.Version != 0 || !c.Vacio() {
		t.Fatal(err, c)
	}

	// 2. Guardamos un carrito con dos platos y su reserva.
	c.ID = uuid.NewString()
	_ = c.Agregar(domain.PlatoCatalogo{ID: "p17", CocinaID: "cerro", RestauranteID: "r5", Nombre: "Burger ' doble", Precio: 12000, Activo: true}, 2)
	_ = c.Agregar(domain.PlatoCatalogo{ID: "p21", CocinaID: "cerro", RestauranteID: "r6", Nombre: "Pizza", Precio: 9500, Activo: true}, 1)
	vence := time.Now().Add(10 * time.Minute).UTC().Truncate(time.Millisecond)
	c.ReservaID, c.VenceEn, c.Version = "res-1", &vence, 1
	if err := repo.Guardar(ctx, c, 0); err != nil {
		t.Fatal(err)
	}

	leido, err := repo.Obtener(ctx, cliente)
	if err != nil || leido.Version != 1 || len(leido.Items) != 2 || leido.Total != 33500 ||
		leido.ReservaID != "res-1" || leido.CocinaID != "cerro" || !leido.VenceEn.Equal(vence) || leido.Items[0].PlatoID != "p17" {
		t.Fatal(err, leido)
	}

	// 3. Guardar con una versión vieja no pisa nada.
	leido.Version = 2
	if err := repo.Guardar(ctx, leido, 0); !errors.Is(err, domain.ErrCarritoModificado) {
		t.Fatal("crear de nuevo el mismo carrito:", err)
	}
	if err := repo.Guardar(ctx, leido, 5); !errors.Is(err, domain.ErrCarritoModificado) {
		t.Fatal("versión vieja:", err)
	}

	// 4. Vaciar: queda sin platos, sin reserva y con otro id.
	leido.Reiniciar(uuid.NewString())
	if err := repo.Guardar(ctx, leido, 1); err != nil {
		t.Fatal(err)
	}
	vacio, _ := repo.Obtener(ctx, cliente)
	if !vacio.Vacio() || vacio.ReservaID != "" || vacio.VenceEn != nil || vacio.ID == c.ID || vacio.Version != 2 {
		t.Fatal(vacio)
	}
}
