# Executor brief: LAND-001-fe-shell

**Slice ID:** LAND-001-fe-shell
**Story / spec:** `docs/stories/LAND-001-public-landing-page/story.md` (+ `acceptance-criteria.md`, `test-cases.md`)
**Domain:** frontend (shell + API client)
**Depends on:** none — may start immediately. Runs in parallel with `LAND-001-fe-landing`.
**Read first:** `docs/shared-contract.md`, `docs/frontend.md`,
`.claude/skills/multi-agent-code-execution/references/frontend-microfrontend.md`,
`docs/stories/LAND-001-public-landing-page/contract.md` (the frozen feature contract),
`docs/design-system.md` §4, `docs/i18n-guidelines.md` §5.

Repo root: `D:\PROJECTS\grindstats`. App: `apps/web` (Vite + React 18 + TS, already
scaffolded; `npm install` has been run). No Python on this machine.

## Task

Build the application shell for one Vite SPA and the **mock** auth client, so that the
landing feature (built by another executor at the same time, against the same contract)
plugs in with zero changes:

1. **`src/api/auth.ts`** — replace the placeholder with the in-memory mock implementing
   `AuthApi` exactly as `contract.md` §3 specifies (seed users, sessionStorage-backed
   session and user store, ~400–700 ms latency, 202-always register, indistinguishable 401s,
   5-failure → 429 with `retryAfterSeconds: 30` for 30 s, offline switch via
   `sessionStorage gs.mock.offline === "1"` or `?mock=offline`, `googleAuthorizeUrl()` →
   `/__mock/oauth/google`). Throw `ApiRequestError` for failures. Honour `VITE_AUTH_MOCK`
   (unset/"true" → mock; anything else → an implementation whose methods throw
   "not implemented" — do **not** write a real HTTP client). You may split internals into
   `src/api/mock/`.
2. **`src/app/`** — routing, layout and auth context:
   - `AuthProvider` + `useAuth()`: boot-time `authApi.me()` probe → state
     `{ status: "loading" | "anonymous" | "authenticated", user?, csrfToken? }`;
     `markAuthenticated({csrfToken})` (then re-probe `me()` to fetch the user), `logout()`
     (calls `authApi.logout(csrfToken)`, clears state).
   - Routes (react-router-dom v6, `createBrowserRouter` or `BrowserRouter`):
     `/` → while `loading` render `common.loading` (never the landing page); if
     `authenticated` → `<Navigate to="/dashboard" replace />`; else
     `<LandingPage onAuthenticated notice onNoticeDismiss />` where `notice` is derived
     from `?auth_error=oauth_cancelled` and `onNoticeDismiss` removes that param
     (`setSearchParams`, replace).
     `/dashboard` → protected placeholder page: `app.dashboard.heading`,
     `app.dashboard.signed_in_as` (email), `app.dashboard.placeholder`, a Log out button
     (`app.dashboard.logout`) that calls `logout()` then navigates to `/`. Unauthenticated →
     `<Navigate to="/" replace />`.
     `/__mock/oauth/google` → mock consent page (`app.mock_oauth.*`): Approve → mock creates
     /finds `google.user@example.com`, writes session, then `markAuthenticated` + navigate
     `/dashboard`; Deny → navigate `/?auth_error=oauth_cancelled`. Put the "write a Google
     session" helper inside `src/api/mock/` and export it from `src/api/auth.ts` as a
     clearly-named mock-only export (e.g. `mockOAuth`), so the page doesn't reach into
     storage keys directly. Register this route only when the mock is active.
     `*` → `app.not_found.*` with a link back to `/`.
   - A root `ErrorBoundary` (message via existing `common.*` keys or `app.not_found.*`; do
     not add keys).
   - Document title: set from `landing.meta.title` on `/` and `common.app_name` elsewhere.
     `<html lang>` is already handled by `src/i18n/index.ts` — don't duplicate.
3. **`src/App.tsx`** — mount the router inside `AuthProvider`.
4. **`apps/web/.env.example`** — document `VITE_AUTH_MOCK` (names only, no secrets).

Style: Tailwind utilities mapped to tokens (`bg-paper-1`, `text-ink-1`, `p-4` …) —
no raw values. Placeholder pages should be plain, deadpan, and correct in both themes and
at `?lng=vi`. Every string is a `t()` key that already exists (`common.*`, `app.*`,
`landing.meta.title`). Do not add keys — list any you'd want in your report.

## You own (exclusive write access)

- `apps/web/src/app/` (create)
- `apps/web/src/api/auth.ts` (replace placeholder body; keep the `authApi: AuthApi` export)
- `apps/web/src/api/mock/` (create, optional)
- `apps/web/src/App.tsx`
- `apps/web/.env.example` (create)

## Read-only context

- `apps/web/src/api/auth.types.ts` — the contract types (frozen)
- `apps/web/src/features/landing/types.ts`, `apps/web/src/features/landing/index.ts` —
  the seam; import `LandingPage` from `"../features/landing"` (the other executor replaces
  the placeholder body; the export name and props will not change)
- `apps/web/src/i18n/*`, `apps/web/src/styles/*`, `apps/web/tailwind.config.js`
- `docs/stories/LAND-001-public-landing-page/contract.md`

## Do not touch

- `apps/web/src/features/**` — owned by `LAND-001-fe-landing`
- `apps/web/src/i18n/locales/*.json` — owned by `LAND-001-fe-landing` this run
- `apps/web/src/styles/tokens.css`, `globals.css` — frozen (landing slice may add tokens; you may not)
- `apps/web/src/api/auth.types.ts`, `apps/web/src/api/generated/**`, `apps/web/src/i18n/errorMessages.ts`
- `apps/web/package.json` — do not add dependencies; everything needed is installed
  (react-router-dom, i18next, react-i18next). If you believe you need one, report it.
- Any file under `docs/`

## Contract you implement against

`docs/stories/LAND-001-public-landing-page/contract.md` §1–§4 in full, and
`apps/web/src/api/auth.types.ts`. Key points restated:

- `register` → `{status:"pending_verification"}`, never a session, identical for existing
  emails. `login` → `{csrf_token}`; failures throw `ApiRequestError` with
  `{kind:"http", status, error:{code, message}, retryAfterSeconds?}`; transport failure →
  `{kind:"network"}`. `me()` → `CurrentUser | null` (null, not throw, on 401).
- Error codes are only the generated `ErrorCode` union.
- Seam props: `onAuthenticated({csrfToken})`, `notice?: "oauth_cancelled" | null`,
  `onNoticeDismiss?()`.
- Section ids `features`, `how-it-works`, `pricing`; main landmark id `main` (landing owns
  these; you only need to know the landing page renders its own `<main id="main">`).

## Done when

- [ ] `cd apps/web && npm run build` passes (tsc + vite) with the landing placeholder in place
- [ ] `npm run dev`: `/` shows `common.loading` briefly then the (placeholder) landing render; `/dashboard` redirects to `/` when anonymous
- [ ] Mock: in the browser console, `await (await import('/src/api/auth.ts')).authApi.login({email:'returning@example.com',password:'Str0ng-Passw0rd!'})` resolves `{csrf_token}`; wrong password rejects with `AUTH_INVALID_CREDENTIALS`; 5 wrong attempts → `AUTH_RATE_LIMITED` with `retryAfterSeconds: 30`; `register` for `taken@example.com` and a new email both resolve `{status:"pending_verification"}`; `?mock=offline` makes calls reject with `{kind:"network"}`
- [ ] After a successful mock login + `markAuthenticated`, reloading `/` redirects to `/dashboard` without the landing page flashing; Log out returns to `/` and `me()` is null
- [ ] `/__mock/oauth/google` Approve lands on `/dashboard`; Deny lands on `/?auth_error=oauth_cancelled`
- [ ] All shell pages readable in light and dark theme and at `?lng=vi`; no raw hex/px/ms; no hardcoded user-facing strings (grep your files for quoted English)
- [ ] `git status` shows changes only inside the allowlist

## Stop condition — read this before starting

If you find the contract wrong, ambiguous, or insufficient for this task:

> **Stop work on the affected surface. Write an amendment request using
> `.claude/skills/multi-agent-code-execution/assets/amendment-request-template.md`
> (save it as `docs/stories/LAND-001-public-landing-page/briefs/AMENDMENT-fe-shell.md`).
> Do not proceed on that surface.**

Do not fix the contract yourself, and do not implement around it. Continue any part of
your slice that doesn't depend on the disputed shape.

## Report back

1. **Changed:** files touched, and what each change does
2. **Verified:** which commands you ran and their results — not what you expect to pass
3. **Could not do:** anything blocked, and why
4. **Noticed:** problems outside your scope. Report them, don't fix them.
