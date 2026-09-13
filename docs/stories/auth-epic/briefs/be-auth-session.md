# Executor brief: be-auth-session

**Slice ID:** `be-auth-session`
**Story / spec:** [AUTH-002](../../AUTH-002-auth-login/story.md) ·
[AUTH-003](../../AUTH-003-auth-session-refresh-logout/story.md), with both stories'
[AUTH-002 AC](../../AUTH-002-auth-login/acceptance-criteria.md) /
[test cases](../../AUTH-002-auth-login/test-cases.md) and
[AUTH-003 AC](../../AUTH-003-auth-session-refresh-logout/acceptance-criteria.md) /
[test cases](../../AUTH-003-auth-session-refresh-logout/test-cases.md) ·
SRS-AUTH-001 §3.2 (FR-10..14), §3.4 (FR-30..34)
**Domain:** backend
**Depends on:** `libs-authmw`, `libs-auditlog`, `be-auth-store` — all must have reported done
**Load skill:** `multi-agent-execution` (`.claude/skills/multi-agent-execution/SKILL.md`)
**Read first:** [`docs/backend.md`](../../../backend.md) §5, §5a, §7 ·
[`docs/shared-contract.md`](../../../shared-contract.md) §3 ·
[`../contract.md`](../contract.md) §1, §2, §3, §5 ·
`.claude/skills/multi-agent-execution/references/backend-distributed.md`

## Task

Build the session lifecycle: login, CSRF issuance, the failed-login counter and backoff, refresh
with rotation and replay detection, logout, logout-all, the session record, and `GET /users/me`.

This slice owns the largest share of the run's acceptance criteria — 19 of them — because
AUTH-002 and AUTH-003 are two halves of one mechanism and splitting them would have put the
session record under two authors.

Two peers are building alongside you and you import neither. `be-auth-credentials` calls your
`SessionIssuer` (it is how the OAuth callback establishes a session) and you never call it back.
`be-gateway-authz` validates tokens on the way in; you mint them. Both meet you at wave 4's
composition root, through `authdomain`.

## You own (exclusive write access)

- `services/monolith/internal/auth/session/**`

That includes the `/users/me` handler (contract D6) — every field it returns is auth-owned, and
it re-issues the session-bound CSRF token, which is session state.

## Read-only context

- `docs/stories/auth-epic/contract.md` §1 rows 8–12, §2 (claims, cookies, CSRF), §3 (every Redis
  key and its exact TTL semantics), §5 (`SessionIssuer`, which you implement)
- `services/monolith/internal/auth/authdomain/**` — types, `AccountRepo`, `SessionIssuer`
- `libs/authmw/**` — **use it, do not reimplement it.** Minting, verifying, the check chain,
  every Redis accessor and the backoff arithmetic already live there.
- `libs/auditlog/**` — `Writer` and `Fake`
- `libs/httpkit/envelope.go` — the only way you write a body
- `docs/stories/AUTH-002-auth-login/test-cases.md` and
  `docs/stories/AUTH-003-auth-session-refresh-logout/test-cases.md` — TC data and preconditions

## Do not touch

- `services/monolith/internal/auth/{authdomain,store,credentials,oauth,hibp,mailer}/**` — peers'
- `services/monolith/internal/gateway/**` — `be-gateway-authz`'s
- `services/monolith/cmd/**`, `internal/platform/**` — `be-wiring`'s
- `libs/**`, `infra/**`, `go.mod` / `go.sum`, anything under `apps/web/`

## Contract you implement against

### Endpoints (contract §1, rows 8–12)

| Endpoint | Auth | Success | Errors |
|---|---|---|---|
| `POST /auth/login` | none | 200 `{"data":{"csrf_token"}}` + both cookies | 401 `AUTH_INVALID_CREDENTIALS`; **403 `AUTH_ACCOUNT_SUSPENDED` only after the password verifies**; 429 `AUTH_RATE_LIMITED` + `Retry-After`; 400 `VALIDATION_FAILED` |
| `POST /auth/refresh` | refresh cookie | 200 `{"data":{"csrf_token"}}` + rotated cookies | 401 `AUTH_INVALID_TOKEN`; 401 `AUTH_SESSION_EXPIRED`; 503 `SERVICE_UNAVAILABLE` |
| `POST /auth/logout` | access + CSRF | **204**, cookies cleared | 401, 403 `AUTH_CSRF_FAILED`, 503 |
| `POST /auth/logout-all` | access + CSRF | **204**, cookies cleared | as above |
| `GET /users/me` | access | 200 `{"data":{"user":{id,email,role,tier,email_verified},"csrf_token"}}` | 401 `AUTH_INVALID_TOKEN` / `AUTH_SESSION_EXPIRED` |

`tier` is the literal `"free"` with a `TODO` naming the subscription domain that will own it
(contract D6). Do not invent a table for it.

**`email_verified` added by [AMD-003](../amendments/AMD-003-users-me-needs-email-verified.md).**
The SPA can't read an httpOnly cookie's JWT claims, so the client-side half of AUTH-001's
unverified-banner scenario (already built by `fe-auth-flows`, which correctly anticipated this
gap) has nothing to read without it. **Read it from `Account.IsVerified()`
(`authdomain.Account`, via the `AccountRepo.ByID` call this handler already makes) — not from the
access token claim already on the request.** The two must always agree, but the account row is
the source of truth the claim is derived from; reading from the claim would go stale the instant
an account is verified without the user obtaining a new token, which is exactly the window this
field exists to cover correctly.

### Login order of operations — this is where D2 lives

1. Rate limit / backoff check (`login_backoff:{user_id}` present → 429 with the key's remaining
   TTL as `Retry-After`, and **do not verify the password**; emit
   `auth.login.backoff_triggered`).
2. Look up the account. **Always perform an argon2id verification** — against a fixed dummy hash
   when there is no account — so an unknown address costs the same as a wrong password
   (contract §1.1, FR-08/FR-14).
3. Password wrong, or no account → `401 AUTH_INVALID_CREDENTIALS`, **byte-identical in both
   cases**; increment `login_fail:{user_id}` and `login_fail_ip:{ip}`; emit `auth.login.failed`
   with the internal `reason` (`unknown_email` / `bad_password`) that never reaches the caller.
4. **Password correct and account suspended → `403 AUTH_ACCOUNT_SUSPENDED`**, emit
   `auth.login.suspended`. This ordering *is* decision D2: gating the distinct code behind a
   correct password is what keeps it from being an enumeration oracle. Do not move this check
   earlier for efficiency — the efficiency is the vulnerability.
5. Success → clear all three counter keys, mint the pair, write the session record, return the
   CSRF token, emit `auth.login.succeeded`.

Unverified accounts log in normally (FR-03); the write restriction is the gateway's job, not
yours. The access token's `email_verified` claim is what carries it — set it correctly.

### Refresh, rotation, replay (FR-30, FR-31, FR-34)

- Verify the refresh cookie through `authmw`'s chain — signature → expiry → blacklist → epoch —
  and check `typ == "refresh"` first.
- **Blacklisted refresh token presented → replay.** Set the epoch to now+1 (killing every
  session, *including* the one legitimately issued moments ago), emit
  `auth.refresh.replay_detected` at `critical`, return 401. There is no grace window; that is
  deliberate (`plan.md` §7).
- Otherwise: mint a new pair with a new `jti` and the **same `sid`**, blacklist the old refresh
  `jti` with a TTL equal to its remaining life, update `refresh_jti` and `last_seen_at` in the
  session record, keep `sid` and `csrf_token`, emit `auth.refresh.rotated`.
- **The old access token keeps working until it expires** (AUTH-003 scenario 1). Rotation does
  not retroactively kill it, and adding a blacklist write for it would break that scenario.
- Idle expiry: `now - last_seen_at > 14d` → `401 AUTH_SESSION_EXPIRED`, even inside the 30-day
  absolute window. An actively-refreshing session survives past 14 days — idle time, not
  absolute age, is what the rule measures.
- Take a clock as a dependency. Two scenarios are only testable with time under your control.

### Logout / logout-all

- `logout` (FR-32): blacklist the current access **and** refresh `jti`s, each with a TTL equal
  to its own remaining life; delete the `sid` field from `user_sessions`; clear both cookies;
  emit `auth.logout`. Other devices are untouched.
- `logout-all` (FR-33): **one** epoch write, `now + 1`. No per-token enumeration, no blacklist
  writes — AUTH-003's scenario asserts exactly that, and a loop over sessions here would be an
  O(n) operation NFR-05 exists to forbid. Emit `auth.logout_all`.
- `SessionIssuer.RevokeAll` is the same operation, exported for `be-auth-credentials` to call on
  a completed password reset.

### CSRF (FR-12, contract §2.3)

32 random bytes, base64url, stored in the session record, returned in the body at login, at
refresh and from `/users/me`. Compare constant-time. **You issue it; `be-gateway-authz`
validates it** — do not also write a validation middleware.

## Acceptance-criteria scenarios this slice covers with automated tests

Named after the scenario verbatim (`backend.md` §7). Handler tests through `httptest` against
the Gin router, asserting status, envelope, code, and cookie attributes (`HttpOnly`, `Secure`,
`SameSite`, `Path`). Redis via miniredis; audit via `auditlog.Fake`.

### AUTH-002

| Scenario | TCs | Kind | Location |
|---|---|---|---|
| successful login issues cookie-only tokens and a CSRF token | TC-01, TC-11 | handler — both cookie attribute sets, refresh `Path=/api/v1/auth`, **no token string anywhere in the body** | `.../session/login_test.go` |
| login creates a session record with a device label | TC-02 | handler + miniredis — label parsed from User-Agent, `Unknown device` fallback for an unparseable one | ” |
| Google OAuth login for an existing account issues the same session shape | TC-03 | unit on `SessionIssuer` — one issuer, therefore one shape; assert cookies, CSRF and record are identical to the login path's | `.../session/issuer_test.go` |
| wrong password gives a generic failure without revealing which factor failed | TC-04 | handler | `.../session/login_test.go` |
| nonexistent email gives an identical generic failure | TC-05 | handler — **byte-equal body and status** to the above | ” |
| five consecutive failures trigger backoff | TC-06 | handler | `.../session/backoff_test.go` |
| backoff increases with continued failures | TC-07 | unit on the delay sequence (30 → 60 → 120, capped) | ” |
| a successful login resets the failure counter | TC-08 | handler | ” |
| backoff blocks the account even from a new source IP | TC-09 | handler, two source IPs, same account | ” |
| an unverified account can still log in | TC-10 | handler — 200, and `email_verified:false` in the minted claim | `.../session/login_test.go` |

### AUTH-003

| Scenario | TCs | Kind | Location |
|---|---|---|---|
| valid refresh rotates the token pair | TC-01 | handler — new `jti`s, same `sid`, old refresh blacklisted with a correct TTL, **old access still valid** | `.../session/refresh_test.go` |
| replaying a rotated refresh token kills every session | TC-02 | handler — 401, epoch advanced, `auth.refresh.replay_detected` at `critical` in the fake | ” |
| logout ends only the current session | TC-05 | handler — device A's `jti`s blacklisted and record removed, device B untouched | `.../session/logout_test.go` |
| logout-all ends every session without enumerating tokens | TC-06 | handler — **exactly one epoch write, zero blacklist writes** | ” |
| an idle session beyond 14 days cannot refresh even within the 30-day window | TC-07 | handler, injected clock | `.../session/refresh_test.go` |
| an active session survives past 14 days as long as it keeps refreshing | TC-08 | handler, injected clock | ” |

**Locale coverage** (`backend.md` §7): assert one error response renders differently under
`Accept-Language: en-US` and `vi-VN`, while the audit row written in the same test is en-US.

## Done when

- [ ] All 16 scenarios above have passing tests named after them, asserting their *Then*
- [ ] The two enumeration-neutral responses are asserted **byte-equal**
- [ ] `SessionIssuer` is implemented per `authdomain`; your package imports no peer wave-3 package
- [ ] Nothing in this slice reimplements minting, verification, a Redis key or the backoff
      arithmetic — all of it comes from `libs/authmw`
- [ ] `Register(r gin.IRouter)` exists for `be-wiring`; you do not edit `main.go` or the gateway
- [ ] No token string is written to a body, a URL, a log or an audit field — only `jti` and `sid`
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

The two most likely temptations, both of which quietly break a story: adding a rotation grace
window so a retried refresh isn't flagged as replay (FR-31 — raise it as an amendment, the
client's single-flight refresh is the agreed mitigation), and checking suspension before the
password to save a hash (that is the enumeration oracle D2 exists to close).

## Report back

Write the report to `docs/stories/auth-epic/reports/be-auth-session.md`.

1. **Changed:** files touched, and what each change does
2. **Verified:** commands run and their actual results, with a scenario → test → result line for
   every row in both scenarios tables
3. **Could not do:** anything blocked, and why
4. **Noticed:** problems outside your scope. Report them, don't fix them.
