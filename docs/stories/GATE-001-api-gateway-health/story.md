# API gateway skeleton with health and readiness probes

**Code:** `GATE-001`

**As an** operator running GrindStats (and the container orchestrator and load balancer
acting on my behalf)
**I want** the gateway to answer two distinct questions — "is this process alive?" and
"should traffic route here?" — through the same middleware chain every later endpoint will
use
**So that** a dependency outage degrades the system proportionally instead of restarting
healthy containers or emptying the load balancer

## Description

The gateway is the entry point for every request in the system: it owns routing, the
middleware chain, and — once the auth stories land — JWT validation, CSRF, rate limiting and
role checks (`docs/backend.md` §2). None of that exists yet. This story builds the gateway
as a package with a real middleware chain and a single first route pair, so that the
scaffolding every later endpoint depends on (error envelope, locale rendering, request IDs,
panic recovery, route registration seam) is established once and tested, rather than
improvised by whichever story happens to add the first endpoint.

The health check is the right first API precisely because it is the smallest endpoint that
still exercises the whole chain end to end. It is also the endpoint the deployment platform
itself depends on, which makes getting its semantics right unusually consequential: a
liveness probe that fails when Postgres blips causes the orchestrator to kill and restart
otherwise-healthy tasks, and a readiness probe that fails when Redis blips pulls every task
out of the load balancer and takes the whole product down for a cache outage — the specific
outcome `SRS-AUTH-001` NFR-02 exists to prevent.

Two questions, two endpoints, and a dependency classification that distinguishes "cannot
serve" from "serving with reduced guarantees".

## In scope

- `internal/gateway/` as the single place the router, middleware chain and route
  registration seam are assembled; `cmd/server/main.go` reduces to wiring
- `GET /healthz` — liveness. Reports process health only, consults no dependency
- `GET /readyz` — readiness. Consults dependencies, each classified **required** or
  **optional**, each under its own timeout
- Dependency classification: **Postgres required** (unreachable → not ready),
  **Redis optional** (unreachable → ready but degraded, per NFR-02)
- The server starts and serves probes when a dependency is unavailable at boot, rather than
  exiting
- First implementation of the shared error envelope writer (`libs/httpkit`) and the Go
  renderer for `libs/i18n/locales/` that it needs
- New error codes `SERVICE_UNAVAILABLE` (503) and `INTERNAL_ERROR` (500) declared in
  `libs/auditmodel/model.yaml`, generated to Go and TypeScript, with messages in both locale
  catalogs
- Middleware: panic recovery, request ID, structured request logging, locale resolution
- The `/api/v1` route group created empty, and the full chain order documented as reserved
  registration points

## Out of scope

- **JWT validation, CSRF, rate limiting, role checks.** Specified by FR-20/21/22/24 and
  NFR-04 and built by the AUTH stories that own them. This story reserves their position in
  the chain and builds none of them — `docs/backend.md` §1 is explicit that building the
  target shape ahead of its roadmap phase is the most expensive mistake available here
- A separate gateway binary or any network hop between gateway and domain (Phase 10)
- Health reporting for RabbitMQ or Elasticsearch — neither is deployed yet
- Metrics, tracing or profiling endpoints
- Authentication on the probes themselves — they are consumed by infrastructure that cannot
  hold a credential
- Any domain route. `/api/v1` is created empty

## Dependencies / assumptions

- `docker-compose.yml` (Postgres, Redis) and `services/monolith/internal/platform/`
  (`config`, `db`, `cache`) exist and are verified against running containers
- `scripts/gen_audit_model.py` regenerates `libs/auditmodel/generated.go` and
  `apps/web/src/api/generated/audit.ts`; both are generated, never hand-edited
- `libs/i18n/locales/{en-US,vi-VN}.json` exist but have **no Go renderer** — this story
  builds it, as the envelope writer cannot render a message without one
- `SRS-AUTH-001` NFR-02 governs the required/optional split, and NFR-01's ≤5 ms gateway
  budget constrains what the chain may do per request
- An error code needs no audit event: `model.yaml` declares the two sections independently,
  and a probe from a load balancer is not an actor performing an auditable action
- `AUTH-002` will be the first story to register routes under `/api/v1`, and the first real
  consumer of the seam this story establishes

## Open questions

- **Is `/readyz` reachable from the public internet in production?** In AWS the ALB probes
  from inside the VPC, but a probe on the same listener as the product API is publicly
  reachable unless a listener rule blocks it. Deferred to the deployment phase; this story
  keeps the response body free of anything worth leaking so the answer cannot become a
  security dependency.
- **Should the response distinguish `degraded` from `ready` at all**, given an unauthenticated
  caller? Currently yes, because an operator needs to see partial failure without shell
  access — but the body names no dependency, and which one is degraded appears only in the
  structured log.
- **How long may a task remain unready at startup before the orchestrator gives up?** An ECS
  health-check grace-period setting, not application behavior. Deferred to the deployment
  phase.
