# Auth epic — execution plan (run 1)

**Run:** 1 of 2 planned for the epic.
**Instructor:** this session, 2026-09-07.
**Contract:** [`contract.md`](contract.md), **frozen** at commit `6eee4cd`
(`deployment-doc-reconciliation`, the tip `dev` was branched from). Any change to it from here
follows `docs/shared-contract.md` §4 and bumps this line.
**Branch policy:** every slice branches off `dev` and merges back into `dev` by PR. `main` is
production; nothing in this run targets it.
**Scope:** AUTH-001, AUTH-002, AUTH-003 + FR-20..24 + `GET /users/me` + the SPA's real HTTP
client (decision D1). AUTH-004 and AUTH-005 are run 2.

---

## 1. Roadmap check

Blueprint §20 Phase 2 ("real auth"), which is where this epic belongs. What that permits, and
what it does not:

- **One Gin binary**, `services/monolith/`, internal packages named after future services.
  No second binary, no service-to-service HTTP, no RabbitMQ (`backend.md` §1).
- **One Vite + React SPA**, feature folders. No module federation, no shell app
  (`frontend.md` §1).
- Postgres + Redis, both already in `docker-compose.yml`. No Elasticsearch, no S3.
- The boundaries in this plan are **package and folder rules**, which is what makes the Phase-10
  extraction possible later. A slice that reaches across one is the failure this partition
  exists to prevent — not a shortcut that happens to compile.

---

## 2. Slices

Eleven slices, four waves. A slice is dispatched only when every slice it depends on has
reported done and its report has been checked against gate 2.

| # | Slice ID | Wave | Domain | Depends on |
|---|---|---|---|---|
| 1 | `plat-audit-model` | 1 | platform | — |
| 2 | `db-migrations` | 1 | migration | — |
| 3 | `libs-authmw` | 1 | backend lib | — |
| 4 | `libs-auditlog` | 1 | backend lib | — |
| 5 | `fe-auth-client` | 1 | frontend | — |
| 6 | `be-auth-store` | 2 | backend | 1, 2 |
| 7 | `fe-auth-flows` | 2 | frontend | 1, 5 |
| 8 | `be-auth-credentials` | 3 | backend | 3, 4, 6 |
| 9 | `be-auth-session` | 3 | backend | 3, 4, 6 |
| 10 | `be-gateway-authz` | 3 | backend | 1, 3, 4 |
| 11 | `be-wiring` | 4 | backend | all of the above |

**Why the waves fall where they do.** Wave 1 is everything that has no dependency of its own and
that others consume: the generated model, the schema, the token/Redis library, the audit writer,
and the SPA client (which codes against the frozen HTTP contract, not against any Go code, and
so does not wait for the backend at all). Wave 2 is the single seam that wave 3 divides along —
`authdomain`'s types and interfaces plus the repositories behind them. **The three wave-3 slices
never import each other**; they share only `authdomain`, and their concrete implementations meet
in wave 4's composition root. That is the entire reason `authdomain` is its own earlier slice
rather than a file inside whichever slice happened to need it first.

### Slice detail

#### 1. `plat-audit-model` — the declare-once model and its generated consumers

Adds the enums, error codes and events in contract §7, regenerates both consumers, and adds the
server-side message catalog entries and the SPA's code→key mappings. **No other slice may edit
`model.yaml` or a generated file** (`CLAUDE.md`, `audit-and-errors.md` §5); everything else in
this run imports what this slice generates.

#### 2. `db-migrations` — every DDL statement in the run, single-writer

Contract §4, as numbered `.up.sql` / `.down.sql` pairs, plus `cmd/migrate`. Sole owner because
migration files are globally ordered by number and two parallel authors both write `0001_` and
one silently loses (`backend.md` §3).

#### 3. `libs-authmw` — tokens, Redis auth state, and nothing that knows about HTTP handlers

RS256 mint/verify with `kid` rotation, the claim set, the four-step check chain as a pure
function, and typed accessors for every Redis key in contract §3, tested against miniredis so
NFR-06's "runnable without Docker" holds.

#### 4. `libs-auditlog` — the append-only writer

Takes a generated event plus its fields, validates them against the generated spec, translates
enum values to compact codes, renders the en-US message, and inserts one `audit.records` row.
Never localizes. Ships a `Writer` interface and an in-memory fake, because every wave-3 slice
asserts "and an audit record was written" and none of them should need a database to do it.

#### 5. `fe-auth-client` — the real HTTP client behind the frozen `AuthApi`

Replaces the LAND-001 mock as the default implementation. Envelope unwrapping (D5), the CSRF
header, `Accept-Language`, `credentials: "include"`, and the single-flight reactive refresh in
contract §8.2. Tests mock `fetch` and assert against contract shapes; no component changes,
because nothing outside `api/` ever knew which implementation it was calling.

#### 6. `be-auth-store` — `authdomain` and the repositories

Transcribes contract §5 into Go and implements `AccountRepo`, `OAuthIdentityRepo` and
`LinkTokenRepo` over pgx. The interfaces exist so wave 3 can be three parallel slices; the
repositories exist so wave 3 writes no SQL.

#### 7. `fe-auth-flows` — verify-email, password reset, unverified banner, shell wiring

The screens AUTH-001 needs that LAND-001's modal does not cover, their routes, and the SPA
catalogs. Owns `app/` this run, so `AuthContext` learns about the unverified state and the
refresh-driven session lifecycle in one place.

#### 8. `be-auth-credentials` — AUTH-001's backend

Register, verify-email, both password-reset halves, the OAuth authorize/callback pair, argon2id
hashing, the HIBP check (D3), the emailed-link lifecycle, and `AccountProvisioner`. The
enumeration-neutral responses live here and are the slice's most important tests.

#### 9. `be-auth-session` — AUTH-002 and AUTH-003's backend

Login, CSRF issuance, the failure counter and backoff, refresh with rotation and replay
detection, logout, logout-all, the session record, and `/users/me` (D6). Implements
`SessionIssuer` for the OAuth callback to use.

#### 10. `be-gateway-authz` — FR-20..24 as middleware

The five reserved stages GATE-001 left empty, in the order it reserved them, plus the
Redis-degradation policy. Owns `gateway.go` this run because filling those stages is the change
that file exists to receive.

#### 11. `be-wiring` — the composition root and the end-to-end proof

`main.go`, config, local key generation, compose and `.env.example`, and the integration tests
that span slices — including TC-16, which compares two endpoints owned by two different slices
and therefore belongs to neither.

---

## 3. Ownership map

Every path in the run, exactly once. Gate 1 greps this list for duplicates rather than
eyeballing it.

```
libs/auditmodel/model.yaml                                  → plat-audit-model
libs/auditmodel/generated.go                                → plat-audit-model
libs/i18n/locales/en-US.json                                → plat-audit-model
libs/i18n/locales/vi-VN.json                                → plat-audit-model
apps/web/src/api/generated/audit.ts                         → plat-audit-model
apps/web/src/i18n/errorMessages.ts                          → plat-audit-model
scripts/gen_audit_model.py                                  → plat-audit-model

infra/db/migrations/**                                      → db-migrations
infra/db/README.md                                          → db-migrations
services/monolith/cmd/migrate/**                            → db-migrations

libs/authmw/**                                              → libs-authmw
libs/auditlog/**                                            → libs-auditlog

apps/web/src/api/auth.ts                                    → fe-auth-client
apps/web/src/api/auth.types.ts                              → fe-auth-client
apps/web/src/api/http.ts                                    → fe-auth-client
apps/web/src/api/mock/**                                    → fe-auth-client
apps/web/src/api/*.test.ts                                  → fe-auth-client

services/monolith/internal/auth/authdomain/**               → be-auth-store
services/monolith/internal/auth/store/**                    → be-auth-store

apps/web/src/features/auth/**                               → fe-auth-flows
apps/web/src/app/**                                         → fe-auth-flows
apps/web/src/i18n/locales/en-US.json                        → fe-auth-flows
apps/web/src/i18n/locales/vi-VN.json                        → fe-auth-flows

services/monolith/internal/auth/credentials/**              → be-auth-credentials
services/monolith/internal/auth/oauth/**                    → be-auth-credentials
services/monolith/internal/auth/hibp/**                     → be-auth-credentials
services/monolith/internal/auth/mailer/**                   → be-auth-credentials

services/monolith/internal/auth/session/**                  → be-auth-session

services/monolith/internal/gateway/gateway.go               → be-gateway-authz
services/monolith/internal/gateway/gateway_test.go          → be-gateway-authz
services/monolith/internal/gateway/middleware/auth.go       → be-gateway-authz
services/monolith/internal/gateway/middleware/auth_test.go  → be-gateway-authz
services/monolith/internal/gateway/middleware/csrf.go       → be-gateway-authz
services/monolith/internal/gateway/middleware/csrf_test.go  → be-gateway-authz
services/monolith/internal/gateway/middleware/role.go       → be-gateway-authz
services/monolith/internal/gateway/middleware/role_test.go  → be-gateway-authz
services/monolith/internal/gateway/middleware/verified.go   → be-gateway-authz
services/monolith/internal/gateway/middleware/verified_test.go → be-gateway-authz
services/monolith/internal/gateway/middleware/ratelimit.go  → be-gateway-authz
services/monolith/internal/gateway/middleware/ratelimit_test.go → be-gateway-authz

services/monolith/cmd/server/main.go                        → be-wiring
services/monolith/internal/platform/config/config.go        → be-wiring
services/monolith/internal/platform/config/config_test.go   → be-wiring
services/monolith/internal/auth/integration/**              → be-wiring
docker-compose.yml                                          → be-wiring
.env.example                                                → be-wiring
scripts/dev/**                                              → be-wiring
go.mod / go.sum                                             → be-wiring (see below)
apps/web/package.json / package-lock.json                   → be-wiring (see below)
```

**Dependency files are a shared-file hazard.** `go.mod`, `go.sum`, `package.json` and
`package-lock.json` are touched by any slice that adds a library, and every such slice will
conflict on them. They are assigned to `be-wiring`, and the dependency set is **decided now**
so no slice needs to add one:

| Need | Library | Added by |
|---|---|---|
| JWT RS256 | `github.com/golang-jwt/jwt/v5` | `be-wiring`, before wave 1 dispatch |
| argon2id | `golang.org/x/crypto/argon2` (already an indirect dep) | ” |
| UUIDs | `github.com/google/uuid` | ” |
| Redis test double | `github.com/alicebob/miniredis/v2` | ” |
| OAuth2 + PKCE | `golang.org/x/oauth2` | ” |
| Migrations runner | none — `cmd/migrate` runs plain SQL through pgx | — |
| Frontend | nothing new; Vitest + RTL are already configured | — |

`be-wiring` therefore does one thing **before** wave 1 (add the modules, commit a building
`go.mod`/`go.sum`) and the rest of its slice in wave 4. That pre-step is the only work that
happens outside its wave, and it is listed in its brief.

**Explicitly not owned by anyone this run — do not modify:**
`apps/web/src/features/landing/**`, `apps/web/src/styles/**`,
`apps/web/src/i18n/voice-critical-keys.json`, `libs/httpkit/**`, `libs/i18n/i18n.go`,
`services/monolith/internal/gateway/health/**`, the existing five middleware files
(`cors.go`, `locale.go`, `logging.go`, `recovery.go`, `requestid.go`),
`services/monolith/internal/platform/{cache,db}/**`, and every file under `docs/`.

---

## 4. Acceptance-criteria coverage map

Every scenario in the three run-1 stories, the slice that owns its test, and where the *Then*
is observable. A scenario observable on both sides appears twice — each side tests its own half
against the contract, never against the other side's code.

### AUTH-001 — registration

| Scenario | TCs | Slice | Side | Kind |
|---|---|---|---|---|
| registering with a new email creates a User account | TC-01 | `be-auth-credentials` | server | handler + service |
| email uniqueness is case-insensitive | TC-02 | `be-auth-credentials` | server | handler (byte-equal to the duplicate response) |
| password below the minimum length is rejected | TC-03 | `be-auth-credentials` | server | handler |
| a breached password is rejected or flagged | TC-04, TC-05 | `be-auth-credentials` | server | service, with a stubbed HIBP (hit, miss, and unreachable) |
| an injected role field is ignored on registration | TC-06 | `be-auth-credentials` | server | handler |
| first Google login auto-creates an account | TC-07 | `be-auth-credentials` | server | service (`ProvisionFromOAuth`) |
| OAuth login matching an existing local account requires proof of control | TC-08 | `be-auth-credentials` | server | service + handler (redirect target, no session) |
| unverified account is read-only on its own data | TC-09 | `be-gateway-authz` | server | middleware |
| unverified account is read-only on its own data | TC-09 | `fe-auth-flows` | client | component (banner + the write control's disabled state) |
| verification token expires after 24 hours | TC-10 | `be-auth-credentials` | server | service |
| verification token is single-use | TC-11 | `be-auth-credentials` | server | service |
| password reset completes and logs out every session | TC-12 | `be-auth-credentials` | server | handler, asserting `SessionIssuer.RevokeAll` was called |
| expired or already-used reset token is rejected | TC-13 | `be-auth-credentials` | server | handler (and no revoke call) |
| registration response does not confirm an email is already registered | TC-14 | `be-auth-credentials` | server | handler, **byte-equal bodies** |
| password-reset request does not confirm whether the email exists | TC-15 | `be-auth-credentials` | server | handler, **byte-equal bodies** |
| (cross-endpoint enumeration regression) | TC-16 | `be-wiring` | server | integration |

### AUTH-002 — login

| Scenario | TCs | Slice | Side | Kind |
|---|---|---|---|---|
| successful login issues cookie-only tokens and a CSRF token | TC-01, TC-11 | `be-auth-session` | server | handler (cookie attrs, body has no token) |
| successful login issues cookie-only tokens and a CSRF token | TC-01 | `fe-auth-client` | client | unit (CSRF token retained, never persisted; no cookie read) |
| login creates a session record with a device label | TC-02 | `be-auth-session` | server | handler + Redis (miniredis) |
| Google OAuth login for an existing account issues the same session shape | TC-03 | `be-auth-session` | server | unit on `SessionIssuer` — one issuer, so one shape |
| wrong password gives a generic failure without revealing which factor failed | TC-04 | `be-auth-session` | server | handler |
| nonexistent email gives an identical generic failure | TC-05 | `be-auth-session` | server | handler, **byte-equal to the above** |
| five consecutive failures trigger backoff | TC-06 | `be-auth-session` | server | handler |
| backoff increases with continued failures | TC-07 | `be-auth-session` | server | unit on the backoff calculation |
| a successful login resets the failure counter | TC-08 | `be-auth-session` | server | handler |
| backoff blocks the account even from a new source IP | TC-09 | `be-auth-session` | server | handler, two source IPs |
| an unverified account can still log in | TC-10 | `be-auth-session` | server | handler |

### AUTH-003 — refresh, rotation, logout

| Scenario | TCs | Slice | Side | Kind |
|---|---|---|---|---|
| valid refresh rotates the token pair | TC-01 | `be-auth-session` | server | handler |
| replaying a rotated refresh token kills every session | TC-02 | `be-auth-session` | server | handler + audit assertion (critical event) |
| the gateway check order is signature, then expiry, then blacklist, then epoch | TC-03 | `libs-authmw` | server | unit on the check chain |
| the gateway check order is signature, then expiry, then blacklist, then epoch | TC-03 | `be-gateway-authz` | server | middleware (logged reason + 401 code) |
| downstream services trust the gateway without their own Redis check | TC-04 | `be-gateway-authz` | server | middleware, with a Redis client that fails the test if called after the gateway stage |
| logout ends only the current session | TC-05 | `be-auth-session` | server | handler |
| logout-all ends every session without enumerating tokens | TC-06 | `be-auth-session` | server | handler, asserting exactly one epoch write and zero blacklist writes |
| an idle session beyond 14 days cannot refresh even within the 30-day window | TC-07 | `be-auth-session` | server | handler with an injected clock |
| an active session survives past 14 days as long as it keeps refreshing | TC-08 | `be-auth-session` | server | handler with an injected clock |
| Redis unreachable fails closed on refresh and open on reads | TC-09 | `be-gateway-authz` | server | middleware (unsafe → 503) |
| Redis unreachable fails closed on refresh and open on reads | TC-10 | `be-gateway-authz` | server | middleware (GET → proceeds, logs warn) |
| (every blacklist key carries a TTL) | TC-11 | `libs-authmw` | server | unit against miniredis |

### End-to-end

One Playwright-free integration path, owned by `be-wiring`, run against compose:
register → verify → login → authenticated GET → refresh → unsafe request with CSRF → logout →
the old access cookie is rejected. This is SRS §6's criteria 1–4 as a script, and it is the only
place both sides are live at once in run 1.

---

## 5. Sequencing and dispatch

```
pre  : be-wiring adds the module dependencies and commits a building go.mod/go.sum
wave1: plat-audit-model │ db-migrations │ libs-authmw │ libs-auditlog │ fe-auth-client   (5 parallel)
         ↓ gate 2
wave2: be-auth-store │ fe-auth-flows                                                     (2 parallel)
         ↓ gate 2
wave3: be-auth-credentials │ be-auth-session │ be-gateway-authz                          (3 parallel)
         ↓ gate 2
wave4: be-wiring                                                                         (1)
         ↓ gate 2 → phase 3 (multi-agent-testing)
```

No slice starts optimistically. `fe-auth-flows` in wave 2 does not wait for any backend slice —
it waits for `fe-auth-client`'s `AuthApi` surface and `plat-audit-model`'s error codes, and
nothing else.

---

## 6. Gate 1 checklist

- [x] The contract covers every surface this feature crosses — HTTP (§1), tokens and cookies
      (§2), Redis (§3), SQL (§4), Go interfaces (§5), middleware order (§6), the model (§7),
      the TS client (§8) — and is frozen at the commit named at the top of this file
- [x] Every slice has an owner, an exclusive write allowlist, a read-only list, and
      done-criteria with runnable commands (see `briefs/`)
- [x] No path appears in two allowlists (§3 is the flat list to grep)
- [x] Migrations are one slice, one executor (`db-migrations`)
- [x] `libs/auditmodel/model.yaml` and both i18n catalog sets have exactly one owner each
      (contract §8.3)
- [x] Every acceptance-criteria scenario in the three run-1 stories appears in §4 with a slice,
      a side and a test kind
- [x] Shared foundations (model, schema, token lib, audit writer, API client) are wave 1 and
      complete before their consumers start
- [x] Dependency manifests are pre-resolved so no slice needs to edit `go.mod` or `package.json`

## 7. Carried-forward open questions

Resolved for run 1 by the contract, but still open in the stories and worth revisiting before
run 2:

- **Rotation grace window** (AUTH-003): none, per the SRS's "immediately". The client's
  single-flight refresh (contract §8.2) is what stops a retry storm from looking like replay.
  If the testing phase produces a false replay under real latency, that is a contract amendment,
  not a slice bug.
- **Resending a verification email** (AUTH-001): no resend endpoint in run 1. An expired link
  currently means registering again, which the enumeration-neutral register response makes
  awkward. Needs a story before run 2.
- **First SystemAdmin bootstrap** (epic overview): still a direct database operation. Run 2
  owns it; the `role` column exists from run 1.
- **Account deletion**: still unspecified anywhere. Not in run 2 either unless a story is written.
