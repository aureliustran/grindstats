# Executor brief: be-wiring

**Slice ID:** `be-wiring`
**Story / spec:** all three run-1 stories, at the point where they become one running system ·
SRS-AUTH-001 §6 (acceptance criteria 1–4 and 8), SEC-02, SEC-06 ·
[`docs/deployment-aws.md`](../../../deployment-aws.md) (read before touching anything AWS-shaped
— you should not need to)
**Domain:** backend
**Depends on:** every other slice in the run
**Load skill:** `multi-agent-execution` (`.claude/skills/multi-agent-execution/SKILL.md`)
**Read first:** [`docs/backend.md`](../../../backend.md) §1, §5, §7 ·
[`../plan.md`](../plan.md) §3 (the dependency-manifest note — the pre-step below is yours) ·
[`../contract.md`](../contract.md) §1, §2, §6

## Task

Two pieces of work, at opposite ends of the run.

### Pre-step — before wave 1 is dispatched

**Every other slice waits on this**, backend and frontend alike, because every brief's
done-criteria includes a test command. Two halves.

**(a) Go modules.** `go.mod` and `go.sum` are assigned to you precisely because every slice that
added a library would collide on them. The set is decided (`plan.md` §3):

| Need | Module |
|---|---|
| JWT RS256 | `github.com/golang-jwt/jwt/v5` |
| argon2id | `golang.org/x/crypto/argon2` (already indirect — promote it) |
| UUIDs | `github.com/google/uuid` |
| Redis test double | `github.com/alicebob/miniredis/v2` |
| OAuth2 + PKCE | `golang.org/x/oauth2` |

**(b) The frontend test runner, which does not exist yet.** Added by
[AMD-001](../amendments/AMD-001-frontend-test-tooling.md): `docs/frontend.md` §7 describes Vitest
and Testing Library in the present tense, but `apps/web` has no test runner, no `test` block in
`vite.config.ts`, and not one test file. Two frontend slices have `npx vitest run` in their
done-criteria and cannot install it themselves — the manifest is yours.

Install and **configure**:

- `vitest`, `jsdom`, `@testing-library/react`, `@testing-library/user-event`,
  `@testing-library/jest-dom` (devDependencies)
- a `test` block in `vite.config.ts`: `environment: "jsdom"`, `globals: true`, a setup file at
  `apps/web/src/test/setup.ts` importing `@testing-library/jest-dom`, and
  **`passWithNoTests: true`** — a green run with zero test files must not read as a failure while
  wave 1 is still writing the first one
- a `"test": "vitest run"` script in `package.json`
- `"types": ["vitest/globals"]` in `tsconfig.app.json` so `tsc --noEmit` accepts the globals

**(c) `python3` → `py`.** On this machine `python3` and `python` are Microsoft Store
execution-alias stubs; the real interpreter is `py` (Python 3.13.3). Fix `package.json`'s
`check-i18n` script accordingly. Every brief's done-criteria now says `py`, and the generator
check is mandatory rather than excusable — it runs here.

Then: `go mod tidy`, confirm `go build ./... && go test ./...` is green, confirm
`npx vitest run` and `npx tsc --noEmit` both execute, and **report the pre-step separately** so
the instructor can dispatch wave 1.

**Prove the frontend tooling actually works** — installing packages is not evidence that
`screen.getByText` resolves in this repo's TypeScript and jsdom setup. Write a throwaway test
that renders a trivial component and asserts on its text, run it, paste the real output into your
report, then delete it. A tooling pre-step that "should work" is how five slices discover at once
that it doesn't.

**Do nothing else at this point** — no `main.go`, no config, no route wiring. Everything below
waits for wave 3.

### Wave 4 — the composition root and the end-to-end proof

Assemble what nine slices built into one running binary, and prove the whole path works.

- `main.go`: construct the pool, the Redis client, the audit writer, the key set, the
  repositories, the two handler sets and the middleware, then call each slice's
  `Register(r gin.IRouter)` on the right route group. **Route grouping is yours**, and it is
  what makes the middleware's opt-outs work: the unauthenticated auth endpoints
  (`/auth/register`, `/auth/login`, `/auth/verify-email`, `/auth/password-reset/*`,
  `/auth/oauth/*`) sit in a group without the auth stage; everything else sits behind the full
  chain. `be-gateway-authz` was told explicitly not to maintain its own path list — this is why.
- `config`: the new env vars, with the names the other slices' READMEs and reports asked for.
  At minimum: JWT key paths, `AUTH_COOKIE_SECURE`, `AUTH_HIBP_ENABLED`, the Google OAuth client
  id/secret/redirect URI, the mailer mode, and the base URL used in emailed links.
- Local RS256 keys: a script that generates a keypair into a **git-ignored** path, and the
  `.gitignore` entry to match (SEC-06 — no secrets in the repo or an image). `.env.example`
  documents **names only**, never values.
- `docker-compose.yml`: whatever the auth stack needs beyond the existing Postgres and Redis —
  the env wiring, the key volume, `AUTH_COOKIE_SECURE=false` for plain-http local dev.
- The integration tests below.

## You own (exclusive write access)

- `services/monolith/cmd/server/main.go`
- `services/monolith/internal/platform/config/config.go` + `config_test.go`
- `services/monolith/internal/auth/integration/**`
- `docker-compose.yml`
- `.env.example`
- `scripts/dev/**`
- `go.mod`, `go.sum`
- `apps/web/package.json`, `apps/web/package-lock.json`
- `apps/web/vite.config.ts` *(pre-step only — AMD-001)*
- `apps/web/tsconfig.json`, `apps/web/tsconfig.app.json`, `apps/web/tsconfig.node.json`
  *(pre-step only — AMD-001)*
- `apps/web/src/test/**` — the Vitest setup file only. Tests live beside the code they test
  (`frontend.md` §7); this is not a tests directory. *(pre-step only — AMD-001)*
- `.gitignore`

## Read-only context

- Every slice's report under `docs/stories/auth-epic/reports/` — **read all nine before you
  start.** Their "Could not do" and "Noticed" sections are where you will find the env var
  somebody needed, the assertion that went unrun, and the mismatch nobody could fix from inside
  their allowlist.
- `docs/stories/auth-epic/contract.md` §1 (the full surface you are assembling), §2, §6
- `services/monolith/internal/auth/**` and `internal/gateway/**` — everything you wire
- `infra/db/README.md` — how migrations are applied, and any env var `db-migrations` asked for

## Do not touch

- Every source file another slice owns — this is the whole rest of the repo. If wiring reveals a
  bug inside a slice, **that is a defect report, not a fix.** Phase 3 files it and phase 4 fixes
  it inside the owning allowlist. A "quick fix" from the composition root is how a slice's brief
  stops describing what was built.
- Any document under `docs/`
- `apps/web/src/**`

## Contract you implement against

The whole of [`../contract.md`](../contract.md) §1 is now real and reachable at the paths it
names, behind the middleware chain in §6, with the cookie attributes in §2.2. Specifically, the
route groups must produce these facts, which your integration tests then assert:

- `/api/v1/auth/{register,login,verify-email,password-reset/request,password-reset/confirm,
  oauth/google,oauth/google/callback,oauth/link/confirm}` — reachable with **no** access token
- `/api/v1/auth/{logout,logout-all}` — access token **and** CSRF required
- `/api/v1/auth/refresh` — refresh cookie required, **CSRF exempt** (D7)
- `/api/v1/users/me` — access token required, no CSRF (safe method)
- `/healthz`, `/readyz` — unchanged from GATE-001, still unenveloped

## Acceptance-criteria scenarios this slice covers with automated tests

| Scenario | TCs | Story | Side | Kind | Location |
|---|---|---|---|---|---|
| (cross-endpoint enumeration regression) | TC-16 | AUTH-001 | server | integration — `POST /register` with a taken address and `POST /login` with a wrong password on an unknown address are each compared against their counterpart; neither pair differs in status, body or observable timing | `services/monolith/internal/auth/integration/enumeration_test.go` |

Plus the end-to-end path, which is the only place in run 1 where the whole system is live at
once. It is SRS-AUTH-001 §6's criteria 1–4 as a runnable test, and it belongs to no single slice:

```
register → consume the verification link → login
  → assert: two cookies with the contract's attributes, CSRF token in the body,
            and no token string anywhere in the body
  → authenticated GET /users/me
  → POST /auth/refresh → new pair, same sid
  → replay the pre-rotation refresh token → 401, and every session is now dead
  → (fresh login) unsafe request with and without X-CSRF-Token → 200 / 403
  → logout → the captured access cookie is rejected on the next call, without waiting for expiry
```

Location: `services/monolith/internal/auth/integration/e2e_test.go`, gated on
`GRINDSTATS_TEST_DB` plus a reachable Redis, so `go test ./...` stays fast by default and the
full run is one flag away (`backend.md` §7).

## Done when

- [ ] **Pre-step reported separately, before wave 1 is dispatched**, with:
      - `go build ./... && go test ./...` green
      - `cd apps/web && npx vitest run` executing (green, `passWithNoTests`) and
        `npx tsc --noEmit` clean
      - the throwaway render-and-assert test's **actual output pasted in the report**, and the
        file deleted afterwards
      - `npm run check-i18n` running under `py`
- [ ] The binary starts against compose and serves every endpoint in contract §1 at its path
- [ ] Route groups produce the four facts listed above; each is asserted
- [ ] TC-16 and the end-to-end test pass:
      `GRINDSTATS_TEST_DB=... go test -tags=integration ./services/monolith/internal/auth/integration/...`
- [ ] `go build ./... && go vet ./... && go test ./...` green without a database
- [ ] `cd apps/web && npx tsc --noEmit && npx vitest run && npm run build` green
- [ ] `py scripts/gen_audit_model.py --check` clean (the interpreter here is `py`, not `python3`)
- [ ] i18n parity passes (the Node one-liner in
      `docs/stories/LAND-001-public-landing-page/contract.md` §5)
- [ ] No secret, key or credential is committed; `.env.example` carries names only; the generated
      keypair path is git-ignored
- [ ] Migrations are **not** applied by application startup
- [ ] Nothing modified outside the allowlist above (check your own diff)

## Stop condition — read this before starting

If you find the contract wrong, ambiguous, or insufficient for this task:

> **Stop work on the affected surface. Write an amendment request using
> `.claude/skills/multi-agent-execution/assets/amendment-request-template.md`. Do not proceed on
> that surface.**

Do not fix the contract yourself, and do not implement around it. Other slices are being built
against the current version right now; a local fix produces two implementations that each
believe they conformed, and nothing detects the mismatch until runtime.

You are the slice most likely to break this rule, because you are the first to see everything
running and the failures will land in your terminal. **Wiring bugs are yours; slice bugs are
not.** If `be-auth-session`'s login sets the wrong cookie path, you record it — you do not open
that file. That discipline is what keeps the briefs an accurate record of what was built, and it
is what phases 3 and 4 exist to handle.

## Report back

Write **two** reports:

- `docs/stories/auth-epic/reports/be-wiring-prestep.md` — after the dependency pre-step
- `docs/stories/auth-epic/reports/be-wiring.md` — after wave 4

Each with:

1. **Changed:** files touched, and what each change does
2. **Verified:** commands run and their actual results, with a scenario → test → result line for
   TC-16 and each stage of the end-to-end path
3. **Could not do:** anything blocked, and why
4. **Noticed:** every defect you found while wiring, with enough detail for phase 3 to reproduce
   it and enough to name the slice you suspect. **Report them, don't fix them** — this section is
   the most valuable thing you produce.
