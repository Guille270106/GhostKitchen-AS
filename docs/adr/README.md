# Decisiones de arquitectura (ADR)

Un ADR es un archivo donde anotamos **una decisión**: qué elegimos, qué otras opciones pensamos y qué consecuencias aceptamos. El número del ADR es el mismo que el de la decisión del enunciado (ADR-001 = D1, …, ADR-013 = D13). Si tomamos otras decisiones importantes, las numeramos desde el ADR-014.

**Nunca borramos un ADR.** Si cambiamos de idea, el viejo queda marcado como "Reemplazado por ADR-XXX" y el nuevo explica qué cambió y por qué.

## Índice

| ADR | Decisión | Estado | Responsable | Fecha |
| --- | --- | --- | --- | --- |
| [ADR-001](ADR-001-limites-de-los-servicios.md) | D1 — Cómo separamos los servicios | Aceptada; validada en la Entrega 2 | Guillermina | 2026-10-09; validada 2026-10-16 |
| [ADR-002](ADR-002-arquitectura-interna.md) | D2 — Cómo se organiza cada servicio por dentro | Aceptada para Búsqueda y Catálogo; propuesta para los demás | Valentina | 2026-10-16 |
| [ADR-003](ADR-003-persistencia.md) | D3 — Qué base usa cada servicio | Aceptada; validada para MongoDB y Solr, MySQL pendiente | Valentina | 2026-10-09; validada 2026-10-16 |
| ADR-004 | D4 — Consistencia y concurrencia | Pendiente; definir antes de implementar el flujo crítico y demostrar en la defensa individual | — | — |
| [ADR-005](ADR-005-comunicacion-entre-servicios.md) | D5 — Cómo se comunican los servicios | Aceptada; validada la parte síncrona | Lucía | 2026-10-09; validada 2026-10-16 |
| [ADR-006](ADR-006-busqueda.md) | D6 — Búsqueda | Aceptada | Guillermina | 2026-10-16 |
| [ADR-007](ADR-007-cache.md) | D7 — Caché | Propuesta | Guillermina | 2026-10-16 |
| [ADR-008](ADR-008-contrato-propio.md) | D8 — Lo que publicamos para otro grupo | Aceptada; implementada, contrato 1.1.0 sin cambios | Lucía | 2026-10-09; actualizada 2026-10-16 |
| [ADR-009](ADR-009-consumo-del-proveedor.md) | D9 — Lo que consumimos de otro grupo | Propuesta (falta que la cátedra asigne el proveedor) | Lucía | 2026-10-16 |
| [ADR-010](ADR-010-resiliencia.md) | D10 — Qué pasa cuando algo falla | Aceptada | Lucía | 2026-10-16 |
| [ADR-011](ADR-011-observabilidad.md) | D11 — Monitoreo | Primera versión | Guillermina | 2026-10-16 |
| [ADR-012](ADR-012-balanceo-de-carga.md) | D12 — Balanceo de carga | Propuesta | Guillermina | 2026-10-16 |
| [ADR-013](ADR-013-capacidad-y-costos.md) | D13 — Capacidad y costos | Primera versión | Valentina | 2026-10-16 |

**Estados:** Propuesta → Aceptada → Reemplazada u Obsoleta. "Primera versión" quiere decir que el enunciado pide un borrador en esta entrega y la versión validada en la siguiente.

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
