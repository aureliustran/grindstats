# Executor brief: db-migrations

**Slice ID:** `db-migrations`
**Story / spec:** [AUTH-001](../../AUTH-001-auth-registration/story.md) ·
[AUTH-002](../../AUTH-002-auth-login/story.md) ·
[AUTH-003](../../AUTH-003-auth-session-refresh-logout/story.md)
**Domain:** migration
**Depends on:** none — may start immediately
**Load skill:** `multi-agent-execution` (`.claude/skills/multi-agent-execution/SKILL.md`)
**Read first:** [`docs/backend.md`](../../../backend.md) §3 (all of it) ·
[`docs/shared-contract.md`](../../../shared-contract.md) · [`../contract.md`](../contract.md) §4

## Task

Create the entire schema for run 1 as numbered migration pairs, plus the separate binary that
applies them.

**You are the only writer of migrations in this run.** Migration files are globally ordered by
their number; if a second agent also created `0001_`, one of them would silently lose or the
merge would leave someone's local database half-applied. That is why the four slices that need
these tables are waiting on you rather than each writing their own DDL.

## You own (exclusive write access)

- `infra/db/migrations/**`
- `infra/db/README.md`
- `services/monolith/cmd/migrate/**`

## Read-only context

- `docs/stories/auth-epic/contract.md` §4 — the DDL, verbatim; §3 — what lives in Redis instead
  (so you can see what deliberately has no table)
- `docs/backend.md` §3 (single-writer, expand/contract, rollback, no startup migration) and
  §2 (schema-per-domain namespacing)
- `docs/audit-and-errors.md` §6 — why enum columns are `char(8)` and not text
- `services/monolith/internal/platform/db/db.go` — the existing pgx pool constructor. Reuse it;
  do not modify it.
- `docker-compose.yml` — Postgres service name, credentials, and the `DATABASE_*` env names

## Do not touch

- `services/monolith/internal/platform/db/**` — GATE-001's, unchanged this run
- `docker-compose.yml`, `.env.example` — owned by `be-wiring`; if the migrate binary needs a new
  env var, name it in your README and your report, and `be-wiring` wires it
- `services/monolith/cmd/server/**` — owned by `be-wiring`
- `go.mod` / `go.sum` — owned by `be-wiring`, and every module you need is already there
  (pgx is present; the runner uses plain SQL, no migration library)
- Any file under `services/monolith/internal/auth/` — later slices own those

## Contract you implement against

[`../contract.md`](../contract.md) §4, reproduced in full there: schemas `auth` and `audit`, and
tables `auth.users`, `auth.oauth_identities`, `auth.link_tokens`, `audit.records` with the exact
columns, types, constraints and indexes given. Implement it as written. Points that are easy to
get subtly wrong:

- **`email_lower` carries the UNIQUE constraint**, not `email`. That is FR-01's case-insensitive
  uniqueness, and it is why `Taken@Example.com` and `taken@example.com` collide.
- **`password_hash` is nullable** — an OAuth-only account has no password, and a `NOT NULL ''`
  would make "has no password" indistinguishable from "has an empty one".
- **Every enum-typed column is `char(8)`**, storing the generated compact code
  (`audit-and-errors.md` §6), never the readable value. Your migration does not need to know
  what the codes are; it needs to reserve the right width and type.
- **`audit.records` is append-only.** Add a migration step granting the application's DB role
  `INSERT, SELECT` on it and **not** `UPDATE, DELETE` (FR-45). If the compose setup uses a
  superuser and that grant is therefore inert locally, say so in the README rather than skipping
  it — the statement documents the intent and is what a real deployment role inherits.
- **`status` and `role` are `NOT NULL`** with no default. The application supplies the compact
  code; a database-level default would be a second place that decides what role a new account
  gets, which is exactly what FR-04 forbids.

### The runner

`services/monolith/cmd/migrate` — a small binary, not a library, with three subcommands:

```
migrate up        apply every unapplied migration, in order, each in its own transaction
migrate down      roll back exactly one migration
migrate status    list applied/pending
```

- It tracks applied versions in its own table (`public.schema_migrations` or equivalent — your
  choice, documented in the README).
- It reads the DSN from the same env the app does (`internal/platform/config` names them; read
  that file).
- **Nothing applies migrations at application startup** (`backend.md` §3). If you find yourself
  adding a call in `main.go`, stop — that file is not yours and the rule is deliberate.
- Every `.up.sql` has a `.down.sql` that actually reverses it, or a comment at the top of the
  down file stating why it cannot and what a real rollback would require.

## Acceptance-criteria scenarios this slice covers with automated tests

No user-facing scenario is observable in a migration. What you test instead — required, and the
form `backend.md` §3 asks for:

| What | Kind | Location |
|---|---|---|
| every migration applies cleanly to an empty database, then rolls back cleanly | integration, env-gated on `GRINDSTATS_TEST_DB` | `services/monolith/cmd/migrate/migrate_test.go` |
| `up` is idempotent — running it twice applies nothing the second time | integration, same gate | ” |
| `email_lower` rejects a case-variant duplicate | integration, same gate | ” |

Gate them behind `GRINDSTATS_TEST_DB` so `go test ./...` stays fast and green without a database
(`backend.md` §7).

## Done when

- [ ] Every table, column, constraint and index in contract §4 exists, spelled as written
- [ ] Each migration is a numbered `.up.sql` / `.down.sql` pair, sequential, never edited after
      this slice is reported done
- [ ] `cmd/migrate` supports `up`, `down`, `status`, and applies nothing implicitly
- [ ] The three tests above pass with a database:
      `GRINDSTATS_TEST_DB=... go test ./services/monolith/cmd/migrate/...`
- [ ] And the suite is green without one: `go test ./...`
- [ ] Builds: `go build ./... && go vet ./...`
- [ ] `infra/db/README.md` says how to run the runner locally, what env it reads, and names any
      env var `be-wiring` must add
- [ ] Nothing modified outside the allowlist above (check your own diff)

## Stop condition — read this before starting

If you find the contract wrong, ambiguous, or insufficient for this task:

> **Stop work on the affected surface. Write an amendment request using
> `.claude/skills/multi-agent-execution/assets/amendment-request-template.md`. Do not proceed on
> that surface.**

Do not fix the contract yourself, and do not implement around it. Other slices are being built
against the current version right now; a local fix produces two implementations that each
believe they conformed, and nothing detects the mismatch until runtime.

Adding a column because it "will obviously be needed" is the specific version of this that
matters here: four slices are writing repository code against exactly the columns in §4, and a
column you added is a column nobody writes to.

## Report back

Write the report to `docs/stories/auth-epic/reports/db-migrations.md`.

1. **Changed:** files touched, and what each change does
2. **Verified:** commands run and their actual output — including whether you had a database
   available for the gated tests, and if not, exactly which assertions went unrun
3. **Could not do:** anything blocked, and why
4. **Noticed:** problems outside your scope. Report them, don't fix them.
