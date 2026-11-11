package mysql_test

import (
	"context"
	"errors"
	"testing"
	"time"

	adapter "ghostkitchen/pedidos/internal/adapters/mysql"
	"ghostkitchen/pedidos/internal/domain"
	"ghostkitchen/pedidos/internal/ports"
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

// reserva.vencida contra MySQL real: vacía el carrito que tiene esa reserva y lo marca; el mismo
// aviso otra vez no hace nada; el próximo cambio del cliente borra la marca.
func TestMySQLMarcarVencido(t *testing.T) {
	db := conectar(t)
	ctx := context.Background()
	repo := adapter.CarritosMySQL{DB: db}

	// 1. Un carrito con un plato y su reserva.
	cliente := "u-" + uuid.NewString()[:8]
	vence := time.Now().Add(time.Minute)
	c := domain.Carrito{ID: uuid.NewString(), ClienteID: cliente, ReservaID: "res-" + uuid.NewString()[:8], VenceEn: &vence, Version: 1}
	_ = c.Agregar(domain.PlatoCatalogo{ID: "p17", CocinaID: "cerro", RestauranteID: "r5", Nombre: "Burger", Precio: 12000, Activo: true}, 1)
	if err := repo.Guardar(ctx, c, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM carrito_item WHERE cliente_id=?", cliente)
		db.Exec("DELETE FROM carrito WHERE cliente_id=?", cliente)
	})

	// 2. Un aviso de otra reserva no lo toca.
	if cambio, err := repo.MarcarVencido(ctx, c.ID, "res-otra"); err != nil || cambio {
		t.Fatal("otra reserva no tiene que tocarlo:", cambio, err)
	}

	// 3. El aviso de su reserva lo vacía y lo marca; repetido, no hace nada.
	if cambio, err := repo.MarcarVencido(ctx, c.ID, c.ReservaID); err != nil || !cambio {
		t.Fatal(cambio, err)
	}
	if cambio, _ := repo.MarcarVencido(ctx, c.ID, c.ReservaID); cambio {
		t.Fatal("el mismo aviso dos veces no cambia nada")
	}
	leido, err := repo.Obtener(ctx, cliente)
	if err != nil || len(leido.Items) != 0 || leido.ReservaID != "" || leido.VencioEn == nil || leido.ID == c.ID || leido.Version != 2 {
		t.Fatalf("%v %+v", err, leido)
	}

	// 4. El cliente vuelve a cambiar el carrito: la marca se borra.
	leido.VencioEn = nil
	leido.Version = 3
	if err := repo.Guardar(ctx, leido, 2); err != nil {
		t.Fatal(err)
	}
	if otra, _ := repo.Obtener(ctx, cliente); otra.VencioEn != nil {
		t.Fatal("después de un cambio ya no tiene que figurar como vencido")
	}
}

// La Idempotency-Key de "agregar" se guarda en la misma transacción que el carrito: si vuelve a
// llegar, no se guarda nada y el carrito queda como estaba.
func TestMySQLLaMismaClaveNoSeAplicaDosVeces(t *testing.T) {
	db := conectar(t)
	repo := adapter.CarritosMySQL{DB: db}
	ctx := context.Background()
	cliente := "u-" + uuid.NewString()[:8]
	t.Cleanup(func() {
		db.Exec("DELETE FROM carrito_operacion WHERE cliente_id=?", cliente)
		db.Exec("DELETE FROM carrito_item WHERE cliente_id=?", cliente)
		db.Exec("DELETE FROM carrito WHERE cliente_id=?", cliente)
	})

	c, _ := repo.Obtener(ctx, cliente)
	c.ID = uuid.NewString()
	_ = c.Agregar(domain.PlatoCatalogo{ID: "p17", CocinaID: "cerro", RestauranteID: "r5", Nombre: "Burger", Precio: 12000, Activo: true}, 1)
	c.Version, c.Operacion = 1, "clic-1"
	if err := repo.Guardar(ctx, c, 0); err != nil {
		t.Fatal(err)
	}
	if aplicada, err := repo.OperacionAplicada(ctx, cliente, "clic-1"); err != nil || !aplicada {
		t.Fatal("la clave no quedó anotada:", aplicada, err)
	}

	// El reenvío llega con el carrito ya sumado otra vez: no se tiene que guardar.
	c.Items[0].Cantidad = 2
	c.Version = 2
	if err := repo.Guardar(ctx, c, 1); !errors.Is(err, domain.ErrOperacionRepetida) {
		t.Fatal("tenía que avisar que la clave ya se aplicó:", err)
	}
	leido, _ := repo.Obtener(ctx, cliente)
	if leido.Version != 1 || leido.Items[0].Cantidad != 1 {
		t.Fatal("el reenvío cambió el carrito:", leido)
	}
	if aplicada, _ := repo.OperacionAplicada(ctx, cliente, "otra"); aplicada {
		t.Fatal("una clave nueva no puede figurar como aplicada")
	}
}

func TestMySQLDiarioCarritoEsAtomicoYComparteBloqueo(t *testing.T) {
	db := conectar(t)
	otraDB := conectar(t)
	repo := adapter.CarritosMySQL{DB: db}
	otraInstancia := adapter.CarritosMySQL{DB: otraDB}
	ctx := context.Background()
	cliente := "u-" + uuid.NewString()
	t.Cleanup(func() {
		db.Exec("DELETE FROM carrito_item WHERE cliente_id=?", cliente)
		db.Exec("DELETE FROM carrito WHERE cliente_id=?", cliente)
		db.Exec("DELETE FROM carrito_operacion WHERE cliente_id=?", cliente)
	})
	c := domain.Carrito{ID: uuid.NewString(), ClienteID: cliente, Version: 1, Operacion: "clic"}
	op := ports.CambioCarrito{ID: uuid.NewString(), Estado: ports.CambioIncierto, Destino: c}
	c.CambioID = op.ID

	err := repo.ConBloqueo(ctx, cliente, func(ctx context.Context) error {
		corto, cancelar := context.WithTimeout(ctx, 200*time.Millisecond)
		defer cancelar()
		if err := otraInstancia.ConBloqueo(corto, cliente, func(context.Context) error {
			t.Error("dos conexiones tomaron el mismo bloqueo")
			return nil
		}); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("la segunda conexión debía esperar el bloqueo: %v", err)
		}
		if err := repo.IniciarCambio(ctx, cliente, op); err != nil {
			return err
		}
		pendiente, err := otraInstancia.CambioPendiente(context.Background(), cliente)
		if err != nil || pendiente == nil || pendiente.ID != op.ID || pendiente.Destino.ClienteID != cliente {
			t.Fatalf("operación sin persistir: %+v, %v", pendiente, err)
		}
		if err := repo.Guardar(ctx, c, 7); !errors.Is(err, domain.ErrCarritoModificado) {
			t.Fatalf("guardar con otra versión debía fallar: %v", err)
		}
		if pendiente, err := repo.CambioPendiente(ctx, cliente); err != nil || pendiente == nil {
			t.Fatalf("el fallo de guardado perdió la operación: %+v, %v", pendiente, err)
		}
		if aplicado, err := repo.OperacionAplicada(ctx, cliente, "clic"); err != nil || aplicado {
			t.Fatalf("el guardado fallido consumió la clave del cliente: %v, %v", aplicado, err)
		}
		return repo.Guardar(ctx, c, 0)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := otraInstancia.ConBloqueo(ctx, cliente, func(ctx context.Context) error {
		if pendiente, err := otraInstancia.CambioPendiente(ctx, cliente); err != nil || pendiente != nil {
			t.Fatalf("se confirmó el carrito sin cerrar el cambio: %+v, %v", pendiente, err)
		}
		if aplicado, err := otraInstancia.OperacionAplicada(ctx, cliente, "clic"); err != nil || !aplicado {
			t.Fatalf("faltó la clave del cliente en el commit: %v, %v", aplicado, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
