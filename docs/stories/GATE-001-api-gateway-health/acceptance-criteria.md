# Acceptance criteria: API gateway skeleton with health and readiness probes

**Story:** [`GATE-001`](story.md)

## Acceptance criteria (summary)

- [ ] `GET /healthz` returns 200 whenever the process is running, without consulting any
      dependency
- [ ] `GET /readyz` returns 200 `ready` when Postgres and Redis are both reachable
- [ ] `GET /readyz` returns 200 `degraded` when Redis is unreachable but Postgres is
      reachable (NFR-02)
- [ ] `GET /readyz` returns 503 with the standard error envelope when Postgres is
      unreachable
- [ ] The 503 body carries error code `SERVICE_UNAVAILABLE` from the generated registry —
      no hand-written code string appears anywhere in the gateway
- [ ] The 503 `message` is rendered from `libs/i18n/locales/` in the request's
      `Accept-Language`, in both `en-US` and `vi-VN`
- [ ] No response body names which dependency failed; the structured log does
- [ ] Each dependency check is bounded by its own timeout, so a hung dependency cannot hang
      the probe
- [ ] The server starts and serves probes when a dependency is unavailable at boot, instead
      of exiting
- [ ] A panic in any handler returns 500 `INTERNAL_ERROR` in the envelope rather than
      dropping the connection
- [ ] Every response carries a request ID — echoed from `X-Request-ID` when supplied,
      generated when not
- [ ] Both probes are reachable without credentials and live outside `/api/v1`

## Acceptance criteria (scenarios)

### Scenario: liveness answers while every dependency is unreachable

**Given** the server process is running
**And** neither Postgres nor Redis is reachable
**When** the orchestrator sends `GET /healthz`
**Then** the response status is 200
**And** the body reports the process as alive
**And** no dependency check is invoked

> This is the scenario the endpoint exists for. A liveness probe that consults dependencies
> converts a brief Postgres outage into a rolling container restart, because the orchestrator
> reacts to a failed liveness probe by killing the task.

### Scenario: readiness reports ready when all dependencies are healthy

**Given** Postgres is reachable
**And** Redis is reachable
**When** the load balancer sends `GET /readyz`
**Then** the response status is 200
**And** the body reports status `ready`

### Scenario: readiness stays ready when only the optional dependency is unreachable

**Given** Postgres is reachable
**And** Redis is unreachable
**When** the load balancer sends `GET /readyz`
**Then** the response status is 200
**And** the body reports status `degraded`
**And** a warning naming Redis is written to the structured log

> NFR-02: the gateway fails open on read-only GETs when Redis is down, so the instance can
> still serve. Returning 503 here would pull every task from the load balancer and take the
> product down for a cache outage — strictly worse than the revocation lag it would avoid.

### Scenario: readiness reports not-ready when the required dependency is unreachable

**Given** Postgres is unreachable
**When** the load balancer sends `GET /readyz`
**Then** the response status is 503
**And** the body is the standard error envelope
**And** the error code is `SERVICE_UNAVAILABLE`

### Scenario: readiness failure renders its message in the request locale

**Given** Postgres is unreachable
**When** `GET /readyz` is sent with `Accept-Language: vi-VN`
**Then** the envelope `message` is the `vi-VN` string for `SERVICE_UNAVAILABLE`
**And** the same request with `Accept-Language: en-US` returns the `en-US` string
**And** the error `code` is byte-identical in both responses

> The three-planes rule (`docs/audit-and-errors.md` §1a) as a test: the locale changes the
> message and nothing else. Clients branch on `code`, never on `message`.

### Scenario: readiness failure does not disclose which dependency failed

**Given** Postgres is unreachable
**When** an unauthenticated caller sends `GET /readyz`
**Then** the response body does not name Postgres, Redis, any host, port, or driver error
**And** the underlying error is present in the structured log

### Scenario: a hung dependency does not hang the probe

**Given** Postgres accepts connections but never answers
**When** the load balancer sends `GET /readyz`
**Then** the check abandons that dependency once its timeout elapses
**And** the response is returned within the probe's overall budget
**And** the response reports not-ready

> Without a per-check timeout the probe inherits the dependency's hang, and the load
> balancer's own timeout fires instead — which reports the same 503 far more slowly and
> leaves a connection pinned for every probe interval.

### Scenario: the server starts when a dependency is unavailable at boot

**Given** Postgres is unreachable
**When** the server process starts
**Then** the process stays running and binds its port
**And** `GET /healthz` returns 200
**And** `GET /readyz` returns 503
**And** readiness becomes 200 once Postgres is reachable, with no restart

> The alternative — exiting at boot — means that during a Postgres outage no task can reach
> a running state at all, so the platform crash-loops instead of waiting, and recovery needs
> a manual deploy rather than happening on its own.

### Scenario: a panic in a handler returns the error envelope

**Given** a registered handler panics
**When** a request reaches that handler
**Then** the response status is 500
**And** the body is the standard error envelope with code `INTERNAL_ERROR`
**And** the panic and stack trace are written to the structured log
**And** the process continues serving subsequent requests

### Scenario: a caller-supplied request ID is echoed, and generated when absent

**Given** the gateway is running
**When** a request arrives carrying `X-Request-ID`
**Then** the response carries the same `X-Request-ID`
**And** the log line for that request carries it
**When** a request arrives without the header
**Then** the response carries a generated `X-Request-ID`

### Scenario: probes are unauthenticated and unversioned

**Given** the gateway is running
**When** `GET /healthz` and `GET /readyz` are sent with no credentials
**Then** both are served
**And** neither path is under `/api/v1`

> They are consumed by an ALB and a container runtime, neither of which can hold a
> credential; versioning them would make an API version bump an infrastructure change.

### Scenario: the chain leaves the auth stages unbuilt but positioned

**Given** the middleware chain as registered by this story
**When** the registered middleware is inspected
**Then** recovery, request ID, logging, CORS and locale are present and ordered
**And** rate limiting, authentication, CSRF and role checking are absent
**And** each absent stage has a named registration point in the declared order

> Ordering is what is expensive to retrofit; the stages themselves belong to the stories that
> specify them (FR-20/22/24, NFR-04).
