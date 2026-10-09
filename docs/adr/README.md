# Decisiones de arquitectura (ADR)

Un ADR es un archivo donde anotamos **una decisión**: qué elegimos, qué otras opciones pensamos y qué consecuencias aceptamos. El número del ADR es el mismo que el de la decisión del enunciado (ADR-001 = D1, …, ADR-013 = D13). Si tomamos otras decisiones importantes, las numeramos desde el ADR-014.

**Nunca borramos un ADR.** Si cambiamos de idea, el viejo queda marcado como "Reemplazado por ADR-XXX" y el nuevo explica qué cambió y por qué.

## Índice

| ADR | Decisión | Estado | Fecha |
| --- | --- | --- | --- |
| [ADR-001](ADR-001-limites-de-los-servicios.md) | D1 — Cómo separamos los servicios | Aceptada | 2026-10-09 |
| ADR-002 | D2 — Cómo se organiza cada servicio por dentro | Pendiente (Entrega 2) | — |
| [ADR-003](ADR-003-persistencia.md) | D3 — Qué base usa cada servicio | Primera versión | 2026-10-09 |
| ADR-004 | D4 — Consistencia y concurrencia | Pendiente; definir antes de implementar el flujo crítico y demostrar en la defensa individual | — |
| [ADR-005](ADR-005-comunicacion-entre-servicios.md) | D5 — Cómo se comunican los servicios | Primera versión | 2026-10-09 |
| ADR-006 | D6 — Búsqueda | Pendiente (Entrega 2) | — |
| ADR-007 | D7 — Caché | Pendiente (Entrega 2) | — |
| [ADR-008](ADR-008-contrato-propio.md) | D8 — Lo que publicamos para otro grupo | Aceptada | 2026-10-09 |
| ADR-009 | D9 — Lo que consumimos de otro grupo | Pendiente (Entrega 2) | — |
| ADR-010 | D10 — Qué pasa cuando algo falla | Pendiente (Entrega 2) | — |
| ADR-011 | D11 — Monitoreo | Pendiente (Entrega 2) | — |
| ADR-012 | D12 — Balanceo de carga | Pendiente (Entrega 2) | — |
| ADR-013 | D13 — Capacidad y costos | Pendiente (Entrega 2) | — |

**Estados:** Propuesta → Aceptada → Reemplazada u Obsoleta. "Primera versión" quiere decir que el enunciado pide un borrador en esta entrega y la versión validada en la siguiente.

## Plantilla

```markdown
# ADR-XXX — Título

- **Estado:** Propuesta | Aceptada | Reemplazada por ADR-YYY | Obsoleta
- **Fecha:** AAAA-MM-DD
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
