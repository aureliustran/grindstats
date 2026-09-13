# Executor brief: be-gateway-authz

**Slice ID:** `be-gateway-authz`
**Story / spec:** [AUTH-003](../../AUTH-003-auth-session-refresh-logout/story.md) (the FR-20
check chain and NFR-02 degradation) · [AUTH-001](../../AUTH-001-auth-registration/story.md)
(the unverified-write rule) · [AUTH-005](../../AUTH-005-auth-admin-account-management/story.md)
(the role check, built now and mounted in run 2) · SRS-AUTH-001 §3.3 (FR-20..24), NFR-02, NFR-04
**Domain:** backend
**Depends on:** `plat-audit-model`, `libs-authmw`, `libs-auditlog` — all must have reported done
**Load skill:** `multi-agent-execution` (`.claude/skills/multi-agent-execution/SKILL.md`)
**Read first:** [`docs/backend.md`](../../../backend.md) §5, §7 ·
[`docs/stories/GATE-001-api-gateway-health/story.md`](../../GATE-001-api-gateway-health/story.md)
(what the chain already is, and why the stages were reserved) ·
[`../contract.md`](../contract.md) §2, §3, §6

## Task

Fill the five middleware stages GATE-001 reserved and left empty, in the order it reserved them,
and wire them into `gateway.New`.

```
recovery → request ID → logging → CORS → locale →
rate limit → auth → CSRF → verified-write → role → handler
```

`gateway.go`'s own comment names the stories that own these stages; you are those stories'
implementation. You own `gateway.go` this run because filling those stages is the change that
file exists to receive — but you change **only** what the stages require. The GATE-001 chain
above the reserved block, `/healthz`, `/readyz` and their unenveloped bodies are frozen contract
with tests already written.

You import no peer wave-3 package. Everything you need is `libs/authmw` and `libs/auditlog`.

## You own (exclusive write access)

- `services/monolith/internal/gateway/gateway.go`
- `services/monolith/internal/gateway/gateway_test.go`
- `services/monolith/internal/gateway/middleware/auth.go` + `auth_test.go`
- `services/monolith/internal/gateway/middleware/csrf.go` + `csrf_test.go`
- `services/monolith/internal/gateway/middleware/role.go` + `role_test.go`
- `services/monolith/internal/gateway/middleware/verified.go` + `verified_test.go`
- `services/monolith/internal/gateway/middleware/ratelimit.go` + `ratelimit_test.go`

## Read-only context

- `docs/stories/auth-epic/contract.md` §6 (every stage, its behavior and its error code),
  §2 (claims and CSRF), §3 (the Redis keys you read)
- `libs/authmw/**` — the check chain, the typed reject reasons, the Redis accessors and the
  degradation-policy helper. **All of it already exists; reimplementing any of it here is the
  failure this partition was drawn to prevent.**
- `libs/auditlog/**` — `Writer` and `Fake`
- `libs/httpkit/envelope.go` — how a code becomes a response
- `services/monolith/internal/gateway/middleware/{cors,locale,logging,recovery,requestid}.go` —
  the established middleware idiom in this repo. Match it.
- `services/monolith/internal/gateway/health/**` — read-only; its behavior must not change

## Do not touch

- The five existing middleware files, and `health/**` — GATE-001's, frozen
- Anything under `services/monolith/internal/auth/**` — three peer slices own it
- `services/monolith/cmd/**` and `internal/platform/**` — `be-wiring`'s. You expose middleware
  constructors and a `Deps` field or two; **wiring the auth handlers' routes is not yours.**
- `libs/**`, `infra/**`, `go.mod` / `go.sum`, `apps/web/**`

## Contract you implement against

### auth (FR-20) — contract §6

Checks in exactly this order, reporting the **first** failure as the `TokenRejectReason`:
signature → expiry → `blacklist:{jti}` → `iat` vs `user_blacklist_epoch:{sub}`. Call
`libs/authmw`'s chain; do not re-derive the order here — two copies of an ordering rule is two
places for it to drift.

**Every failure returns `401 AUTH_INVALID_TOKEN`**, whichever check failed. The reason is
recorded in `auth.token.rejected` and never reaches the caller: telling a client which check
failed tells an attacker whether a `jti` is known or whether an epoch moved
(`audit-and-errors.md` §4).

Public routes opt out **by route group**, not by the middleware inspecting paths. A middleware
that maintains its own list of public paths is a list that will disagree with the router.

Put the validated claims in the request context. Downstream packages read them via
`authmw.ClaimsFromContext` and **never call Redis** (FR-21) — the gateway paid that cost once.

### CSRF (FR-24) — contract §2.3, D7

Required on `POST/PUT/PATCH/DELETE`, comparing `X-CSRF-Token` constant-time against the token in
the session record. Mismatch or absence → `403 AUTH_CSRF_FAILED`. Safe methods exempt.

**Exempt:** the unauthenticated auth endpoints (no session exists to bind to) and
`POST /auth/refresh`. The refresh exemption is decision D7 and has a reason worth knowing: the
SPA holds the CSRF token in memory only, so after a reload with an expired access token the boot
path is refresh → `/users/me`, and requiring CSRF on refresh deadlocks it. Refresh is protected
instead by `SameSite=Lax` (a browser does not attach the cookie to a cross-site POST) plus its
`Path=/api/v1/auth` scope and replay detection.

### verified-write (FR-03, D4)

Unsafe method + `email_verified == false` in the claims → `403 AUTH_EMAIL_UNVERIFIED`, and emit
`auth.write_blocked_unverified` with the path and request ID. Exempt: everything under
`/api/v1/auth/`. This is the *only* place FR-03 is enforced (D4) — per-service checks were
rejected precisely so no future domain has to remember.

### role (FR-22)

`RequireRole(system_admin)` → `403 AUTH_FORBIDDEN` for a `User` token, and emit
`admin.action.denied`. **403, never 404** — the SRS explicitly rejects hiding admin routes behind
a fake not-found. Run 1 ships the middleware and its tests; run 2 (AUTH-005) mounts routes behind
it. Build it complete anyway; a role check written later, under deadline, next to the endpoints
it guards is how a 404-instead-of-403 gets shipped.

### rate limit (NFR-04)

Redis token bucket, per IP and per account where the contract says both:
`login` 10/min per IP and per account · `register` 5/min per IP ·
`password-reset/*` 5/min per IP and per account · `verify-email` and `oauth/link/confirm`
10/min per IP · `refresh` 60/min per session. Exceeded → `429 AUTH_RATE_LIMITED` with
`Retry-After`. This is a separate control from AUTH-002's 5-failure backoff; both apply, and
neither substitutes for the other.

### degradation (NFR-02) — contract §6

When Redis is unreachable, ask `libs/authmw`'s policy helper, then act on its answer:

- **Safe methods (GET/HEAD): fail open.** Proceed on signature + expiry alone, log at `warn`.
  Availability of a fitness dashboard's reads outweighs revocation lag.
- **Everything else, including `/auth/refresh` and `/auth/logout*`: fail closed.**
  `503 SERVICE_UNAVAILABLE`, log at `error`. Nothing state-changing proceeds unchecked.

## Acceptance-criteria scenarios this slice covers with automated tests

Named after the scenario verbatim (`backend.md` §7). Middleware tested through `httptest`
against a router with the real chain assembled; Redis via miniredis; audit via `auditlog.Fake`.

| Scenario | TCs | Story | Kind | Location |
|---|---|---|---|---|
| unverified account is read-only on its own data | TC-09 | AUTH-001 | middleware — an unverified claim POSTing a non-auth route gets 403 `AUTH_EMAIL_UNVERIFIED`; a GET succeeds; the same POST under `/api/v1/auth/` succeeds | `.../middleware/verified_test.go` |
| the gateway check order is signature, then expiry, then blacklist, then epoch | TC-03 | AUTH-003 | middleware — a token failing several checks at once logs the first reason in order and returns 401 `AUTH_INVALID_TOKEN` either way | `.../middleware/auth_test.go` |
| downstream services trust the gateway without their own Redis check | TC-04 | AUTH-003 | middleware — a Redis client that fails the test if called after the gateway stage; a downstream handler reads claims from context and makes zero Redis calls | ” |
| Redis unreachable fails closed on refresh and open on reads | TC-09 | AUTH-003 | middleware — unsafe method / refresh path with Redis down → 503, logged at `error` | `.../middleware/auth_test.go` |
| Redis unreachable fails closed on refresh and open on reads | TC-10 | AUTH-003 | middleware — GET with a signature-and-expiry-valid token and Redis down → proceeds, logged at `warn` | ” |

Also required, as this slice's own conformance tests:

- a `User` token on a `RequireRole(system_admin)` route → **403, not 404**, and
  `admin.action.denied` written
- CSRF: missing header → 403 `AUTH_CSRF_FAILED`; wrong value → 403; correct → through; GET
  exempt; `/auth/refresh` exempt
- rate limit: the (n+1)th call in a window → 429 with a `Retry-After`
- the GATE-001 chain is unchanged: `/healthz` and `/readyz` still return their existing
  unenveloped bodies, and the existing middleware order above the reserved block still holds
  (its tests must still pass untouched)

## Done when

- [ ] All five scenario rows have passing tests named after them
- [ ] Every conformance test above passes
- [ ] The stage order in `gateway.New` is exactly the contract's, and the GATE-001 stages above
      it are unmodified
- [ ] `gateway_test.go`'s pre-existing assertions still pass — you extended that file, you did
      not rewrite it
- [ ] No check-order, Redis-key or degradation logic is duplicated from `libs/authmw`
- [ ] Every 401/403 logs the code, `jti`, user ID where known, and request ID — and **never a
      token value** (NFR-07)
- [ ] Builds: `go build ./... && go vet ./...`; tests pass: `go test ./...`
- [ ] Nothing modified outside the allowlist above (check your own diff)

## Stop condition — read this before starting

If you find the contract wrong, ambiguous, or insufficient for this task:

> **Stop work on the affected surface. Write an amendment request using
> `.claude/skills/multi-agent-execution/assets/amendment-request-template.md`. Do not proceed on
> that surface.**

Do not fix the contract yourself, and do not implement around it. Other slices are being built
against the current version right now; a local fix produces two implementations that each
believe they conformed, and nothing detects the mismatch until runtime.

The one to watch: it will look reasonable to give the auth middleware its own list of public
paths, or to have it register the auth routes since it is "already in the gateway". Both are
other slices' territory — route groups are `be-wiring`'s composition, and the handlers are
`be-auth-credentials`' and `be-auth-session`'s.

## Report back

Write the report to `docs/stories/auth-epic/reports/be-gateway-authz.md`.

1. **Changed:** files touched, and what each change does
2. **Verified:** commands run and their actual results, with a scenario → test → result line for
   every row in the scenarios table
3. **Could not do:** anything blocked, and why
4. **Noticed:** problems outside your scope. Report them, don't fix them.
