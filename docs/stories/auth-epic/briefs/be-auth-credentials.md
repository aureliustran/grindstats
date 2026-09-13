# Executor brief: be-auth-credentials

**Slice ID:** `be-auth-credentials`
**Story / spec:** [AUTH-001](../../AUTH-001-auth-registration/story.md) +
[its acceptance criteria](../../AUTH-001-auth-registration/acceptance-criteria.md) +
[test cases](../../AUTH-001-auth-registration/test-cases.md) ·
SRS-AUTH-001 §3.1 (FR-01..08), SEC-01, SEC-04, SEC-05
**Domain:** backend
**Depends on:** `libs-authmw`, `libs-auditlog`, `be-auth-store` — all must have reported done
**Load skill:** `multi-agent-execution` (`.claude/skills/multi-agent-execution/SKILL.md`)
**Read first:** [`docs/backend.md`](../../../backend.md) §5, §5a, §7 ·
[`docs/shared-contract.md`](../../../shared-contract.md) §3 ·
[`../contract.md`](../contract.md) §1, §4.3, §5, §7 ·
`.claude/skills/multi-agent-execution/references/backend-distributed.md`

## Task

Build AUTH-001's backend: registration, email verification, both halves of password reset, and
the Google OAuth authorize/callback pair with its linking-proof flow.

Two peers are building alongside you and you import neither: `be-auth-session` owns login,
refresh and logout; `be-gateway-authz` owns the middleware. You reach them only through the
`authdomain` interfaces — you *implement* `AccountProvisioner`, you *consume* `SessionIssuer`.
Wave 4 wires the concrete types together.

**The single rule everything in AUTH-001 defends: a registrant can never end up with any role but
`User`** (FR-04). No request field, header or payload value influences role assignment. There is
no code path in this slice that reads a role from a request.

## You own (exclusive write access)

- `services/monolith/internal/auth/credentials/**` — register, verify-email, password-reset,
  password hashing, `AccountProvisioner`
- `services/monolith/internal/auth/oauth/**` — Google authorize + callback, state/PKCE, linking
- `services/monolith/internal/auth/hibp/**` — the breached-password client
- `services/monolith/internal/auth/mailer/**` — sending the three emailed links

## Read-only context

- `docs/stories/auth-epic/contract.md` §1 rows 1–7, §4.3 (the OAuth linking flow), §5 (the
  interfaces), §7 (the events you emit)
- `services/monolith/internal/auth/authdomain/**` — your types and interfaces. **Read this
  before designing anything**; it is what `be-auth-store` shipped and what your peers also read.
- `libs/authmw/**`, `libs/auditlog/**` — token/cookie helpers and the audit `Writer` + `Fake`
- `libs/httpkit/envelope.go` — the only way you write a response body
- `docs/audit-and-errors.md` §4 (why one code covers several events) and §1a (three language
  planes)
- `docs/stories/AUTH-001-auth-registration/test-cases.md` — TC-01..TC-15 give the data and
  preconditions your tests encode

## Do not touch

- `services/monolith/internal/auth/{authdomain,store}/**` — `be-auth-store`'s, frozen
- `services/monolith/internal/auth/session/**` — `be-auth-session`'s, being written right now
- `services/monolith/internal/gateway/**` — `be-gateway-authz`'s
- `services/monolith/cmd/**`, `internal/platform/config/**` — `be-wiring`'s. You expose a
  constructor and a `Register(r gin.IRouter)`; you do not wire yourself into `main.go`.
- `libs/**`, `infra/db/migrations/**`, `go.mod` / `go.sum`
- Anything under `apps/web/`

## Contract you implement against

### Endpoints (contract §1, rows 1–7)

| Endpoint | Request | Success | Errors |
|---|---|---|---|
| `POST /auth/register` | `{email, password}` | **202** `{"data":{"status":"pending_verification"}}` — **byte-identical whether or not the email exists**; no cookies | 400 `VALIDATION_FAILED`, 429 `AUTH_RATE_LIMITED` |
| `POST /auth/verify-email` | `{token}` | 200 `{"data":{"status":"verified"}}` | 400 `AUTH_LINK_INVALID` |
| `POST /auth/password-reset/request` | `{email}` | **202** `{"data":{"status":"sent"}}` — byte-identical for an unknown address, no mail sent | 400 `VALIDATION_FAILED`, 429 |
| `POST /auth/password-reset/confirm` | `{token, password}` | 200 `{"data":{"status":"reset"}}` + **epoch advanced** | 400 `AUTH_LINK_INVALID`, 400 `VALIDATION_FAILED` |
| `GET /auth/oauth/google` | — | 302 to Google, `gs_oauth_state` cookie set (state + PKCE verifier, signed) | — |
| `GET /auth/oauth/google/callback` | `?code&state` | 302 → `/dashboard` **with session cookies** | 302 → `/?auth_error=oauth_cancelled` on denial; `…=oauth_link_required` per §4.3; `…=oauth_failed` on bad state/PKCE. **Never a JSON body** — it is a browser navigation |
| `POST /auth/oauth/link/confirm` | `{token}` | 200 `{"data":{"status":"linked"}}`, no session issued | 400 `AUTH_LINK_INVALID` |

Bodies are written only through `libs/httpkit` (`backend.md` §5). Messages are rendered
server-side from the error code and the request's `Accept-Language` — never built in Go.

### Enumeration protection is the hard requirement of this slice

FR-08 and AUTH-001's last two scenarios are not "return a similar message". They require the
**same bytes**: same status, same body, same headers modulo `Date`, and no observable timing
difference.

- Register with a taken address does everything a new registration does *except* create the
  account and send the mail, and returns the identical 202.
- Password-reset request for an unknown address does everything except issue a token and send
  the mail, and returns the identical 202.
- Timing is equalised by always performing an argon2id operation — against a fixed dummy hash
  when there is no account (contract §1.1). A `return` before the hash is a timing oracle.
- `backend.md` §7 says it explicitly: when a scenario says two responses are identical, assert
  the raw bodies and statuses are **byte-equal**. Asserting each independently proves nothing
  about the pair.

### The rest, precisely

- **Hashing:** argon2id (SEC-01). Plaintext never logged, never stored, never in an event.
- **Password policy (FR-02, contract D3):** length ≥ 10 and nothing else — no composition rules.
  Then the HIBP k-anonymity check: a hit → `400 VALIDATION_FAILED` with
  `details:[{field:"password", rule:"breached"}]`. **HIBP unreachable or slow (default 2s
  timeout) → skip the check, log `warn`, proceed.** `AUTH_HIBP_ENABLED=false` disables it. This
  is decision D3 and it is what makes local dev and an HIBP outage non-blocking.
- **Role (FR-04):** every path creates role `User`. An injected `"role"` field is ignored by
  virtue of not existing in your request struct — that is the implementation, not a check you
  add. Test it anyway (TC-06).
- **Verification token:** `LinkKind.email_verification`, 24h, single-use, hashed at rest — all
  of which `LinkTokenRepo` already does. A second use returns `AUTH_LINK_INVALID` and **leaves
  the account verified** (AUTH-001 scenario: "the second attempt does not un-verify it").
- **Reset (FR-07):** `LinkKind.password_reset`, 1h, single-use. On success: set the new password,
  then call `SessionIssuer.RevokeAll` — that is logout-everywhere, and it is `be-auth-session`'s
  mechanism, not a second epoch write of your own. On a rejected token: password unchanged and
  **no revoke call at all** (AUTH-001 scenario, TC-13). Assert both halves.
- **OAuth (FR-05/06, SEC-05):** authorization-code + PKCE, `state` validated, redirect URIs
  exact-match allowlisted. First login for an unknown identity auto-creates a `User` account
  from the *verified* email claim. An email matching an existing local account with no identity
  row → contract §4.3: no session, no merge, an emailed `oauth_link` token, redirect to
  `/?auth_error=oauth_link_required`. The token is emailed rather than put in the redirect
  because a single-use credential in a query string lands in history, referrers and proxy logs.
- **Mailer:** an interface plus a development implementation that logs the link (email delivery
  infrastructure is explicitly out of scope for AUTH-001). Never log a raw token in production
  mode; the dev logger is opt-in via config that `be-wiring` supplies.
- **Audit (contract §7):** emit `auth.register.succeeded`, `auth.email.verified`,
  `auth.link.rejected` (with `kind` and `reason`), `auth.password_reset.requested`,
  `auth.password_reset.completed`, `auth.oauth.link_required`. `auth.password_reset.requested`
  carries `user_id` only when one exists — its absence is how an unknown address is recorded
  internally without the response saying so.

## Acceptance-criteria scenarios this slice covers with automated tests

Each row is a test named after the scenario verbatim (`backend.md` §7). Handler tests go through
`httptest` against the Gin router with a contract-shaped request.

| Scenario | TCs | Side | Kind | Location |
|---|---|---|---|---|
| registering with a new email creates a User account | TC-01 | server | handler + service | `.../credentials/register_test.go` |
| email uniqueness is case-insensitive | TC-02 | server | handler — response byte-equal to the duplicate case | ” |
| password below the minimum length is rejected | TC-03 | server | handler | ” |
| a breached password is rejected or flagged | TC-04, TC-05 | server | service, stubbed HIBP: hit → rejected, miss → allowed, **unreachable → allowed with a warn** | `.../hibp/hibp_test.go` + `.../credentials/register_test.go` |
| an injected role field is ignored on registration | TC-06 | server | handler | `.../credentials/register_test.go` |
| first Google login auto-creates an account | TC-07 | server | service (`ProvisionFromOAuth`) | `.../oauth/provision_test.go` |
| OAuth login matching an existing local account requires proof of control | TC-08 | server | service + handler: no session, no merge, redirect target, `auth.oauth.link_required` written | `.../oauth/callback_test.go` |
| verification token expires after 24 hours | TC-10 | server | service, injected clock | `.../credentials/verify_test.go` |
| verification token is single-use | TC-11 | server | service — second use rejected, account still verified | ” |
| password reset completes and logs out every session | TC-12 | server | handler, asserting `SessionIssuer.RevokeAll` called once | `.../credentials/reset_test.go` |
| expired or already-used reset token is rejected | TC-13 | server | handler — password unchanged, `RevokeAll` **not** called | ” |
| registration response does not confirm an email is already registered | TC-14 | server | handler, **byte-equal bodies and statuses** | `.../credentials/register_test.go` |
| password-reset request does not confirm whether the email exists | TC-15 | server | handler, **byte-equal bodies and statuses** | `.../credentials/reset_test.go` |

**Locale coverage** (`backend.md` §7): for at least one error response in this slice, assert the
rendered `message` differs between `Accept-Language: en-US` and `vi-VN`, and that the audit row
written in the same test is en-US regardless. That is the three-planes rule as a test.

Use `auditlog.Fake` for every "and an event was written" assertion; use fakes for
`SessionIssuer` and the repositories (`backend.md` §7: services are tested as plain functions
with fakes — no HTTP, no DB).

## Done when

- [ ] Every scenario above has a passing test named after it, asserting its *Then*
- [ ] The three enumeration-neutral pairs are asserted **byte-equal**, not merely both-present
- [ ] `AccountProvisioner` is implemented per `authdomain`, and `SessionIssuer` is consumed
      through the interface — your package imports no peer wave-3 package
- [ ] `Register(r gin.IRouter)` exists for `be-wiring` to call; nothing in this slice edits
      `main.go` or the gateway
- [ ] No response body is written except through `libs/httpkit`
- [ ] No plaintext password, raw link token, or token value appears in any log or audit field
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

Two that would be genuinely damaging to fix locally: adding a "resend verification" endpoint
(it does not exist in run 1 — `plan.md` §7 records why), and returning any distinguishable
response for a taken email "just for better UX". The second is FR-08, and the story it breaks is
not this one.

## Report back

Write the report to `docs/stories/auth-epic/reports/be-auth-credentials.md`.

1. **Changed:** files touched, and what each change does
2. **Verified:** commands run and their actual results, with a scenario → test → result line for
   every row in the scenarios table
3. **Could not do:** anything blocked, and why
4. **Noticed:** problems outside your scope. Report them, don't fix them.
