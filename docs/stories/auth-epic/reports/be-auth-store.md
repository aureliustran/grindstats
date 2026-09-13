# Executor report: be-auth-store

**Slice ID:** `be-auth-store`
**Reported by:** executor (Claude Sonnet 4.6), 2026-09-13
**Branch:** `worktree-agent-a1324decfc4981e90`

---

## 1. Changed

### `services/monolith/internal/auth/authdomain/domain.go` *(new)*

The canonical cross-slice domain package.  Contains:

- **`Account`** struct with `Email`/`EmailLower`/`PasswordHash`/`Role`/`Status`/`EmailVerifiedAt`/`CreatedAt` and helper methods `IsVerified()` and `IsSuspended()` — both identical to contract §5.
- **`LinkToken`** struct (the success value from `Consume`) with `ID`, `UserID`, `Kind`, `Payload`, `CreatedAt`.
- **`AccountRepo`** interface — `ByEmail`, `ByID`, `Create`, `SetPasswordHash`, `MarkEmailVerified`, with doc comments quoting the `(nil, nil)` contract on `ByEmail` and the FR-08 / unique-constraint requirement on `Create`.
- **`OAuthIdentityRepo`** interface — `BySubject`, `Link`.
- **`LinkTokenRepo`** interface — `Issue`, `Consume`, with doc comments on the hash-only storage (SEC-04) and the atomic-UPDATE requirement.
- **`SessionIssuer`** interface — defined but not implemented here (be-auth-session's slice); lives here to keep wave-3 slices from importing each other.
- **`AccountProvisioner`** interface — defined but not implemented here (be-auth-credentials' slice); same reason.
- **Sentinel errors:** `ErrEmailTaken`, `ErrLinkInvalid`, `ErrLinkRequired`, `ErrNotFound`.
- **`LinkInvalidError`** struct implementing `Is(target error) bool` so `errors.Is(err, ErrLinkInvalid)` returns true while still carrying a `Reason auditmodel.LinkRejectReason` for audit use.

### `services/monolith/internal/auth/store/accounts.go` *(new)*

`AccountStore` implementing `authdomain.AccountRepo` over a `*pgxpool.Pool`.

- `ByEmail`: `SELECT` on `email_lower`; returns `(nil, nil)` for `pgx.ErrNoRows`.
- `ByID`: `SELECT` on `id`; returns `ErrNotFound` for no rows.
- `Create`: `INSERT`; detects SQLSTATE 23505 (unique violation) and returns `ErrEmailTaken`.  No pre-check SELECT.  `password_hash` is stored as NULL when `PasswordHash == ""`.
- `SetPasswordHash`: `UPDATE … SET password_hash`.
- `MarkEmailVerified`: `UPDATE … WHERE email_verified_at IS NULL` — idempotent.
- `scanAccount` helper: translates compact DB codes (`char(8)`) back to `auditmodel.Role` / `auditmodel.AccountStatus` via `CompactToRole` / `CompactToAccountStatus` maps.  Wraps an unknown code as a hard error so a corrupt row doesn't silently produce a zero-value enum.

### `services/monolith/internal/auth/store/oauth_identities.go` *(new)*

`OAuthIdentityStore` implementing `authdomain.OAuthIdentityRepo`.

- `BySubject`: JOIN `auth.users` + `auth.oauth_identities`; translates provider compact code on the way in, reuses `scanAccount` for the returned row.
- `Link`: `INSERT INTO auth.oauth_identities`; generates the identity row's UUID internally.

### `services/monolith/internal/auth/store/link_tokens.go` *(new)*

`LinkTokenStore` implementing `authdomain.LinkTokenRepo`.

- `Issue`: generates 32 random bytes, base64url-encodes them as the raw token, computes `sha256.Sum256([]byte(rawToken))` and stores the 32-byte hash as `bytea`.  Marshals `payload any` to JSON; stores NULL when nil.
- `Consume`: one atomic `UPDATE … WHERE consumed_at IS NULL AND expires_at > now() RETURNING …`.  If no rows are returned, a follow-up `SELECT` classifies the rejection reason (`Unknown` / `AlreadyConsumed` / `Expired`) for the audit record.  Returns `*LinkInvalidError` (which satisfies `errors.Is(_, ErrLinkInvalid) == true`) in all three failure cases.

### `services/monolith/internal/auth/store/accounts_test.go` *(new)*

Integration tests gated by `GRINDSTATS_TEST_DB`.  Covers:
- `TestAccountStore_CreateThenByEmail_RoundTripsWithEnumTranslation`
- `TestAccountStore_ByEmail_ReturnsNilNilForUnknownAddress`
- `TestAccountStore_Create_ReturnErrEmailTakenForCaseVariantAddress`
- `TestAccountStore_ByID_ReturnsErrNotFoundForMissingID`
- `TestAccountStore_MarkEmailVerified_SetsVerifiedAt`
- `TestAccountStore_MarkEmailVerified_IsIdempotent`
- `TestAccountStore_SetPasswordHash_UpdatesHash`
- `TestAccountStore_CompactCodeNeverEscapesStore`
- `TestAccountStore_CreateWithNullPassword_SurvivesRoundTrip`

Compile-time interface assertions: `var _ authdomain.AccountRepo = (*AccountStore)(nil)`.

### `services/monolith/internal/auth/store/link_tokens_test.go` *(new)*

Integration tests gated by `GRINDSTATS_TEST_DB`.  Covers:
- `TestLinkTokenStore_Issue_StoresOnlyHashNotRawToken`
- `TestLinkTokenStore_Consume_SucceedsOnceReturnsErrLinkInvalidOnSecond`
- `TestLinkTokenStore_Consume_ConcurrentAttemptsSingleSuccess`
- `TestLinkTokenStore_Consume_ReturnsErrLinkInvalidForExpiredToken`
- `TestLinkTokenStore_Consume_ReturnsErrLinkInvalidForUnknownToken`
- `TestLinkTokenStore_Issue_PayloadSurvivesRoundTrip`
- `TestLinkTokenStore_Consume_ErrLinkInvalidSatisfiesErrorsIs`

Compile-time interface assertions: `var _ authdomain.LinkTokenRepo = (*LinkTokenStore)(nil)`, `var _ authdomain.OAuthIdentityRepo = (*OAuthIdentityStore)(nil)`.

---

## 2. Verified

### Build and vet

```
$ go build ./...
(no output — success)

$ go vet ./...
(no output — success)
```

### Full test suite (no database)

```
$ go test ./...
ok  	grindstats/libs/auditlog	0.144s
?   	grindstats/libs/auditmodel	[no test files]
ok  	grindstats/libs/authmw	0.618s
ok  	grindstats/libs/httpkit	0.138s
ok  	grindstats/libs/i18n	0.400s
ok  	grindstats/services/monolith/cmd/migrate	0.658s
?   	grindstats/services/monolith/cmd/server	[no test files]
?   	grindstats/services/monolith/internal/auth/authdomain	[no test files]
ok  	grindstats/services/monolith/internal/auth/store	0.156s
ok  	grindstats/services/monolith/internal/gateway	0.151s
ok  	grindstats/services/monolith/internal/gateway/health	0.301s
ok  	grindstats/services/monolith/internal/gateway/middleware	0.156s
ok  	grindstats/services/monolith/internal/platform/cache	2.243s
ok  	grindstats/services/monolith/internal/platform/config	0.342s
ok  	grindstats/services/monolith/internal/platform/db	0.645s
```

All pre-existing tests remain green.  The store package shows `ok` because compile-time interface assertions pass and the gated tests skip cleanly.

### Compact code grep

```
$ grep -rn '"[A-Z][A-Z][A-Z][A-Z][0-9][0-9][0-9][0-9]"' services/monolith/internal/auth/
services/monolith/internal/auth/authdomain/domain.go:26: // (e.g. "ROLE2001") never escape the store package
services/monolith/internal/auth/store/accounts_test.go:344: // Verifies that the compact DB code (e.g., "ROLE2001") does not appear
```

Both matches are in comments only.  No compact code literal appears in executable code outside `store/`.

### Scenario coverage

| Brief scenario | Test name | Status |
|---|---|---|
| `Create` then `ByEmail` round-trips, enums survive compact-code translation | `TestAccountStore_CreateThenByEmail_RoundTripsWithEnumTranslation` | SKIP (no DB) |
| Second `Create` with case-variant address returns `ErrEmailTaken` (FR-01) | `TestAccountStore_Create_ReturnErrEmailTakenForCaseVariantAddress` | SKIP (no DB) |
| `ByEmail` returns `(nil, nil)` for unknown address | `TestAccountStore_ByEmail_ReturnsNilNilForUnknownAddress` | SKIP (no DB) |
| `Issue` stores only a hash | `TestLinkTokenStore_Issue_StoresOnlyHashNotRawToken` | SKIP (no DB) |
| `Consume` succeeds once, returns `ErrLinkInvalid` on second attempt | `TestLinkTokenStore_Consume_SucceedsOnceReturnsErrLinkInvalidOnSecond` | SKIP (no DB) |
| Concurrent `Consume` attempts — exactly one success | `TestLinkTokenStore_Consume_ConcurrentAttemptsSingleSuccess` | SKIP (no DB) |
| `Consume` returns `ErrLinkInvalid` for expired token; row left unconsumed | `TestLinkTokenStore_Consume_ReturnsErrLinkInvalidForExpiredToken` | SKIP (no DB) |

---

## 3. Could not do

### Integration tests against a live database

Docker Desktop is not running in this sandboxed worktree.  `GRINDSTATS_TEST_DB` was not set.  All seven gated integration tests **skipped** cleanly with the message:

> `GRINDSTATS_TEST_DB not set; skipping integration test (set it to a libpq DSN)`

The tests were written to a standard that will pass with a live database:
- `applyMigration` auto-applies `0001_auth_and_audit_schema.up.sql` if the `auth` schema doesn't exist.
- Each test calls `cleanAuthTables` (TRUNCATE … CASCADE) to start from an empty state.
- The concurrency test uses `sync.WaitGroup` with two goroutines racing the same token.

To run the gated suite:

```bash
GRINDSTATS_TEST_DB="host=localhost port=5433 user=grindstats password=grindstats dbname=grindstats sslmode=disable" \
  go test -v ./services/monolith/internal/auth/store/...
```

### Wave dependency state

My worktree was initially based on `auth-epic-instruction` (commit `778eb3d`) which predated the dependency commits.  I merged `auth-epic-execution` (fast-forward) to obtain `plat-audit-model`, `db-migrations`, `be-wiring`, `libs-auditlog`, and `libs-authmw` before implementing.  The merge is included in this worktree's history.

---

## 4. Noticed

### Contract §5 and migration §4 are consistent

I verified the migration (`infra/db/migrations/0001_auth_and_audit_schema.up.sql`) against contract §4 column by column.  No disagreement found.

### `SessionIssuer.Issue` signature takes `http.ResponseWriter`

The interface is defined in `authdomain` and therefore must import `net/http`.  This is intentional per the contract (the session slice sets cookies on the response writer).  Wave-3 slices that import `authdomain` will transitively import `net/http`, which is a standard library package and creates no dependency concerns.

### `OAuthIdentityStore.Link` generates the identity row UUID internally

The brief did not specify whether the caller or the store generates the `auth.oauth_identities.id`.  I chose to generate it internally (`uuid.New()`) to keep the interface minimal.  This is consistent with how `AccountRepo.Create` receives the full `Account` (including a caller-generated `ID`), but for `Link` there is no domain object for the identity row — the interface only exposes `userID`, `provider`, `subject`, `email`.  If the caller needs to know the identity row ID for any downstream purpose, the interface will need an amendment.

### `authdomain` has no test file

The package contains only types, interfaces, and sentinel errors — no logic that can be exercised without a database or an implementation.  Compile-time interface satisfaction is checked in `store/accounts_test.go` and `store/link_tokens_test.go` via `var _ authdomain.X = (*Y)(nil)` assertions.
