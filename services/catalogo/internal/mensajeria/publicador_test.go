package mensajeria

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"ghostkitchen/catalogo/internal/domain"
	"ghostkitchen/catalogo/internal/repositories/memoria"
)

// canalFalso registra lo publicado. Falla las primeras fallas veces.
type canalFalso struct {
	mu         sync.Mutex
	publicados []string
	fallas     int
	cerrados   int
}

func (c *canalFalso) Publicar(_ context.Context, p domain.Pendiente) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fallas > 0 {
		c.fallas--
		return errors.New("broker caído")
	}
	c.publicados = append(c.publicados, p.Tipo+":"+p.EventoID)
	return nil
}
func (c *canalFalso) Cerrar() { c.mu.Lock(); c.cerrados++; c.mu.Unlock() }

func repoConEventos(t *testing.T, n int) (*memoria.Repositorio, []domain.Evento) {
	t.Helper()
	repo := memoria.Nuevo()
	c := domain.Cocina{ID: "nc", Nombre: "NC", Version: 1}
	var evs []domain.Evento
	for i := 0; i < n; i++ {
		ev := domain.EventoCocina(domain.CocinaActualizada, c, "", time.Now())
		if i == 0 {
			ev = domain.EventoCocina(domain.CocinaCreada, c, "", time.Now())
			_ = repo.CrearCocina(context.Background(), c, ev)
		} else {
			anterior := c.Version
			c.Version++
			_ = repo.ReemplazarCocina(context.Background(), c, anterior, ev)
		}
		evs = append(evs, ev)
	}
	return repo, evs
}

func correr(t *testing.T, p *Publicador, hasta func() bool) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	listo := make(chan struct{})
	go func() { p.Correr(ctx); close(listo) }()
	limite := time.After(3 * time.Second)
	for !hasta() {
		select {
		case <-limite:
			cancel()
			<-listo
			t.Fatal("no terminó a tiempo")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	<-listo
}

func nuevo(repo *memoria.Repositorio, canal *canalFalso, fallasConexion int) *Publicador {
	var mu sync.Mutex
	p := NuevoPublicador(repo, func() (Canal, error) {
		mu.Lock()
		defer mu.Unlock()
		if fallasConexion > 0 {
			fallasConexion--
			return nil, errors.New("sin conexión")
		}
		return canal, nil
	}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	p.Intervalo, p.EsperaMax = 5*time.Millisecond, 20*time.Millisecond
	return p
}

func pendientes(repo *memoria.Repositorio) int {
	p, _ := repo.Pendientes(context.Background(), 1000)
	return len(p)
}

func TestPublicaEnOrdenYMarca(t *testing.T) {
	repo, evs := repoConEventos(t, 3)
	canal := &canalFalso{}
	correr(t, nuevo(repo, canal, 0), func() bool { return pendientes(repo) == 0 })
	if len(canal.publicados) != 3 {
		t.Fatalf("publicados: %v", canal.publicados)
	}
	for i, ev := range evs {
		if canal.publicados[i] != ev.Tipo+":"+ev.EventoID {
			t.Fatalf("orden: %v", canal.publicados)
		}
	}
}

// Con RabbitMQ caído no se pierde nada: los eventos esperan y salen cuando vuelve, en orden.
func TestRabbitCaidoLosEventosEsperan(t *testing.T) {
	repo, evs := repoConEventos(t, 3)
	canal := &canalFalso{fallas: 2}
	correr(t, nuevo(repo, canal, 2), func() bool { return pendientes(repo) == 0 })
	if len(canal.publicados) != 3 || canal.publicados[0] != evs[0].Tipo+":"+evs[0].EventoID {
		t.Fatalf("publicados: %v", canal.publicados)
	}
	if canal.cerrados < 2 {
		t.Fatalf("después de cada falla se cierra el canal y se reconecta: %d cierres", canal.cerrados)
	}
}

// Un evento que no se pudo publicar no se marca: sigue pendiente.
func TestSinConfirmacionNoSeMarca(t *testing.T) {
	repo, _ := repoConEventos(t, 1)
	canal := &canalFalso{fallas: 1000}
	p := nuevo(repo, canal, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	p.Correr(ctx)
	if pendientes(repo) != 1 {
		t.Fatal("sin confirmación del broker, el evento sigue pendiente")
	}
}
