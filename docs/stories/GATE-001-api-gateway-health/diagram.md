# Flow: API gateway skeleton with health and readiness probes

**Story:** [`GATE-001`](story.md) · **Criteria:** [`acceptance-criteria.md`](acceptance-criteria.md)

Two diagrams, because the story has two distinct shapes worth showing: the readiness
decision tree, and the order in which a request passes through the chain.

## 1. Probe handling and the readiness decision

```mermaid
flowchart TD
    A[Probe arrives] --> B{Which path?}

    B -->|GET /healthz| C[Report process alive]
    C --> D[200 alive]

    B -->|GET /readyz| E[Check required and optional<br/>dependencies, each under<br/>its own timeout]

    E --> F{Postgres reachable<br/>within timeout?}
    F -->|no| G[Log the underlying error]
    G --> H[Render SERVICE_UNAVAILABLE<br/>from the request locale]
    H --> I[503 envelope<br/>no dependency named]

    F -->|yes| J{Redis reachable<br/>within timeout?}
    J -->|yes| K[200 ready]
    J -->|no| L[Log a warning naming Redis]
    L --> M[200 degraded]

    style D fill:#1b5e20,color:#fff
    style K fill:#1b5e20,color:#fff
    style M fill:#e65100,color:#fff
    style I fill:#b71c1c,color:#fff
```

**Why `/healthz` never reaches the dependency checks.** Its consumer is the container
runtime, whose response to a failure is to kill the task. Consulting Postgres here would
turn a brief outage into a rolling restart of healthy containers.

**Why Redis-unreachable exits at `200 degraded` rather than joining the 503 branch.**
NFR-02 fails open on read-only GETs when Redis is down, so the instance can still serve.
Routing this branch to 503 would empty the load balancer during a cache outage — worse than
the revocation lag it would avoid.

**Why the 503 branch logs before rendering.** The log is where the host, port and driver
error go; the response body is deliberately stripped of them, so the log line is the only
copy.

## 2. The middleware chain a probe passes through

```mermaid
sequenceDiagram
    participant LB as Load balancer
    participant R as Recovery
    participant ID as Request ID
    participant LOG as Logging
    participant LOC as Locale
    participant RES as ⟨reserved: rate limit → auth → CSRF → role⟩
    participant H as Health handler
    participant PG as Postgres
    participant RD as Redis

    LB->>R: GET /readyz
    R->>ID: pass through
    ID->>LOG: attach or generate X-Request-ID
    LOG->>LOC: start timing
    LOC->>RES: resolve Accept-Language
    Note over RES: not built by this story —<br/>registration points only
    RES->>H: dispatch
    par bounded by per-check timeouts
        H->>PG: ping
        PG-->>H: ok / error / timeout
    and
        H->>RD: ping
        RD-->>H: ok / error / timeout
    end
    H-->>LOG: status + body
    LOG-->>LB: response, X-Request-ID echoed
```

The reserved participant is the point of this diagram. The four unbuilt stages have a fixed
position in the order, and each later story inserts its own — inserting a stage into an
existing order is cheap, whereas discovering later that CSRF ran before authentication is
not.

The `par` block reflects that the two dependency checks are independent: running them
sequentially would make the probe's worst case the sum of the timeouts rather than the
larger of the two.
