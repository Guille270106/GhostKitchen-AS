# Historial del contrato

Formato basado en [Keep a Changelog](https://keepachangelog.com/es-ES/1.1.0/). Versiones según [SemVer](https://semver.org/lang/es/): los cambios compatibles suben la versión menor y siguen en `/v1`; los incompatibles van a `/v2` (ver la [guía](README.md#versiones)).

## [1.0.0] — 2026-10-09

### Agregado
- Primera versión de la capacidad **Cocina más cercana y platos disponibles**.
- Operaciones:
  - `listarSedes` (`GET /v1/sedes`): sedes activas con ubicación, tiempos de entrega, costo de envío y restaurantes.
  - `buscarSedeCercana` (`GET /v1/sedes/cercana`): sede activa más cercana a una ubicación y su distancia; `404 SIN_COBERTURA` si ninguna está a menos de 25 km.
  - `buscarPlatos` (`GET /v1/sedes/{sedeId}/platos`): platos de una sede con texto, filtros, orden y paginación.
  - `obtenerPlato` (`GET /v1/platos/{platoId}`): detalle de un plato con su disponibilidad.
- Autenticación con `X-API-Key`, límite de 60 solicitudes por minuto y errores en formato Problem Details (RFC 9457) con `code` estable.
- Mock con Prism (`docker compose up contract-mock`).
