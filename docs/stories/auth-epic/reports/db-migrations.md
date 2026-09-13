# Execution report: db-migrations

**Slice:** `db-migrations`
**Branch:** `worktree-agent-a031ec99359d5b5ba`
**Worktree:** `D:\PROJECTS\grindstats\.claude\worktrees\agent-a031ec99359d5b5ba`
**Date:** 2026-09-13

---

## 1. Changed

All files are new (no existing file was modified).

| File | What it does |
|---|---|
| `infra/db/migrations/0001_auth_and_audit_schema.up.sql` | Creates schemas `auth` and `audit`, tables `auth.users`, `auth.oauth_identities`, `auth.link_tokens`, `audit.records`, three indexes on `audit.records`, one index on `auth.link_tokens(user_id, kind)`, and the `GRANT INSERT, SELECT ON audit.records TO grindstats` statement (FR-45). |
| `infra/db/migrations/0001_auth_and_audit_schema.down.sql` | Drops `audit` schema (CASCADE), then `auth` schema (CASCADE). Uses a `DO` block to revoke the audit grant first, handling a missing role gracefully (test DBs use a superuser). |
| `services/monolith/cmd/migrate/runner.go` | `package main` — `Runner` type with `EnsureTrackingTable`, `Up`, `Down`, `Status` methods. Uses `pgx/v5` directly (no migration library). Each up/down is applied in its own transaction with the tracking-table write/delete inside the same transaction. |
| `services/monolith/cmd/migrate/main.go` | `package main` — entry point. Dispatches `up`/`down`/`status` to `Runner`. Reads DSN from `GRINDSTATS_TEST_DB` (test override) or `config.Load().Postgres.DSN()`. Reads migrations directory from `MIGRATIONS_DIR` env or defaults to `infra/db/migrations` relative to cwd. |
| `services/monolith/cmd/migrate/migrate_test.go` | Integration tests gated on `GRINDSTATS_TEST_DB`; skipped automatically without a database. Three tests (see §2). |
| `infra/db/README.md` | Explains file naming, how to run the binary, all env vars (including the new `MIGRATIONS_DIR`), the tracking table schema, and how to run the integration tests. Names the `be-wiring` action required for `MIGRATIONS_DIR`. |

**Tracking table:** `public.schema_migrations (version text PK, applied_at timestamptz NOT NULL DEFAULT now())` — created at first run by `EnsureTrackingTable`.

**Diff check:** `git status` shows only `infra/` and `services/monolith/cmd/migrate/` as new untracked trees. No file outside the allowlist was modified.

---

## 2. Verified

### Build and ungated suite

```
$ go build ./...
(no output — success)

$ go vet ./...
(no output — success)

$ go test ./...
?   	grindstats/libs/auditmodel	[no test files]
ok  	grindstats/libs/httpkit	0.144s
ok  	grindstats/libs/i18n	0.467s
ok  	grindstats/services/monolith/cmd/migrate	0.799s
?   	grindstats/services/monolith/cmd/server	[no test files]
ok  	grindstats/services/monolith/internal/gateway	0.156s
ok  	grindstats/services/monolith/internal/gateway/health	0.310s
ok  	grindstats/services/monolith/internal/gateway/middleware	0.158s
ok  	grindstats/services/monolith/internal/platform/cache	2.441s
ok  	grindstats/services/monolith/internal/platform/config	0.411s
ok  	grindstats/services/monolith/internal/platform/db	0.808s
```

All 10 packages pass (the migrate package tests skip cleanly without a database).

### Scenario tests

| Scenario | Test name | Result |
|---|---|---|
| Every migration applies cleanly to an empty DB, then rolls back cleanly | `TestMigrationsApplyAndRollbackCleanly` | **SKIPPED** — no database reachable in this worktree (see §3) |
| `up` is idempotent — running it twice applies nothing the second time | `TestUpIsIdempotent` | **SKIPPED** — same |
| `email_lower` rejects a case-variant duplicate | `TestEmailLowerRejectsVariantDuplicate` | **SKIPPED** — same |

---

## 3. Could not do

**Database-gated integration tests were not run.** No live Postgres instance was reachable in this sandboxed worktree. The three tests listed in §2 all call `t.Skip("GRINDSTATS_TEST_DB not set")` when the env var is absent and are therefore not verified by execution.

What went unverified:
- That the `.up.sql` DDL actually parses and executes without error in Postgres 16.
- That the `.down.sql` cleanly reverses the up migration (schemas, tables, indexes all gone).
- That a second `Up` call produces no additional tracking rows (idempotency).
- That the `email_lower` UNIQUE constraint rejects a case-variant insert with `SQLSTATE 23505`.

These must be run by the phase-3 tester against a live database:

```sh
docker compose up -d postgres
GRINDSTATS_TEST_DB="host=localhost port=5433 user=grindstats password=grindstats dbname=grindstats sslmode=disable" \
  go test ./services/monolith/cmd/migrate/...
```

---

## 4. Noticed

1. **`go.mod` declares `go 1.26.1`**, which is not a released version of Go as of the knowledge cutoff (1.24.x was the latest stable). This is outside this slice's allowlist and was not changed. If this causes build failures on developer machines, `be-wiring` should update it.

2. **The `GRANT INSERT, SELECT ON audit.records TO grindstats`** statement in the up migration will fail if a role named `grindstats` does not exist in the test database. The down migration's `DO` block handles missing roles gracefully on rollback, but the up migration does not. For CI test databases that use a superuser account with a different name, this `GRANT` will error. A future migration could wrap the grant in a `DO` block as well, or the CI test DSN user could be named `grindstats`. Reported; not fixed — the contract specifies the grant verbatim.

3. **`runtime.Caller` in `migrationsPath`** is used to resolve the migrations directory relative to the test source file. This is a standard Go test pattern but only works when the test binary is compiled with debugging information. It will work for all normal `go test` invocations. Mentioned in case the tester encounters a trimmed binary.
