# CLAUDE.md

Guidance for Claude (or any AI coding assistant) working in this repository.

## What this is

**GrindStats** is a health/fitness coaching platform: body-metrics and training
tracking (resistance + cardio), meal-photo calorie/nutrition estimation, a
Qwen-backed RAG chatbot that proposes and adapts routines, and a live
checklist-based routine execution flow. Solo, fresher-level build — favor the
simplest thing that works over the "correct" distributed-systems version until
the roadmap says otherwise.

The authoritative spec is the published blueprint (architecture, data models,
formulas, roadmap, AWS deployment plan): see `docs/blueprint-url.txt` for the
link, or ask the repo owner for it. **When this file and the blueprint
disagree, the blueprint wins** — update this file to match rather than the
other way around.

## Stack

- **Backend**: Go, Gin framework, microservices behind a single API
  gateway/BFF (until Phase 10 of the roadmap, most of `services/` is actually
  one Go module with internal packages — see `services/monolith/`)
- **Frontend**: React SPA
- **Data**: PostgreSQL (system of record), Elasticsearch (full-text + kNN
  vector retrieval for RAG, operational log), Redis (JWT blacklist/session,
  rate limiting, hot-read cache)
- **Messaging**: RabbitMQ (topic exchange, event-driven between services)
- **AI**: Qwen (DashScope) — text model for chat/RAG, Qwen-VL for meal-photo
  estimation
- **Auth**: JWT (RS256) + OAuth2, statelessness resolved via a Redis
  blacklist strategy (per-token `blacklist:{jti}` + per-user
  `user_blacklist_epoch:{user_id}` for O(1) logout-everywhere)
- **Infra**: Docker Compose locally; AWS managed services in prod (ECS
  Fargate, RDS, ElastiCache, OpenSearch Service, Amazon MQ, S3+CloudFront) —
  no Terraform/CDK, high-level guidance only

## Non-negotiable conventions

- **Two-step writes for anything LLM-estimated or LLM-proposed.**
  `POST /meals/estimate` → `POST /meals/confirm`,
  `POST /routines/propose` → `POST /routines/confirm`. Never a single endpoint
  that generates and persists in the same call — the user always sees and can
  edit an LLM draft before it's saved.
- **"Services compute, the LLM narrates."** All numeric analytics (BMR, TDEE,
  training volume, trend significance) are computed deterministically in Go —
  never ask the LLM to do arithmetic or invent a number. The LLM receives a
  structured JSON payload of already-computed values with confidence
  metadata, and a post-generation Go check regex-extracts every numeral from
  its output and asserts it appears in that payload. An orphan number means
  regenerate once, then fall back to a template-rendered response.
- **Ad-hoc routine edits never silently mutate the template.** Checking,
  unchecking, or adding a task on today's occurrence only touches that
  occurrence. Promoting an ad-hoc task into the standing plan is a distinct,
  user-initiated action.
- **Unchecking a routine task never auto-deletes a logged training set.**
  Prompt once; don't delete data because a checkbox was toggled.
- **Formulas live in one place.** BMR/MET/EPOC constants and calculations go
  in `libs/physiology/`, imported by every service that needs them — never
  reimplemented per-service.
- **Consistent error envelope** across all services:
  `{ "error": { "code": "...", "message": "..." } }`.
- **Versioned API paths** from day one: `/api/v1/...`.
- **Aggregation endpoints return pre-bucketed series**, not raw rows the
  frontend has to reduce (e.g. `GET /metrics/trend?metric=body_fat&interval=week`
  returns `[{week, value}]` directly).

## Structure

```
grindstats/
├── apps/web/                    # React SPA
├── services/                    # Gin microservices (gateway, auth, routine,
│                                 # nutrition, metrics, diet, chatbot,
│                                 # subscription, notification, user)
├── libs/                        # shared Go modules (authmw, eventbus,
│                                 # qwenclient, physiology, httpkit)
├── infra/                       # docker/, ci/
└── docs/adr/                    # architecture decision records
```

## Working in this repo

- Check the blueprint's roadmap phase before adding a feature that isn't
  scoped yet — this is a sequenced solo build, not a full team sprint; don't
  jump ahead to later-phase infrastructure (e.g. don't stand up the full
  microservice split before Phase 10).
- Prefer editing `services/monolith/` internal packages over creating a new
  standalone service, until the roadmap says to extract one.
- Any new formula or measurement-cadence logic should cite its source
  (population-level sports-science estimates are explicitly not clinical —
  keep that caveat attached in both code comments and any user-facing copy).
- Run local dev via the root `docker-compose.yml`; don't require AWS access
  for anything in local development.
