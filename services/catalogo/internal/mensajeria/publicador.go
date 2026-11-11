package mensajeria

import (
	"context"
	"log/slog"
	"time"

	"ghostkitchen/catalogo/internal/domain"
	"ghostkitchen/catalogo/internal/repositories"
)

// Canal es lo que el publicador necesita del broker. La implementación real es RabbitMQ; los
// tests usan una falsa.
type Canal interface {
	Publicar(ctx context.Context, p domain.Pendiente) error
	Cerrar()
}

// Publicador manda los eventos del outbox a RabbitMQ.
type Publicador struct {
	outbox    repositories.Outbox
	conectar  func() (Canal, error)
	log       *slog.Logger
	Intervalo time.Duration // cada cuánto busca pendientes cuando no hay (500 ms)
	Lote      int           // cuántos pendientes lee por vuelta (100)
	EsperaMax time.Duration // espera máxima entre reintentos si RabbitMQ falla (30 s)
}

// NuevoPublicador crea el publicador. conectar abre un canal nuevo cada vez que hace falta.
func NuevoPublicador(outbox repositories.Outbox, conectar func() (Canal, error), log *slog.Logger) *Publicador {
	return &Publicador{outbox: outbox, conectar: conectar, log: log, Intervalo: 500 * time.Millisecond, Lote: 100, EsperaMax: 30 * time.Second}
}

// Correr publica hasta que se cancele ctx. Busca pendientes cada Intervalo (con 500 ms, un
// cambio llega a Búsqueda bastante antes de los 5 s aceptados).
//
// Si RabbitMQ falla, cierra el canal y espera cada vez más (1 s, 2 s, 4 s… hasta EsperaMax)
// antes de reconectar, para no insistir sobre un broker caído. Los eventos se mandan en el
// orden en que se guardaron y se corta en el primero que falla, así un cambio no se adelanta
// a otro anterior de la misma cocina o plato.
func (p *Publicador) Correr(ctx context.Context) {
	var canal Canal
	espera := time.Second
	defer func() {
		if canal != nil {
			canal.Cerrar()
		}
	}()
	for {
		enviados, err := p.vuelta(ctx, &canal)
		if enviados > 0 {
			p.log.Info("eventos publicados en RabbitMQ", "cantidad", enviados)
		}
		pausa := p.Intervalo
		if err != nil {
			p.log.Warn("no se pudo publicar en RabbitMQ; los eventos esperan en el outbox", "motivo", err.Error(), "reintento_en_s", espera.Seconds())
			if canal != nil {
				canal.Cerrar()
				canal = nil
			}
			pausa = espera
			espera = min(espera*2, p.EsperaMax)
		} else {
			espera = time.Second
		}
		if enviados == p.Lote && err == nil {
			pausa = 0 // había más: seguir sin esperar
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(pausa):
		}
	}
}

// vuelta publica un lote de pendientes y devuelve cuántos salieron.
func (p *Publicador) vuelta(ctx context.Context, canal *Canal) (int, error) {
	pendientes, err := p.outbox.Pendientes(ctx, p.Lote)
	if err != nil || len(pendientes) == 0 {
		return 0, err
	}
	if *canal == nil {
		c, err := p.conectar()
		if err != nil {
			return 0, err
		}
		*canal = c
	}
	for i, ev := range pendientes {
		if err := (*canal).Publicar(ctx, ev); err != nil {
			return i, err
		}
		// Si marcarlo falla, el evento se vuelve a publicar en la próxima vuelta. No es un
		// problema: el consumidor descarta el evento_id repetido (entrega "al menos una vez").
		if err := p.outbox.MarcarPublicado(ctx, ev.EventoID, time.Now()); err != nil {
			return i + 1, err
		}
	}
	return len(pendientes), nil
}
