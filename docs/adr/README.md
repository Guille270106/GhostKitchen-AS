# Decisiones de arquitectura (ADR)

Un ADR es un archivo donde anotamos **una decisión**: qué elegimos, qué otras opciones pensamos y qué consecuencias aceptamos. El número del ADR es el mismo que el de la decisión del enunciado (ADR-001 = D1, …, ADR-013 = D13). Si tomamos otras decisiones importantes, las numeramos desde el ADR-014.

**Nunca borramos un ADR.** Si cambiamos de idea, el viejo queda marcado como "Reemplazado por ADR-XXX" y el nuevo explica qué cambió y por qué.

## Índice

| ADR | Decisión | Estado | Responsable | Fecha |
| --- | --- | --- | --- | --- |
| [ADR-001](ADR-001-limites-de-los-servicios.md) | D1 — Cómo separamos los servicios | Aceptada; validada en la Entrega 2 y en la final | Guillermina | 2026-10-09; validada 2026-11-10 |
| [ADR-002](ADR-002-arquitectura-interna.md) | D2 — Cómo se organiza cada servicio por dentro | Aceptada; implementada en los seis servicios | Valentina | 2026-10-16 |
| [ADR-003](ADR-003-persistencia.md) | D3 — Qué base usa cada servicio | Aceptada; validada (MongoDB y Solr en la Entrega 2, MySQL en la final) | Valentina | 2026-10-09; validada 2026-11-10 |
| [ADR-004](ADR-004-consistencia-y-concurrencia.md) | D4 — Consistencia y concurrencia | Aceptada; implementada y validada | Valentina | 2026-11-06; validada 2026-11-10 |
| [ADR-005](ADR-005-comunicacion-entre-servicios.md) | D5 — Cómo se comunican los servicios | Aceptada; validada (síncrona en la Entrega 2, mensajería y saga en la final) | Lucía | 2026-10-09; validada 2026-11-10 |
| [ADR-006](ADR-006-busqueda.md) | D6 — Búsqueda | Aceptada; actualización por mensajes implementada | Guillermina | 2026-10-16 |
| [ADR-007](ADR-007-cache.md) | D7 — Caché | Aceptada; implementada y medida | Guillermina | 2026-10-16 |
| [ADR-008](ADR-008-contrato-propio.md) | D8 — Lo que publicamos para otro grupo | Aceptada; implementada, contrato 1.1.0 sin cambios | Lucía | 2026-10-09; actualizada 2026-10-16 |
| [ADR-009](ADR-009-consumo-del-proveedor.md) | D9 — Lo que consumimos de otro grupo | Aceptada e implementada: Grupo 4 (comprobantes), factura en segundo plano; validada contra su servicio real | Lucía | 2026-11-09 |
| [ADR-010](ADR-010-resiliencia.md) | D10 — Qué pasa cuando algo falla | Aceptada; implementada y validada en la jornada de fallas | Lucía | 2026-10-16; validada 2026-11-10 |
| [ADR-011](ADR-011-observabilidad.md) | D11 — Monitoreo | Aceptada; implementada (logs, trazas, métricas, tablero y alertas) | Guillermina | 2026-10-16 |
| [ADR-012](ADR-012-balanceo-de-carga.md) | D12 — Balanceo de carga | Aceptada; implementada y validada | Guillermina | 2026-10-16 |
| [ADR-013](ADR-013-capacidad-y-costos.md) | D13 — Capacidad y costos | Aceptada; mediciones y costos completos | Valentina | 2026-10-16 |

**Estados:** Propuesta → Aceptada → Reemplazada u Obsoleta.

## Plantilla

```markdown
# ADR-XXX — Título

- **Estado:** Propuesta | Aceptada | Reemplazada por ADR-YYY | Obsoleta
- **Fecha:** AAAA-MM-DD
- **Responsable:** Nombre
- **Decisión del enunciado:** DX
- **Reemplaza a:** (si corresponde)

## Contexto
Qué problema teníamos y qué teníamos que tener en cuenta.

## Decisión
Qué elegimos, de forma concreta.

## Alternativas que pensamos
| Alternativa | A favor | En contra | Qué decidimos |

## Consecuencias
- Lo bueno
- Lo que aceptamos
- Riesgos y qué hacemos

## Cómo lo vamos a validar
Cómo vamos a comprobar que la decisión funciona.
```
