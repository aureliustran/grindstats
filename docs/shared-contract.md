# Shared contract

**Status:** authoritative for everything that crosses a boundary.
**Companions:** [`backend.md`](backend.md) · [`frontend.md`](frontend.md)

This document is the **only sanctioned coupling** between the backend and the frontend, and
between any two domains. If a dependency isn't written here, it doesn't exist and must not
be relied on. It is also the registry through which every other specification is reachable
(§6) — so a reader who finds this file can find everything else without knowing what to
look for.

---

## 1. Why a contract document exists at all

Two agents (or two people) building opposite sides of the same seam cannot both be the
authority on its shape. If they are, each one implements what seemed reasonable from their
side, both pass their own tests, and the mismatch surfaces at integration — after all the
work is done, at the point where it is most expensive to fix.

So the seam is written down **before either side is built**, and both sides implement
against the written thing rather than against their assumptions about each other.

That only works if the contract is treated as immovable during implementation. Which leads
to the rule this whole document rests on:

> ### The contract is frozen during execution.
>
> Once implementation starts against a contract version, no implementer may change it —
> not to fix an obvious mistake, not to add a field they need. Discovering the contract is
> wrong is a **stop-and-escalate** event, not a fix-it-locally event (§4).

The failure this prevents is specific and bad: two implementers each "fix" the contract to
suit their own side, each ships something internally consistent, and the two fixes are
mutually incompatible. Both sides now believe they conformed. Nothing detects it until
runtime, and the contract file no longer describes either implementation.

---

## 2. What the contract covers

| Surface | Source of truth | Format |
|---|---|---|
| HTTP API | OpenAPI spec generated per domain from Gin route comments (swaggo) | OpenAPI 3 |
| Frontend API client | Generated from the above — never hand-written | TypeScript |
| Events | Routing keys, publishers, consumers, payload shapes (blueprint §11) | Table + payload schemas |
| Errors | `{ "error": { "code": "...", "message": "..." } }` — every endpoint, no exceptions. Codes are declared in [`libs/auditmodel/model.yaml`](../libs/auditmodel/model.yaml) and generated for both sides; see [`audit-and-errors.md`](audit-and-errors.md) | Envelope + generated code registry |
| Success responses | `{ "data": ... }` for a single resource/result, `{ "data": [...], "pagination": {...} }` for a list. Written only via [`libs/httpkit/envelope.go`](../libs/httpkit/envelope.go) (`OK`/`Created`/`Data`/`Paginated`). `/healthz`/`/readyz` (GATE-001) predate this and keep their own unenveloped shape | Envelope |
| Auth | Cookie transport, CSRF header, token claims, check order | [`srs-authentication.md`](srs-authentication.md) |
| Cross-domain interfaces | Exported service interfaces (in-process today, HTTP at Phase 10) | Go interfaces |

### Prefer machine-checkable over prose

Wherever a contract element can be expressed as something a program can verify — an
OpenAPI document, a JSON Schema, a TypeScript type, a Go interface — it is expressed that
way, and this document links to it rather than restating it. Prose restatements of a spec
drift from the spec silently; a generated client that no longer compiles does not.

Prose in this document is reserved for what a schema cannot carry: policy, ordering
guarantees, idempotency requirements, and the reasoning behind a shape.

---

## 3. Standing policies

These hold across every endpoint and event, and do not need restating per-feature.

**API**
- Versioned `/api/v1/...` paths from day one
- **Every response body is enveloped, success or error, no exceptions** (`/healthz`/`/readyz`
  excepted — see the table above). A client can rely on `data`/`error` being present at the
  top level without knowing which endpoint it called
- The error envelope is universal; error codes are stable identifiers, not display text
- A list response's `pagination.total_pages` is always server-computed; a client never
  derives it from `total_items`/`page_size` itself
- **Every request carries `Accept-Language`.** The server renders the envelope's `message`
  in that locale from `libs/i18n/locales/`, falling back to en-US. The header affects the
  *response only* — audit records are always written in en-US, because a per-locale audit
  log cannot be searched or aggregated (`audit-and-errors.md` §1a)
- Clients branch on `code`, never on `message`. A client renders its own string for codes it
  knows and falls back to the server's `message` for ones it doesn't — which is what lets the
  server ship a new code before every client is updated
- Aggregation endpoints return pre-bucketed series, never raw rows for the client to reduce
- The effective user ID always comes from the token, never a client-supplied parameter

**Two-step writes for anything LLM-generated**
`POST /estimate` → user reviews and edits → `POST /confirm`.
`POST /propose` → user reviews and edits → `POST /confirm`.
Never a single endpoint that generates and persists. This is a contract-level rule because
it binds both sides: the server must not persist on the first call, and the client must not
auto-confirm on the user's behalf.

**Events**
- At-least-once delivery; every consumer is idempotent
- Events carry facts, not commands
- Payloads are self-contained — a consumer never calls back into the producer to interpret
  an event
- Arrival order is not guaranteed; required ordering must be derivable from the payload

**Compatibility**
- Additive changes (new optional field, new endpoint, new event) are backward-compatible and
  may ship without a version bump
- Removing or renaming a field, changing a type, or tightening validation is **breaking** and
  requires the expand/contract treatment described in [`backend.md`](backend.md) §3 — ship
  the addition, migrate consumers, remove later, as separate deployments
- A breaking change is never bundled with the feature that motivated it

---

## 4. Amendment protocol

When an implementer finds the contract wrong, ambiguous, or insufficient:

1. **Stop work on the affected surface.** Not on everything — on the part that depends on
   the disputed shape.
2. **Write an amendment request**: what the contract says, what's wrong with it, what you
   propose instead, and which slices are affected. Keep it to the seam; don't redesign
   adjacent things while you're in there.
3. **The instructor (or contract owner) decides**, updates this document and the relevant
   schema, and notifies every affected implementer — including ones who had already finished
   against the old shape.
4. **Only then does work resume**, against the amended version.

Step 3's "including ones who had already finished" is the part that gets skipped and is
exactly why the protocol exists: an amendment that reaches only the implementer who
requested it produces the same two-incompatible-implementations outcome the freeze rule was
meant to prevent.

An amendment is cheap. Discovering at integration that two sides disagree is not.

---

## 5. Contract-conformance checks

A slice is not done because its own tests pass. It is done when it demonstrably matches the
contract:

- The generated API client compiles against the current OpenAPI spec, and the spec
  regenerates from the route comments with no diff
- Every documented error code an endpoint can return is actually returnable, and every code
  it returns is documented
- Event payloads validate against their declared schemas, from both the publisher's and the
  consumer's side
- Locale catalogs are in parity (`scripts/check_i18n_parity.py`)

Where a check can be automated it should be, and run in CI (roadmap Phase 12) — a
conformance rule enforced only by review is a rule that holds until the first busy week.

---

## 6. Document registry

Every other specification hangs off this file. Anything a new contributor or agent needs is
reachable from here in one hop.

| Document | Covers |
|---|---|
| [`backend.md`](backend.md) | Service catalog, domain ownership, migrations, events, API conventions |
| [`frontend.md`](frontend.md) | Slice structure, import boundaries, shared-dependency discipline |
| [`design-system.md`](design-system.md) | Visual and interaction rules, tokens, accessibility floors |
| [`i18n-guidelines.md`](i18n-guidelines.md) | en-US / vi-VN rules, formatting, voice-critical strings |
| [`srs-authentication.md`](srs-authentication.md) | SRS-AUTH-001: auth, sessions, roles, security requirements |
| [`audit-and-errors.md`](audit-and-errors.md) | Enumerations, audit events, and the error codes derived from them — model file, rules, generator |
| [`deployment-aws.md`](deployment-aws.md) | How the project is deployed and released — the cheap Phase 1–2 shape, not the blueprint's target |
| [`stories/index.md`](stories/index.md) | Every user story, by code, with acceptance criteria and test cases |
| [`stories/auth-epic-overview.md`](stories/auth-epic-overview.md) | How the five auth stories relate and depend on each other |
| [`blueprint-url.txt`](blueprint-url.txt) | Link to the published architecture blueprint (the overall source of truth) |
| `.claude/skills/multi-agent-code-execution/` | The four-phase pipeline for work spanning these boundaries (instruction → execution → testing → bug fixing), the gates between phases, and the run folder the story folder |
| `.claude/skills/multi-agent-instruction/` | Phase 1: contract freeze, slice partitioning with disjoint ownership, executor briefs; owns the amendment protocol in §4 |
| `.claude/skills/multi-agent-execution/` | Phase 2: executor rules, stop-and-escalate, frontend/backend slice references |
| `.claude/skills/multi-agent-testing/` | Phase 3: contract conformance, story test cases, end-to-end path; defect reports, no code changes |
| `.claude/skills/multi-agent-bug-fixing/` | Phase 4: scoped fix briefs, regression-test-first fixes, retest hand-back |

**Adding a specification?** Link it here in the same commit that creates it. A document
nobody can find from this table is a document that will be contradicted by someone who
didn't know it existed — which is worse than not having written it, because now two things
claim authority.

Feature-specific contracts (the endpoints and events a single story introduces) belong in
that story's folder under `docs/stories/<CODE>-<slug>/`, and are linked from the story
rather than duplicated here. This document carries what is cross-cutting and durable; a
story carries what is specific to it.
