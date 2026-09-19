# Executor report: be-wiring (wave 4)

**Slice:** `be-wiring`
**Branch:** `auth-epic-execution`
**Worktree:** `D:\PROJECTS\grindstats\.claude\worktrees\agent-a2b81e60b916fb1ad`
**Executor date:** 2026-09-14

---

## 1. Changed

| File | What it does |
|---|---|
| `services/monolith/cmd/server/main.go` | Wires the full auth stack: loads RS256 key set, constructs the four repositories (`AccountStore`, `OAuthIdentityStore`, `LinkTokenStore`, `auditlog.PgxWriter`), passes all auth deps into `gateway.New`, constructs `session.Handler`, `credentials.Handler` (with `PasswordHasher`, `hibp.HTTPClient`, `mailer`), `credentials.Provisioner`, `oauth.Handler`, and calls each handler's `Register(r)` on `srv.V1`. See "Noticed" for the AMD-005 session routing issue. |
| `services/monolith/internal/platform/config/config.go` | Added `Auth` config block with fields for `JWTPrivateKeyPath`, `JWTPublicKeysDir`, `CookieSecure`, `HIBPEnabled`, `GoogleClientID/Secret/RedirectURL`, `GoogleStateKey` (hex-decoded from env), `MailerMode`, `BaseURL`. Added `getEnvBool` helper. |
| `services/monolith/internal/platform/config/config_test.go` | Extended tests to cover new Auth defaults, bool overrides, and hex-decode of `AUTH_GOOGLE_STATE_KEY`. |
| `.env.example` | Added names for all new `AUTH_*` environment variables (values left blank — names only, never real values). |
| `.gitignore` | Added `.local/` entry to git-ignore the generated RS256 keypair (SEC-06). |
| `docker-compose.yml` | Added `server` service with full env wiring for all `AUTH_*` variables; `AUTH_COOKIE_SECURE=false` by default for plain-http local dev; volume mount for `.local/jwt → /run/secrets/jwt`. |
| `scripts/dev/gen-jwt-keys.sh` | Git Bash script that generates a 4096-bit RSA keypair into `.local/jwt/` (git-ignored). Derives the `kid` from a SHA-256 fingerprint of the public key's DER encoding. Prints the env var values to set. |
| `docs/stories/auth-epic/amendments/AMD-005-session-register-split.md` | Amendment request for `session.Handler.Register` mixing public and protected routes (see "Noticed"). |
| `services/monolith/internal/auth/integration/enumeration_test.go` | TC-16 enumeration test (gated on `//go:build integration`). Tests `POST /auth/register` with taken vs. new email (byte-identical assert) and `POST /auth/login` with known+wrong vs. unknown email (byte-identical assert). Uses in-memory fakes — no live DB required. |
| `services/monolith/internal/auth/integration/e2e_test.go` | SRS-AUTH-001 §6 acceptance criteria 1–4 as a runnable e2e test (gated on `//go:build integration` + `GRINDSTATS_TEST_DB`). Stages blocked by AMD-005 are annotated but written in full. Also contains route-registration smoke tests. |

---

## 2. Verified

### Build, vet, test (ungated)

```
go build ./...
```
Output: (no output — clean)

```
go vet ./...
```
Output: (no output — clean)

```
go test -timeout 180s ./...
```
Output:
```
ok  	grindstats/libs/auditlog	2.303s
?   	grindstats/libs/auditmodel	[no test files]
ok  	grindstats/libs/authmw	2.523s
ok  	grindstats/libs/httpkit	2.151s
ok  	grindstats/libs/i18n	2.229s
ok  	grindstats/services/monolith/cmd/migrate	0.737s
?   	grindstats/services/monolith/cmd/server	[no test files]
?   	grindstats/services/monolith/internal/auth/authdomain	[no test files]
ok  	grindstats/services/monolith/internal/auth/credentials	0.586s
ok  	grindstats/services/monolith/internal/auth/hibp	1.037s
?   	grindstats/services/monolith/internal/auth/mailer	[no test files]
ok  	grindstats/services/monolith/internal/auth/oauth	0.408s
ok  	grindstats/services/monolith/internal/auth/session	3.645s
ok  	grindstats/services/monolith/internal/auth/store	0.324s
ok  	grindstats/services/monolith/internal/gateway	0.394s
ok  	grindstats/services/monolith/internal/gateway/health	0.422s
ok  	grindstats/services/monolith/internal/gateway/middleware	7.781s
ok  	grindstats/services/monolith/internal/platform/cache	2.374s
ok  	grindstats/services/monolith/internal/platform/config	0.360s
ok  	grindstats/services/monolith/internal/platform/db	0.733s
```
All 19 packages green. Integration package excluded (no build tag).

### Integration suite (with build tag, no live DB)

```
go test -tags=integration -timeout 120s -v ./services/monolith/internal/auth/integration/...
```
Output:
```
--- SKIP: TestE2E_AuthLifecycle (0.00s)
    e2e_test.go:277: GRINDSTATS_TEST_DB not set; skipping e2e test
--- PASS: TestRouteRegistration_PublicEndpointsAreReachable (0.07s)
    --- PASS: .../POST_/api/v1/auth/register (0.00s)
    --- PASS: .../POST_/api/v1/auth/verify-email (0.00s)
    --- PASS: .../POST_/api/v1/auth/password-reset/request (0.00s)
    --- PASS: .../POST_/api/v1/auth/password-reset/confirm (0.00s)
    --- PASS: .../POST_/api/v1/auth/oauth/link/confirm (0.00s)
    --- PASS: .../POST_/api/v1/auth/login (0.00s)
    --- PASS: .../POST_/api/v1/auth/refresh (0.00s)
--- PASS: TestRouteRegistration_AuthenticatedEndpointsReturn401WithoutToken (0.05s)
    --- PASS: .../POST_/api/v1/auth/logout (0.00s)
    --- PASS: .../POST_/api/v1/auth/logout-all (0.00s)
    --- PASS: .../GET_/api/v1/users/me (0.00s)
--- PASS: TestEnumeration_RegisterTakenAndNewAddressAreByteIdentical (0.05s)
--- PASS: TestEnumeration_LoginUnknownEmailAndWrongPasswordAreByteIdentical (0.08s)
PASS
ok  	grindstats/services/monolith/internal/auth/integration	0.414s
```

### Audit model generator

```
py scripts/gen_audit_model.py --check
```
Output:
```
OK: model valid and generated files current (17 enums, 12 error codes, 19 events)
```

### Scenario → test → result

| Scenario | Test name | Result |
|---|---|---|
| TC-16: register with taken address vs. new address — byte-identical | `TestEnumeration_RegisterTakenAndNewAddressAreByteIdentical` | PASS |
| TC-16: login wrong-pw known-email vs. unknown-email — byte-identical | `TestEnumeration_LoginUnknownEmailAndWrongPasswordAreByteIdentical` | PASS |
| Route: public endpoints reachable (non-404) | `TestRouteRegistration_PublicEndpointsAreReachable` | PASS (7 sub-tests) |
| Route: protected endpoints return 401 without token (non-404) | `TestRouteRegistration_AuthenticatedEndpointsReturn401WithoutToken` | PASS (3 sub-tests) |
| E2E: full session lifecycle (SRS-AUTH-001 §6 AC 1–4) | `TestE2E_AuthLifecycle` | SKIP (no GRINDSTATS_TEST_DB; blocked stages also noted — see Could Not Do) |

---

## 3. Could not do

### Live DB / Redis — GRINDSTATS_TEST_DB unset

Docker Desktop's daemon is not running on this machine; no live Postgres or Redis is reachable. `TestE2E_AuthLifecycle` skips rather than running. TC-16 and route tests use in-memory fakes and miniredis, so they run and pass without a database.

The e2e test is written in full and compiles cleanly. It will run when `GRINDSTATS_TEST_DB` is set and a Redis is reachable. Results against live infrastructure are unverified.

### E2E stages blocked by AMD-005

Even with live infrastructure, the following stages of `TestE2E_AuthLifecycle` will fail until AMD-005 is resolved, because `session.Handler.Register` places logout/logout-all/users/me on V1 (no auth middleware):

- Stage 4: `GET /api/v1/users/me` — returns 401 instead of 200 with user profile
- Stage 7: CSRF enforcement test (logout without/with CSRF header) — skipped
- Stage 8: Logout and token invalidation verification — skipped

These stages are annotated in `e2e_test.go` with "BLOCKED: AMD-005".

### Binary startup with real keys

The binary was not started against compose (no running Docker daemon). `go build ./...` proves the binary compiles; the startup path (key loading, DB/Redis connections) was not verified against live dependencies. The GATE-001 behavior (server starts even when DB/Redis are down at boot) is unchanged — only a malformed DSN or missing key files cause an `os.Exit(1)` at startup.

### Frontend checks (not re-run — pre-step already complete)

`npx vitest run`, `npx tsc --noEmit`, `npm run check-i18n`, and `npm run build` for `apps/web` were not re-run. The pre-step report confirms they were verified by the instructor before wave 1 was dispatched.

---

## 4. Noticed

### AMD-005: session.Handler.Register mixes public and protected routes

`session.Handler.Register(r gin.IRouter)` registers login, refresh (public) and logout, logout-all, users/me (protected) on a single router group. There is no mechanism in the current API to split them.

The gateway's `Protected` group applies auth → CSRF → verified-write middleware. Calling `sessionHandler.Register(srv.Protected)` would break login (auth middleware rejects requests without an access cookie before the handler runs). Calling `sessionHandler.Register(srv.V1)` makes logout/logout-all/users/me always return 401 (handlers check `authmw.ClaimsFromContext` which is only populated by auth middleware). Calling `Register` twice on different groups causes gin to panic on duplicate routes.

The workaround: all session routes are registered on `srv.V1`. Login and refresh work correctly. Logout, logout-all, and users/me return 401 for all callers (including those with valid tokens) because no auth middleware runs. An amendment (`AMD-005`) has been filed; when implemented, the composition root will call `RegisterPublic(srv.V1)` and `RegisterProtected(srv.Protected)`.

**Owning slice:** `be-auth-session`

### Gateway Deps: AuditLog field in panic condition

`gateway.New` panics when `KeySet != nil && Redis != nil && AuditLog == nil` — the panic check is `authProvided && !authComplete` where `authComplete = KeySet != nil && Redis != nil`. This means a nil AuditLog with non-nil KeySet+Redis does NOT panic (AuditLog is optional in the check). In practice this is fine since `auditlog.New(pool, logger)` always succeeds and the production path always provides it, but the check could be made more explicit if desired.

### No Dockerfile for the server service

`docker-compose.yml` adds a `server` service that references `services/monolith/Dockerfile`, but no such Dockerfile exists in the repo. The service definition is included for completeness, but `docker compose up` will fail on building the server container until a Dockerfile is added. This is out of scope for the auth epic but may block manual end-to-end verification.

### be-gateway-authz report not present

`docs/stories/auth-epic/reports/be-gateway-authz.md` does not exist in the merged branch. The gateway-authz slice's work (middleware files) is present in the codebase (files are committed), but the report was not filed. Phase 3 testers should note this: the slice delivered code but not a completion report.

### Self-heal used: git merge origin/auth-epic-execution

The worktree was forked from a stale base. All required files were missing. Self-healed by:
```
git fetch origin auth-epic-execution
git merge origin/auth-epic-execution --no-edit
```
Fast-forward merge succeeded; no conflicts. 126 files added/changed.
