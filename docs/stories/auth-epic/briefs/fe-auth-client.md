# Executor brief: fe-auth-client

**Slice ID:** `fe-auth-client`
**Story / spec:** [AUTH-002](../../AUTH-002-auth-login/story.md) ·
[AUTH-003](../../AUTH-003-auth-session-refresh-logout/story.md) ·
[LAND-001 contract](../../LAND-001-public-landing-page/contract.md) (the shape you are replacing)
**Domain:** frontend
**Depends on:** `be-wiring`'s **pre-step** only (it installs and configures the Vitest +
Testing Library tooling your done-criteria depend on — see
[AMD-001](../amendments/AMD-001-frontend-test-tooling.md)). No other slice.
**Load skill:** `multi-agent-execution` (`.claude/skills/multi-agent-execution/SKILL.md`)
**Read first:** [`docs/frontend.md`](../../../frontend.md) §6, §7 ·
[`docs/shared-contract.md`](../../../shared-contract.md) §3 ·
[`../contract.md`](../contract.md) §1, §2.3, §8

## Task

Replace the LAND-001 in-memory mock with a real HTTP client behind the same `AuthApi` interface,
and extend that interface with run 1's four new endpoints.

The whole point of LAND-001's design was that this change touches **one file's implementation and
no component**. Keep that true: if you find yourself editing something under `features/` or
`app/`, you are in another slice's territory and the seam was wrong — escalate rather than reach.

You do not wait for the backend. You implement against the frozen HTTP contract and test against
a mocked `fetch`. When the server appears in wave 4, either it matches the contract or the
testing phase has a contract defect to file — which is the entire reason both sides are written
against a document rather than against each other.

## You own (exclusive write access)

- `apps/web/src/api/auth.ts`
- `apps/web/src/api/auth.types.ts`
- `apps/web/src/api/http.ts` (new — the fetch wrapper)
- `apps/web/src/api/mock/**`
- `apps/web/src/api/*.test.ts`

## Read-only context

- `docs/stories/auth-epic/contract.md` §1 (every endpoint, request, success and error), §2.3
  (CSRF), §8 (your TypeScript surface and client obligations)
- `docs/stories/LAND-001-public-landing-page/contract.md` §1–§3 — what the mock does today and
  why; §0.4 (the `/users/me` amendment you must preserve)
- `apps/web/src/i18n/errorMessages.ts` and `apps/web/src/api/generated/audit.ts` — error codes
  and the SPA's code→key policy. **`plat-audit-model` is editing both in parallel**, additively.
- `apps/web/src/app/AuthContext.tsx`, `apps/web/src/features/landing/AuthModal.tsx` — read them
  to confirm your changes are invisible to both. Do not edit either.

## Do not touch

- `apps/web/src/api/generated/**` — owned by `plat-audit-model`
- `apps/web/src/i18n/**` — `errorMessages.ts` is `plat-audit-model`'s, the catalogs are
  `fe-auth-flows`'s
- `apps/web/src/app/**` and `apps/web/src/features/**` — `fe-auth-flows` owns `app/`; the
  landing slice is frozen this run
- `package.json` / `package-lock.json` — owned by `be-wiring`; you need no new dependency
- Any file under `services/` or `libs/`

## Contract you implement against

### Transport rules (contract §8.2)

- `credentials: "include"` and `Accept-Language: <i18n.resolvedLanguage>` on **every** request
- `X-CSRF-Token` on every unsafe request **except** the unauthenticated auth endpoints
  (`register`, `verify-email`, both `password-reset/*`, `oauth/link/confirm`) and **except**
  `/auth/refresh` (contract D7 — refresh cannot require a token the SPA does not hold yet after
  a reload; the path-scoped `SameSite=Lax` cookie is what protects it instead)
- The SPA never reads a cookie and never stores a token. The CSRF token lives in memory.

### Envelope (contract D5) — the change that will bite quietly

Every success body is now `{"data": ...}`; every error body is `{"error": {code, message}}`.
LAND-001 specified bare bodies, and this contract supersedes it. Unwrap `data` **inside this
file**, so `login()` still resolves `{csrf_token}` and `me()` still resolves
`SessionInfo | null`. No caller learns the envelope exists.

Update the **mock** to envelope its responses too. It stays behind `VITE_AUTH_MOCK`; the real
client becomes the default when that flag is unset. Mock and real must remain swap-compatible —
that property is what lets `fe-auth-flows` build screens before any server exists.

### Endpoints (contract §1)

| Method on `AuthApi` | Call | Success | Notable errors |
|---|---|---|---|
| `register` | `POST /auth/register` | 202 `{data:{status:"pending_verification"}}` | 400 `VALIDATION_FAILED`, 429 `AUTH_RATE_LIMITED` |
| `login` | `POST /auth/login` | 200 `{data:{csrf_token}}` | 401 `AUTH_INVALID_CREDENTIALS`, 403 `AUTH_ACCOUNT_SUSPENDED`, 429 + `Retry-After` |
| `logout` | `POST /auth/logout` (CSRF) | 204 | 401, 403 `AUTH_CSRF_FAILED` |
| `me` | `GET /users/me` | 200 `{data:{user, csrf_token}}` | **401 resolves `null`, never throws** |
| `googleAuthorizeUrl` | — | `/api/v1/auth/oauth/google` | never fetched; the browser navigates |
| `verifyEmail` | `POST /auth/verify-email` | 200 `{data:{status:"verified"}}` | 400 `AUTH_LINK_INVALID` |
| `requestPasswordReset` | `POST /auth/password-reset/request` | 202 `{data:{status:"sent"}}` | 429 |
| `confirmPasswordReset` | `POST /auth/password-reset/confirm` | 200 `{data:{status:"reset"}}` | 400 `AUTH_LINK_INVALID`, 400 `VALIDATION_FAILED` |
| `confirmOAuthLink` | `POST /auth/oauth/link/confirm` | 200 `{data:{status:"linked"}}` | 400 `AUTH_LINK_INVALID` |

Type additions are given verbatim in contract §8.1, including `ValidationDetail` and
`ApiError.details?`. The five existing method signatures do not change.

### Single-flight reactive refresh (contract §8.2) — the subtle part

A `401` carrying `AUTH_INVALID_TOKEN` on any call triggers **at most one** in-flight
`POST /auth/refresh`. Concurrent callers await that same promise; when it resolves, each retries
its original request **exactly once**. If the refresh itself fails, every waiter resolves as
unauthenticated and **nothing retries**.

That last rule is not defensive coding, it is FR-31: a second attempt with an already-rotated
refresh token is precisely what the server treats as a stolen token, and it would kill every
session the user has. There is no proactive refresh timer in run 1.

`401 AUTH_SESSION_EXPIRED` is terminal — no refresh attempt, resolve as unauthenticated.

## Acceptance-criteria scenarios this slice covers with automated tests

| Scenario | Test cases | Side | Test kind | Location |
|---|---|---|---|---|
| successful login issues cookie-only tokens and a CSRF token | AUTH-002 TC-01 | client | unit — the CSRF token from the body is retained in memory and echoed on the next unsafe call; nothing is written to `localStorage`/`sessionStorage` by the real client, and no code path reads `document.cookie` | `apps/web/src/api/auth.test.ts` |

Title the `it(...)` with the scenario verbatim (`docs/frontend.md` §7) — the phase-3 coverage
audit matches titles against `acceptance-criteria.md`.

Beyond that scenario, these are contract-conformance obligations and must each have a test:

- envelope unwrapping for a success body, and error-envelope parsing into `ApiFailure{kind:"http"}`
- `Accept-Language` and `credentials: "include"` present on every request
- `X-CSRF-Token` present on `logout`, absent on `refresh` and on the unauthenticated endpoints
- `me()` resolves `null` on 401 rather than throwing
- `Retry-After` → `retryAfterSeconds`, defaulting to 30 when the header is absent
- transport failure and timeout → `ApiFailure{kind:"network"}`
- single-flight refresh: two concurrent 401s produce exactly **one** `/auth/refresh` call, both
  originals retry once, and a failed refresh produces **zero** retries
- the mock, with `VITE_AUTH_MOCK=true`, returns the same unwrapped shapes as the real client for
  every method

## Done when

- [ ] The scenario above has a passing test named after it
- [ ] Every conformance obligation in the list above has a passing test
- [ ] `AuthApi` carries all nine methods; the original five have unchanged signatures
- [ ] No component or context file was modified — `git diff --stat` shows changes only under
      `apps/web/src/api/` (excluding `generated/`)
- [ ] Type-checks: `cd apps/web && npx tsc --noEmit`
- [ ] Tests pass: `cd apps/web && npx vitest run`
- [ ] Builds: `cd apps/web && npm run build`
- [ ] Nothing modified outside the allowlist above (check your own diff)

## Stop condition — read this before starting

If you find the contract wrong, ambiguous, or insufficient for this task:

> **Stop work on the affected surface. Write an amendment request using
> `.claude/skills/multi-agent-execution/assets/amendment-request-template.md`. Do not proceed on
> that surface.**

Do not fix the contract yourself, and do not implement around it. Other slices are being built
against the current version right now; a local fix produces two implementations that each
believe they conformed, and nothing detects the mismatch until runtime.

Specifically: if you conclude the server *should* return a field it doesn't, or that refresh
*should* be proactive, those are amendment requests. A client that compensates for a shape it
thinks the server has is the exact failure the contract freeze exists to prevent.

## Report back

Write the report to `docs/stories/auth-epic/reports/fe-auth-client.md`.

1. **Changed:** files touched, and what each change does
2. **Verified:** commands run and their actual results, with a scenario → test → result line for
   the scenarios table and for each conformance obligation
3. **Could not do:** anything blocked, and why
4. **Noticed:** problems outside your scope. Report them, don't fix them.
