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

### Architecture documents

Three documents describe what crosses which boundary. Read the relevant ones before writing
code that spans more than one domain:

- **`docs/backend.md`** — service catalog, domain ownership, migrations (single-writer,
  expand/contract), events, API conventions
- **`docs/frontend.md`** — slice structure, the import rule, shared-dependency discipline
- **`docs/shared-contract.md`** — the only sanctioned coupling between backend and frontend,
  the contract-freeze and amendment rules, and the registry linking every other spec

Both architecture docs open with a **"Target vs. now"** table. The distributed/micro-frontend
architecture they describe mostly does not exist yet — the boundaries are real today as
package and folder rules, not as deployments. Building the target shape during an early
roadmap phase is the most common way to waste a week here.

### Audit logs, enums and error codes

Declared once in `libs/auditmodel/model.yaml` and generated into Go
(`libs/auditmodel/generated.go`) and TypeScript (`apps/web/src/api/generated/audit.ts`) —
**never hand-edit the generated files.** Run `python3 scripts/gen_audit_model.py` after any
change, and `--check` in CI.

- **Enum values are permanent storage identifiers, never display text.** Renaming one
  invalidates stored rows and append-only audit records. Display text comes from i18n keys.
- **Never log a secret** — log the handle (`jti`), never the token.
- **Several audit events may map to one error code, deliberately.** Unknown-email and
  bad-password are distinct events but one `AUTH_INVALID_CREDENTIALS`, because a
  distinguishable response is an account-enumeration oracle (FR-08, FR-14).
- **Three language planes, never conflated:** audit records are written in **en-US always**
  (a per-locale audit log can't be searched or aggregated, and it's append-only, so there is
  no later fix); API response `message`s are rendered server-side from `libs/i18n/locales/`
  via the request's `Accept-Language`; the SPA renders its own strings from
  `apps/web/src/i18n/locales/`. The request locale affects the response only — it must never
  reach storage.
- The two catalog sets live in separate directories and are **not copies** — only their
  internal coverage is checked, independently, by `scripts/check_i18n_parity.py`.
- The SPA renders its own string per error code by default and shows the server's `message`
  only where a story allows it (`renderServerMessage()`), or as the fallback for a code it
  doesn't recognize.

Rules and the open `AUTH_ACCOUNT_SUSPENDED` tension: `docs/audit-and-errors.md`. Workflow:
the `audit-log-and-error-codes` skill.

### Multi-agent work

For a feature spanning backend and frontend, or several services/slices at once, use the
`multi-agent-code-execution` skill (`.claude/skills/multi-agent-code-execution/`): one
instructor agent writes the contract and partitions the work into slices with **disjoint
file ownership**, then executors build against it in parallel without touching each other's
paths or amending the contract. Don't fan out parallel agents without that partitioning
step — two agents editing one file, or each inventing its own version of an API, is the
failure it exists to prevent.

### Frontend: design system and i18n

Two documents are authoritative for anything rendered to a user. Read them
before writing UI; they exist so that work done in separate sessions by
separate people or agents still adds up to one coherent product.

- **`docs/design-system.md`** — the visual and interaction rules, and the
  reasoning behind them. §7 is a self-check list phrased so the answers are
  observable rather than a matter of taste.
- **`docs/i18n-guidelines.md`** — `en-US` and `vi-VN` rules. §7 is the
  equivalent definition of done.

### User stories

Feature specs live under `docs/stories/<CODE>-<slug>/` — one folder per story, each with
`story.md`, `acceptance-criteria.md`, `test-cases.md` and `diagram.md`. Every story has a
code (`LAND-001`, `AUTH-002`, ...), area-prefixed and sequential within its area; the
folder name and `story.md`'s own header both carry it. `docs/stories/index.md` lists every
story with its code and tracks the next available number per area — check it before
assigning a new one, and update it when you add a story. The `user-story-documentation`
skill (`.claude/skills/user-story-documentation/`) automates this whole workflow, including
code assignment; use it rather than hand-rolling a story doc.


The three rules most likely to be broken by accident:

- **`apps/web/src/styles/tokens.css` is the single source of truth for every
  design value.** Tailwind consumes those custom properties and defines none of
  its own. Never write a raw hex, px or ms into a component — if the value you
  need doesn't exist, add a token with a comment saying what it's for.
- **No user-facing string is hardcoded.** Not labels, not errors, not
  `aria-label`, `alt`, `title` or `placeholder`. Every one resolves through a
  translation key that exists in *both* catalogs
  (`scripts/check_i18n_parity.py` enforces this).
- **Every number goes through `Intl.NumberFormat` with the active locale**, in
  the mono face with tabular figures, carrying its unit. `vi-VN` inverts the
  separators — "2.620" means 2620 in Vietnamese — so a raw `toFixed()` in the
  DOM is a real defect in a calorie app, not a formatting nitpick.

Layouts are designed for Vietnamese, which runs 10–30% longer than English and
stacks diacritics vertically. An English-only mockup is not a finished design.

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
