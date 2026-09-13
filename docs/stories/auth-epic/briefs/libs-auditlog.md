# Executor brief: libs-auditlog

**Slice ID:** `libs-auditlog`
**Story / spec:** [`docs/audit-and-errors.md`](../../../audit-and-errors.md) ·
[PLAT-001](../../PLAT-001-audit-log-and-error-codes/story.md) ·
SRS-AUTH-001 FR-45, NFR-07, SEC-07
**Domain:** backend library
**Depends on:** none — may start immediately
**Load skill:** `multi-agent-execution` (`.claude/skills/multi-agent-execution/SKILL.md`)
**Read first:** [`docs/audit-and-errors.md`](../../../audit-and-errors.md) (all of it) ·
[`docs/backend.md`](../../../backend.md) §5a, §7 · [`../contract.md`](../contract.md) §4, §7

## Task

Build `libs/auditlog` — the one way an audit record is written.

`libs/auditmodel` declares *what* events exist; this library *persists* one. Every wave-3 slice
has scenarios of the form "and a WARN-level security event is logged" or "and an audit record is
written naming the admin, the action, the target", and all of them will assert against the fake
you ship here. If this library is awkward to fake, three slices write three different
work-arounds.

## You own (exclusive write access)

- `libs/auditlog/**`

## Read-only context

- `docs/audit-and-errors.md` — §1a (the three language planes), §3 (event shape and rules),
  §6 (compact codes, and that translation happens at the storage boundary only)
- `docs/stories/auth-epic/contract.md` §4 (`audit.records` DDL) and §7 (the events being added)
- `libs/auditmodel/generated.go` — the event specs, the enum constants, and the
  readable ↔ compact maps. Read what it actually exposes before designing your API around a
  guess. **`plat-audit-model` is regenerating this file in parallel**; it only *appends* entries,
  so code against the shapes, not against a specific event list.
- `services/monolith/internal/platform/db/db.go` — the pgx pool you accept as a dependency
- `libs/i18n/i18n.go` — read it to see what you must **not** do: audit messages are never
  localized

## Do not touch

- `libs/auditmodel/**` — owned by `plat-audit-model`
- `libs/authmw/**` — owned by `libs-authmw` (your peer this wave)
- `infra/db/migrations/**` — owned by `db-migrations`; the `audit.records` DDL is theirs, and
  you code against contract §4's version of it
- Anything under `services/`
- `go.mod` / `go.sum` — owned by `be-wiring`

## Contract you implement against

### The row (contract §4)

```sql
audit.records(
  id bigserial, occurred_at timestamptz, event_code char(8), actor_type char(8),
  outcome char(8), severity char(8), actor_user_id uuid NULL, target_user_id uuid NULL,
  request_id text NULL, fields jsonb NOT NULL DEFAULT '{}', message text NOT NULL)
```

### What a write must do

1. **Validate the event's fields against its generated spec.** A declared-required field that is
   absent, or a field the event never declared, is a programming error and must fail loudly at
   the call site rather than producing a record nobody can query. The generator already refuses
   a message template naming an undeclared field; this is the runtime half of the same rule.
2. **Translate to compact codes at the boundary** (`audit-and-errors.md` §6): the event name,
   `actor_type`, `outcome`, `severity`, and any enum-valued field go into the row as their
   generated 8-char codes. Nothing compact appears in the API you expose — a caller passes
   readable constants and gets readable values back.
3. **Render the message in en-US, always** (`audit-and-errors.md` §1a). The request's
   `Accept-Language` must not reach this library at all. If your `Write` signature has room for
   a locale, delete it: making it impossible is better than documenting that it is wrong.
4. **Never persist a secret** (NFR-07). Reject — do not silently drop — a field whose name looks
   like a credential (`password`, `token`, `secret`, `hash`, `key`, `authorization`). The
   generator has the same check; this is the belt to its braces, because a field value assembled
   at runtime is exactly what the generator cannot see.
5. **Insert only.** No update path, no delete path, no "correct a record" helper (FR-45). A
   correction is a new record.

### The API shape

- A `Writer` interface with a single write method, taking the event, a field map, and the
  actor/target/request-id context.
- A pgx-backed implementation.
- **A `Fake` in the same package** recording what was written, with enough accessors that a
  handler test can assert "exactly one `auth.refresh.replay_detected` with severity `critical`
  and `replayed_jti` equal to X" without a database. This is a deliverable, not a convenience —
  it is what lets three wave-3 slices test their audit assertions with no Docker.
- Writing must never fail the request that triggered it: a database error is logged at `error`
  and swallowed, except that the *fake* surfaces errors so tests can assert them. An audit write
  that 500s the user's login is a worse outcome than a missing row, and the row's absence is
  visible in the operational log.

## Acceptance-criteria scenarios this slice covers with automated tests

No acceptance-criteria scenario is directly observable here — the scenarios that *mention* audit
records ("a WARN-level security event is logged", "an audit record is written naming…") are
owned by the slices that emit them, asserting against your `Fake`. What you must test:

| What | Kind | Location |
|---|---|---|
| a valid event round-trips: readable in, compact stored, en-US message rendered from the template | unit + integration (env-gated `GRINDSTATS_TEST_DB`) | `libs/auditlog/writer_test.go` |
| a missing required field, and an undeclared field, both fail at the call site | unit | ” |
| a credential-looking field name is rejected | unit | ” |
| the rendered message is en-US regardless of any ambient locale state | unit | ” |
| the `Fake` records what the real writer would have written, for the same call | unit | `libs/auditlog/fake_test.go` |

## Done when

- [ ] `Writer`, a pgx implementation, and a `Fake` are exported, and the `Fake` is good enough
      that a handler test can assert an event, its severity and one field value
- [ ] Every test above passes: `go test ./libs/auditlog/...`, and with a database for the gated
      one: `GRINDSTATS_TEST_DB=... go test ./libs/auditlog/...`
- [ ] There is no code path in the library that updates or deletes a row
- [ ] There is no parameter, anywhere in the exported API, through which a locale could reach a
      write
- [ ] Builds: `go build ./... && go vet ./...`; suite green: `go test ./...`
- [ ] Nothing modified outside the allowlist above (check your own diff)

## Stop condition — read this before starting

If you find the contract wrong, ambiguous, or insufficient for this task:

> **Stop work on the affected surface. Write an amendment request using
> `.claude/skills/multi-agent-execution/assets/amendment-request-template.md`. Do not proceed on
> that surface.**

Do not fix the contract yourself, and do not implement around it. Other slices are being built
against the current version right now; a local fix produces two implementations that each
believe they conformed, and nothing detects the mismatch until runtime.

If an event you need is missing from `model.yaml`, that is an amendment request — `model.yaml`
belongs to `plat-audit-model` and adding an event locally would produce a record with no
generated constant behind it.

## Report back

Write the report to `docs/stories/auth-epic/reports/libs-auditlog.md`.

1. **Changed:** files touched, and what each change does
2. **Verified:** commands run and their actual results, including whether the database-gated test
   ran
3. **Could not do:** anything blocked, and why
4. **Noticed:** problems outside your scope. Report them, don't fix them.
