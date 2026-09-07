# Executor brief: libs-authmw

**Slice ID:** `libs-authmw`
**Story / spec:** [AUTH-002](../../AUTH-002-auth-login/story.md) ·
[AUTH-003](../../AUTH-003-auth-session-refresh-logout/story.md) ·
[`docs/srs-authentication.md`](../../../srs-authentication.md) §2.2, FR-20, FR-30..34, NFR-01,
NFR-03, NFR-06, SEC-02, SEC-03
**Domain:** backend library
**Depends on:** none — may start immediately
**Load skill:** `multi-agent-execution` (`.claude/skills/multi-agent-execution/SKILL.md`)
**Read first:** [`docs/backend.md`](../../../backend.md) §7 ·
[`docs/shared-contract.md`](../../../shared-contract.md) ·
[`../contract.md`](../contract.md) §2, §3, §6

## Task

Build `libs/authmw` — the library every auth decision in the system is made with, and the only
place token and Redis-state semantics are implemented.

It has no knowledge of Gin handlers, no HTTP routes, and no domain types. Three slices consume
it in wave 3 (login/refresh, credentials, gateway middleware) and none of them should reimplement
a single line of what you write. NFR-06 requires this whole library to be unit-testable **without
Docker** — use `miniredis` (already in `go.mod`), never a live Redis.

## You own (exclusive write access)

- `libs/authmw/**`

## Read-only context

- `docs/stories/auth-epic/contract.md` §2 (claims, TTLs, cookie attributes), §3 (every Redis key,
  its value shape and its TTL), §6 (the check order and the degradation policy)
- `docs/srs-authentication.md` §2.2, §3.3, §3.4, §5
- `libs/auditmodel/generated.go` — `TokenRejectReason`, `Role` and friends. **Wave 1 peer
  `plat-audit-model` is regenerating this file while you work**; the constants you need
  (`TokenRejectReason*`, `Role*`) already exist and are not being changed, so you are not
  blocked. Do not import anything the model does not already declare.
- `services/monolith/internal/platform/cache/cache.go` — how a `*redis.Client` is constructed;
  you take one as a dependency, you never construct one
- `libs/httpkit/envelope.go` — how a code becomes a response, so you can see why this library
  returns typed reasons rather than writing responses itself

## Do not touch

- `libs/auditmodel/**` — owned by `plat-audit-model`
- `libs/auditlog/**` — owned by `libs-auditlog` (your peer this wave)
- `libs/httpkit/**`, `libs/i18n/**` — not part of this run
- Anything under `services/` — every consumer of this library is a later slice
- `go.mod` / `go.sum` — owned by `be-wiring`; `golang-jwt/v5`, `google/uuid`, `x/crypto` and
  `miniredis` are already present

## Contract you implement against

### Tokens (contract §2.1)

```
access  { sub, sid, role, tier, email_verified, jti, iat, exp, typ:"access" }   TTL 15m
refresh { sub, sid,                             jti, iat, exp, typ:"refresh" }  TTL 30d
```

RS256, `kid` in the header, **two-key verification window** (SEC-02): one key signs, a set of
keys verify, so a rotation does not invalidate live tokens. Loading: a private key from a path,
public keys from a directory of `<kid>.pub`.

`sid` is the **session** id — stable across every rotation within one login. `jti` is the
**token** id — new on every mint, and the unit of blacklisting. Confusing the two produces a
system where rotation kills the session it just refreshed; keep them distinctly named
everywhere.

`typ` is verified before anything else that depends on the token's kind: an access token
presented to refresh, or a refresh token presented as an access token, is rejected outright.

### The check chain (FR-20, contract §6)

Exactly this order, and the **first** failure is what gets reported:

```
signature → expiry → blacklist:{jti} → iat vs user_blacklist_epoch:{sub}
```

Expose it as a function returning `(Claims, TokenRejectReason, error)` — a typed reason from
`auditmodel.TokenRejectReason`, not a string, because the caller records it in an audit event
and the caller must **not** be able to leak which check failed into the response. Every failure
is one code, `AUTH_INVALID_TOKEN`, and that decision belongs to the middleware, not here.

### Redis state (contract §3)

Typed accessors for each key. Reproduce the table's TTL rules exactly:

| Key | Notes that are easy to get wrong |
|---|---|
| `blacklist:{jti}` | TTL is the token's **remaining** life computed from `exp`, never a constant. A blacklist entry outliving its token is a leak; one expiring early is a revocation that silently stops working (NFR-03) |
| `user_blacklist_epoch:{user_id}` | Written as `now + 1` second, because `iat` has second resolution and a token minted in the same second must not survive the epoch (contract §3) |
| `user_sessions:{user_id}` | Hash, field `sid` → JSON `{sid, refresh_jti, device_label, created_at, last_seen_at, csrf_token}`. Rotation updates `refresh_jti` and `last_seen_at` and preserves `sid` and `csrf_token` |
| `login_fail:{user_id}`, `login_fail_ip:{ip}` | 15m sliding; both deleted on success |
| `login_backoff:{user_id}` | Value is the step `n`; TTL **is** the delay, `30 * 2^(n-1)`, capped at 3600. Remaining TTL is what a caller reports as `Retry-After` |

Provide the backoff arithmetic as a pure function too — AUTH-002's "backoff increases with
continued failures" is tested against it directly.

### Degradation (NFR-02, contract §6)

A helper that answers, for a given request method and path class, whether a Redis outage means
*fail closed* (unsafe methods, `/auth/refresh`, `/auth/logout*` → the caller returns 503) or
*fail open* (GET/HEAD proceed on signature+expiry alone). Return the decision as a value; the
middleware slice turns it into a response and a log line. Keeping the policy here is what stops
two slices inventing two different answers.

### Cookies (contract §2.2, SEC-03)

Helpers that set and clear `gs_access` and `gs_refresh` with the exact attributes in §2.2,
including the refresh cookie's `Path=/api/v1/auth`. `Secure` is emitted unless a passed-in
option disables it (`AUTH_COOKIE_SECURE=false`, which exists only for local http). The default
is secure; a test asserting attributes asserts the default.

## Acceptance-criteria scenarios this slice covers with automated tests

| Scenario | Test cases | Side | Test kind | Location |
|---|---|---|---|---|
| the gateway check order is signature, then expiry, then blacklist, then epoch | AUTH-003 TC-03 | server | unit — a token failing several checks at once reports the first one in order | `libs/authmw/verify_test.go` |
| (NFR-03) every blacklist key carries a TTL | AUTH-003 TC-11 | server | unit against miniredis — assert TTL exists and equals the token's remaining life, for every path that blacklists | `libs/authmw/redis_test.go` |

Name the first test after the scenario verbatim
(`TestCheckOrder_SignatureThenExpiryThenBlacklistThenEpoch` is fine; the coverage audit in
phase 3 matches names to `acceptance-criteria.md`). Beyond these two, cover: mint/verify round
trips for both token kinds, `kid` rotation with two verifying keys, `typ` confusion rejection,
epoch boundary at the same-second edge, and the backoff sequence 30/60/120 with its cap.

## Done when

- [ ] Both scenarios above have a named, passing test asserting their *Then*
- [ ] Every Redis key in contract §3 has a typed accessor, and every write sets a TTL — there is
      no code path that creates a key without one
- [ ] The check chain is one exported function whose failure value is a typed
      `auditmodel.TokenRejectReason`
- [ ] No file in this library imports `net/http` handlers, Gin, or any `services/` package
- [ ] Tests run with **no Docker and no Redis** (NFR-06): `go test ./libs/authmw/...`
- [ ] Builds: `go build ./... && go vet ./...`; full suite green: `go test ./...`
- [ ] Nothing modified outside the allowlist above (check your own diff)

## Stop condition — read this before starting

If you find the contract wrong, ambiguous, or insufficient for this task:

> **Stop work on the affected surface. Write an amendment request using
> `.claude/skills/multi-agent-execution/assets/amendment-request-template.md`. Do not proceed on
> that surface.**

Do not fix the contract yourself, and do not implement around it. Other slices are being built
against the current version right now; a local fix produces two implementations that each
believe they conformed, and nothing detects the mismatch until runtime.

The likeliest one here: you may conclude the rotation grace window (AUTH-003's open question)
is needed to avoid false replay detection. The contract deliberately has none, and the client's
single-flight refresh is the mitigation. If you still think it is wrong, that is an amendment
request — not a five-second tolerance you add quietly, which would defeat FR-31.

## Report back

Write the report to `docs/stories/auth-epic/reports/libs-authmw.md`.

1. **Changed:** files touched, and what each change does
2. **Verified:** commands run and their actual results, with a scenario → test → result line for
   each row in the scenarios table
3. **Could not do:** anything blocked, and why
4. **Noticed:** problems outside your scope. Report them, don't fix them.
