# Backend architecture & database

**Status:** authoritative for server-side work and all schema changes.
**Companions:** [`frontend.md`](frontend.md) · [`shared-contract.md`](shared-contract.md)
(the contract is the only sanctioned coupling between this document and the frontend).

> **Read the "Target vs. now" section before building anything.** This document describes
> a distributed system that mostly does not exist yet. Building the target shape during an
> early roadmap phase is the single most likely way for an agent to waste a week here.

---

## 1. Target vs. now

| | Target (blueprint §2–3) | Now (roadmap Phase 1–2) |
|---|---|---|
| Deployment | 10 independent Go services behind a gateway | **One Gin binary**, `services/monolith/` |
| Boundaries | Network calls + RabbitMQ events | **Internal Go packages** named after the future services |
| Data | One schema per service | One Postgres, one schema per domain namespace |
| Messaging | RabbitMQ topic exchange | **None yet** — direct function calls |

The service boundaries in §2 are real *now* as **package boundaries and ownership rules**,
even though the network hop doesn't exist yet. That's the point: code written respecting
these boundaries can be extracted at Phase 10 by moving a package and adding transport.
Code that reaches across them cannot, at any price short of a rewrite.

**So the rule that matters today is not "call over HTTP" — it is "never reach into another
domain's tables or internals, even though the compiler would let you."**

Check the roadmap phase (blueprint §20, summarized in `CLAUDE.md`) before introducing
RabbitMQ, a second binary, or a service-to-service HTTP call. If the phase doesn't call for
it, the equivalent in-process seam is the correct implementation.

---

## 2. Service catalog and ownership

Ten domains. Each owns its tables exclusively. In the monolith these are packages under
`services/monolith/internal/<domain>/`; at Phase 10 each becomes `services/<name>/`.

| Domain | Responsibility | Owns (schema) | Publishes |
|---|---|---|---|
| `gateway` | Routing, JWT validation, rate limiting, fan-out. **The only component that reads Redis for auth.** | — | — |
| `auth` | Signup/login, JWT issue & refresh, OAuth2, blacklist/epoch | `auth.*`, Redis | — |
| `user` | Profile, preferences, units, goals | `user.*` | — |
| `routine` | Habits, routines, schedules, live occurrence checklists | `routine.*` | `routine.*` |
| `nutrition` | Food log, calorie intake, macros, meal-photo estimation | `nutrition.*`, S3 | `calorie.*` |
| `metrics` | Body composition, resistance & cardio logs, BMR/MET/EPOC, trends & aggregation | `metrics.*` | `body_metric.*`, `training.*` |
| `diet` | Diet plans, meal templates, macro targets | `diet.*` | — |
| `chatbot` | RAG orchestration, Qwen calls, conversation history, weekly insights | Elasticsearch, Redis | `chat.*`, `insight.*` |
| `subscription` | Plans, entitlements, POS webhooks, linked accounts | `subscription.*` | `subscription.*` |
| `notification` | Push/email reminders, digests. Pure consumer; no public API initially. | `notification.*` | `notification.*` |

### The ownership rule, stated precisely

A domain may **not**:
- read or write another domain's tables, directly or through a shared repository type
- import another domain's internal packages
- assume another domain's storage layout, even transitively through a shared struct

A domain **may** reach another domain only through a seam declared in
[`shared-contract.md`](shared-contract.md): an API call (now: an exported service
interface; later: HTTP) or an event.

`diet` reading nutrition aggregates is the canonical example — it goes through nutrition's
API surface, never through `nutrition.*` tables, even though both live in the same database
today.

---

## 3. Database and migrations

### Migrations are single-writer, always

**Exactly one agent or person owns migrations for any given unit of work.** This is not a
style preference — it's because migration files are globally ordered by a number or
timestamp in their filename, and two agents working in parallel will both produce
`0007_...` without either noticing. One of them then silently loses, or the merge conflicts
in a way that leaves the schema half-applied on someone's local database.

If a feature needs schema changes across two domains, the migration work is **one slice
assigned to one executor**, and the other executors depend on it rather than each writing
their own.

### Migrations are additive first (expand/contract)

Backend and frontend deploy independently, and old server instances run alongside new ones
during a rolling deploy. So a migration that renames or drops a column breaks whatever is
still running against the old shape. Use the two-step:

1. **Expand** — add the new column/table, backfill, write to both old and new. Ship.
2. **Contract** — once nothing reads the old shape, drop it. A separate, later migration.

A single migration that adds a column and drops another is almost always a bug in disguise.

### Rules

- Migrations live in one place, are numbered sequentially, and are **never edited once
  merged** — a mistake is corrected by a new migration, not by rewriting history
- Every migration has a tested rollback path, or an explicit note in the file saying why it
  is irreversible (e.g. a data-destroying contract step)
- No migration is applied by application startup in production; migration is a deliberate,
  separate step
- Seed/reference data belongs in its own migration, distinguishable from schema changes

### Schema namespaces

One Postgres instance, one schema per domain: `auth.*`, `user.*`, `routine.*`,
`nutrition.*`, `metrics.*`, `diet.*`, `subscription.*`, `notification.*`. The namespace is
what makes the ownership rule in §2 mechanically visible in a code review — a query naming
a schema the current package doesn't own is a violation you can grep for.

The per-domain table listing lives in the blueprint (§12) and is not duplicated here; the
routine/occurrence model in particular is fully specified there and in
`docs/stories/`.

---

## 4. Cross-domain communication

### Synchronous (API / interface call)

Used when the caller needs an answer to proceed. Declared in `shared-contract.md` as an
interface, with its error cases. Today an exported Go interface; at Phase 10 an HTTP call
behind the same interface — which is why the interface, not the transport, is what's
declared.

### Asynchronous (events)

Used when the caller doesn't need an answer, and for anything that fans out. RabbitMQ topic
exchange at target; the in-process equivalent until then.

Event rules that survive the transition to real messaging, and must be respected from the
first in-process publish:

- **At-least-once delivery.** Every consumer must be idempotent — processing the same event
  twice must produce the same result as processing it once. Carry an event ID and make the
  consumer's write conditional on not having seen it.
- **Events carry facts, not commands.** `training.logged` (a fact) not `update_analytics`
  (a command). A fact has one producer and any number of consumers; a command implies the
  producer knows who acts on it, which re-couples what the event was meant to decouple.
- **Payloads are self-contained.** A consumer must not have to call back into the producer
  to understand the event — that recreates the synchronous dependency the event removed.
  `routine.task_checked` therefore carries the linked `training_set_ids` rather than an
  instruction to go look them up.
- **Ordering is not guaranteed.** If two events must be applied in order, that ordering must
  be derivable from the payloads (timestamps, versions), not from arrival sequence.

The event catalog — routing keys, publishers, consumers, payloads — is in the blueprint
(§11) and mirrored in `shared-contract.md` where it forms part of the cross-domain contract.

---

## 5. API conventions

- REST + JSON, versioned `/api/v1/...` from day one
- OpenAPI generated per domain from Gin route comments (swaggo) — this generated spec is
  what `shared-contract.md` points the frontend at
- Error envelope, everywhere, no exceptions: `{ "error": { "code": "...", "message": "..." } }`
- Aggregation endpoints return **pre-bucketed series**, not raw rows — the client never does
  analytics arithmetic
- **Two-step writes for anything LLM-generated**: `POST /estimate` → `POST /confirm`,
  `POST /propose` → `POST /confirm`. Never one endpoint that generates and persists
- The effective user ID always comes from the token, never from a client-supplied parameter

## 5a. Messages, locales and the audit log

The server renders user-facing message text itself — it is not only an API for one SPA, and
emails, webhooks and future non-browser clients all need a localized message.

- **Error messages come from `libs/i18n/locales/<locale>.json`**, keyed by the error code,
  selected by the request's `Accept-Language` header, falling back to en-US. Never build an
  error string in Go, and never send English text and expect a client to translate it.
- **Audit records are always written in en-US**, whatever the requester's locale. A
  per-actor-language audit log cannot be searched or aggregated, and the records are
  append-only, so there is no later fix. The request locale affects the response only — it
  must never reach storage.
- The structured fields on an audit record are the real record; the rendered en-US message
  is a convenience for a human reading one row. Anything a query must filter on belongs in a
  field, not in the message text.
- Error codes, enums and audit events are declared in `libs/auditmodel/model.yaml` and
  generated — see `docs/audit-and-errors.md`. Never hand-write an error code string.

## 6. Computation rules

- **"Services compute, the LLM narrates."** Every number a user sees is produced by
  deterministic Go, stored, and handed to the model pre-computed
- Formulas live once, in `libs/physiology/`, imported everywhere — never reimplemented
  per domain
- Any new formula or cadence rule cites its source, and keeps the population-level /
  not-clinical caveat attached in both code comments and user-facing copy
- The numeral-validation guard on LLM output (every number in generated text must appear in
  the payload) is a checked invariant, not a hope
