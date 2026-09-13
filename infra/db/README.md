# Database migrations

Migration files for the GrindStats monolith live here.  Every file is a
numbered `.up.sql` / `.down.sql` pair.  They are applied and rolled back by
the `cmd/migrate` binary — never automatically at application startup
(`docs/backend.md §3`).

---

## File naming

```
NNNN_<slug>.up.sql
NNNN_<slug>.down.sql
```

`NNNN` is a zero-padded sequential integer.  The runner sorts by filename and
applies migrations in ascending order.  **Never edit a migration file after
it has been applied to any shared environment** — a mistake is corrected by a
new migration, not by rewriting history.

---

## Running the migrate binary

Build and run from the **repository root**:

```sh
# build
go build -o migrate ./services/monolith/cmd/migrate

# apply all pending migrations
./migrate up

# roll back the last migration
./migrate down

# show applied / pending status
./migrate status
```

The binary resolves `infra/db/migrations` relative to the working directory,
so running it from the repository root requires no extra configuration.  Set
`MIGRATIONS_DIR` to an absolute path if you invoke the binary from elsewhere:

```sh
MIGRATIONS_DIR=/path/to/repo/infra/db/migrations ./migrate up
```

---

## Environment variables

| Variable | Default | Notes |
|---|---|---|
| `POSTGRES_HOST` | `localhost` | |
| `POSTGRES_PORT` | `5433` | compose maps container 5432 → host 5433 |
| `POSTGRES_USER` | `grindstats` | |
| `POSTGRES_PASSWORD` | `grindstats` | |
| `POSTGRES_DB` | `grindstats` | |
| `POSTGRES_SSLMODE` | `disable` | set to `require` in production |
| `MIGRATIONS_DIR` | `infra/db/migrations` (relative to cwd) | override when not running from repo root |
| `GRINDSTATS_TEST_DB` | _(unset)_ | raw DSN used by integration tests only; takes precedence over all `POSTGRES_*` vars |

The first six variables (`POSTGRES_*`) are shared with the application server
and are already wired in `docker-compose.yml`.  **`MIGRATIONS_DIR` is new
and must be added to any deployment runbook that invokes `cmd/migrate` from a
working directory other than the repository root.**

> **`be-wiring` action required:** add `MIGRATIONS_DIR` to the deployment
> runbook / CI job that runs migrations, or ensure that job runs from the
> repository root so the default resolves correctly.

---

## Tracking table

Applied versions are tracked in `public.schema_migrations`:

```sql
CREATE TABLE IF NOT EXISTS public.schema_migrations (
    version    text        NOT NULL PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
);
```

The `migrate` binary creates this table on first run if it does not exist.
The `version` value is the filename base without `.up.sql`
(e.g. `0001_auth_and_audit_schema`).

---

## Running against Docker Compose locally

```sh
docker compose up -d postgres
go build -o migrate ./services/monolith/cmd/migrate
./migrate status   # should show "pending  0001_auth_and_audit_schema"
./migrate up       # applies migration 0001
./migrate status   # should show "applied  0001_auth_and_audit_schema  <timestamp>"
```

---

## Integration tests

The tests in `services/monolith/cmd/migrate/migrate_test.go` require a live
Postgres instance and are skipped automatically when `GRINDSTATS_TEST_DB` is
not set, so `go test ./...` stays fast without a database.

To run them:

```sh
docker compose up -d postgres

GRINDSTATS_TEST_DB="host=localhost port=5433 user=grindstats password=grindstats dbname=grindstats sslmode=disable" \
  go test ./services/monolith/cmd/migrate/...
```

Each test cleans up after itself by dropping the `auth` and `audit` schemas
and the `public.schema_migrations` table before and after each run.

---

## Migration history

| Version | Description |
|---|---|
| `0001_auth_and_audit_schema` | `auth` and `audit` schemas for the auth epic (AUTH-001, AUTH-002, AUTH-003) |
