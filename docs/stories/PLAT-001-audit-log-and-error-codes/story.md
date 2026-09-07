# Audit Log & Error Code Model

**Code:** `PLAT-001`

**As a** backend engineer implementing or extending a GrindStats service
**I want** to declare a new status value, audit event, or API error code once, in
`libs/auditmodel/model.yaml`, and have the Go constants/validators, TypeScript types, and
catalog obligations generated and checked automatically
**So that** the audit log, the API's error surface, and the frontend's error handling never
drift out of sync with each other or with what's actually stored in the database and the
append-only audit history

## Description

Every GrindStats service eventually needs the same three things for a new failure mode or
status: a closed-set value it can store and query, an audit record so the event is
forensically visible, and — sometimes — an external error code the caller can act on. Declared
independently per service, these three drift: someone adds a failure path in Go, forgets the
audit event, and a real failure becomes invisible in the log; or a new error code ships without
a message in one locale, and a caller in that locale gets an empty string.

`libs/auditmodel/model.yaml` is the single place this is declared, and
`scripts/gen_audit_model.py` is what turns one edit into three consistent outputs: Go constants
and validators (`libs/auditmodel/generated.go`), TypeScript types and error-code maps
(`apps/web/src/api/generated/audit.ts`), and a set of build-time checks that fail loudly rather
than shipping a silent gap (unknown enum/error-code references, unfilled message placeholders,
field names that look like secrets, missing HTTP status, missing locale coverage, orphaned
catalog entries, stale generated files). This story documents that workflow as the thing an
engineer actually does — not the finished model file, which already exists and is authoritative
per `docs/audit-and-errors.md`.

Three language planes stay distinct throughout: audit records are always written in en-US
(append-only, must stay aggregatable — no per-locale audit log), the API response `message` is
rendered server-side per the request's `Accept-Language` from `libs/i18n/locales/`, and the SPA
renders its own strings from `apps/web/src/i18n/locales/` by default, falling back to the
server's message only where a story explicitly allows it via `renderServerMessage()`.

What's actually written to the DB for an error code, an audit event name, or an enum value is
not the readable string, though — it's a generator-assigned 8-character compact code (4-letter
prefix + a kind digit, `0` for error codes, `1` for audit events, `2` for enum values + a 3-digit
sequence), translated back to the readable form in the backend service that reads the row,
before it reaches any handler, response, audit-message rendering, or application logic that
compares against the readable constant. For enum values, the prefix is derived from the *enum
name* (`RoutineStatus` → `ROUT`), not the value, so `RoutineStatus.active` and
`OccurrenceStatus.active` — same value spelling, different enums — never collide. See
`docs/audit-and-errors.md` §6 for the full format and the permanence guarantee that makes this
safe against an append-only log and against every domain table that stores one of these enums.

## In scope

- Adding a new enum value to an existing enum, or a new enum entirely, in `model.yaml`.
- Declaring a new audit event: actor, outcome, severity, fields, message template, and an
  optional `error_code`.
- Declaring a new error code: `http_status` plus a description (no message text — that lives in
  the i18n catalogs).
- Running `python3 scripts/gen_audit_model.py` (and `--check` in CI) to regenerate
  `libs/auditmodel/generated.go` and `apps/web/src/api/generated/audit.ts`, and to validate the
  model.
- Adding the corresponding message entry to every locale in `libs/i18n/locales/` for a new error
  code, and the SPA-side mapping in `apps/web/src/i18n/errorMessages.ts` plus its catalog key in
  both `apps/web/src/i18n/locales/` files.
- The generator's validation rules as user-facing behavior (what fails the build and why),
  since that's what an engineer actually experiences when using this system.
- The collapsing rule: several audit events may legitimately map to one error code (e.g.
  `auth.login.failed` with different `reason` values all surfacing `AUTH_INVALID_CREDENTIALS`),
  and the generator must not treat that as an error.
- Generator assignment of the compact 8-char DB code for every `error_codes`/`events` entry and
  every value under every enum in `enums:` (prefix derivation — from the entry name for error
  codes/events, from the *enum name* for enum values — kind digit, sequence numbering,
  permanence once assigned, and the bidirectional readable↔compact map emitted into both
  generated consumers).

## Out of scope

- The specific set of audit events, enums, and error codes needed for any one domain (auth,
  routines, nutrition, etc.) — those are each domain story's responsibility to declare, using
  this workflow. `AUTH_ACCOUNT_SUSPENDED`'s open enumeration-oracle tension
  (`docs/audit-and-errors.md` §7) belongs to the AUTH epic, not this story.
- A DB-side lookup table for compact codes. Translation is an in-process generated map read by
  the backend service; nothing queries the database directly for the readable form.
- Runtime storage, querying, or retention of audit records (append-only guarantee, ≥90-day
  retention per SEC-07) — this story covers the declare-and-generate workflow, not the audit log
  as a running system.
- Authoring the actual wording of i18n message content — this story covers that a catalog entry
  is *required* and *checked*, not what it should say.
- Rendering error codes in the SPA UI (toast placement, form-field association, etc.) — covered
  by whichever feature story triggers the error.

## Dependencies / assumptions

- `scripts/gen_audit_model.py` exists and is runnable locally and in CI with `--check`.
- `scripts/check_i18n_parity.py` independently verifies catalog coverage between
  `libs/i18n/locales/` and `apps/web/src/i18n/locales/` (coverage only — the two are not content
  copies).
- `docs/backend.md`'s expand/contract migration discipline governs how an enum value addition
  reaches the database schema, where applicable.
- `docs/i18n-guidelines.md` governs the two locale catalogs' own rules (en-US, vi-VN).
- Assumes the reader already knows *why* this model exists (`docs/audit-and-errors.md` §1) —
  this story is about the mechanics of using it correctly, not re-arguing the design.

## Open questions

- None outstanding for the generator workflow itself. The one open design question in this
  area — how `AUTH_ACCOUNT_SUSPENDED` should behave given the enumeration-oracle conflict — is
  tracked in `docs/audit-and-errors.md` §7 and belongs to the AUTH epic, not here.
- What happens if a (prefix, kind) pair exceeds 999 sequence numbers is unspecified — not
  expected to occur at this project's scale, but the generator should have *some* defined
  failure mode (hard error vs. widening to 4 digits) rather than silently wrapping or
  colliding. Left for whoever implements the generator's compact-code assignment logic.
