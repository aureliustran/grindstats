# GrindStats

A distributed, event-driven health & fitness coaching platform: photo-based
meal logging, resistance + cardio training analytics grounded in real
exercise-science formulas, body-metrics tracking with a configurable
measurement cadence, routine agreement and live checklist execution, and a
Qwen-backed RAG chatbot that proposes and adapts routines from your own data.

> **Status:** blueprint / pre-implementation. This repo currently holds the
> project scaffold and planning docs — see the full architecture,
> data-model, and roadmap document linked below before writing code.

## Stack

| Layer | Choice |
|---|---|
| Backend | Go, Gin, microservices behind a single API gateway |
| Frontend | React |
| Datastore | PostgreSQL |
| Search / vector retrieval | Elasticsearch |
| Cache / session | Redis (JWT blacklist strategy, rate limiting) |
| Messaging | RabbitMQ |
| AI | Qwen (DashScope) — text + vision (Qwen-VL) |
| Auth | JWT (RS256) + OAuth2 |
| Deployment | Docker locally, AWS managed services in production |

## What it does

- Tracks body weight, body fat %, muscle mass, and circumference measurements
  at a **user-configurable cadence** per metric.
- Logs **resistance training** (sets/reps/load) and **cardio training**
  (duration/HR/intensity) as two distinct modalities, feeding separate
  analytics: training-volume trends for resistance, energy-expenditure
  (BMR + session + EPOC/"afterburn") for cardio.
- Estimates calories and macros from **meal photos**, using compulsory
  predefined shooting angles plus optional ingredient/serving hints.
- Runs a **routine agreement flow** (chatbot proposes, user edits, user
  confirms) that generates a live, checkable task list per session — with
  support for ad-hoc tasks added mid-workout.
- Uses a **RAG chatbot** to turn all of the above into grounded, numerically
  cited coaching advice — the LLM narrates pre-computed numbers, it never
  computes or invents them.

## Getting started

```bash
git clone <this-repo>
cd grindstats
cp .env.example .env
docker compose up -d          # Postgres on :5433, Redis on :6379

cd services/monolith
go run ./cmd/server           # http://localhost:8080/healthz
go test ./...                 # unit tests only
GRINDSTATS_TEST_DB=1 go test ./...   # + Postgres/Redis connectivity tests
```

Postgres is published on **5433**, not 5432, so it can't be shadowed by a
Postgres installed directly on the host machine.

See `CLAUDE.md` for repo conventions and structure, and `docs/adr/` for
architecture decisions as they're made.

## Full blueprint

The complete architecture, data models, sourced formulas (BMR, MET, EPOC,
adaptive TDEE, resistance-volume landmarks), event catalog, AWS deployment
plan, and phased build roadmap live in the project blueprint document —
ask the repo owner for the link if it isn't in `docs/`.

## License

TBD.
