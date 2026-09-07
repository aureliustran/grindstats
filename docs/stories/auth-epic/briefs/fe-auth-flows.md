# Executor brief: fe-auth-flows

**Slice ID:** `fe-auth-flows`
**Story / spec:** [AUTH-001](../../AUTH-001-auth-registration/story.md) (the screens its
scenarios need) · [AUTH-002](../../AUTH-002-auth-login/story.md) ·
[AUTH-003](../../AUTH-003-auth-session-refresh-logout/story.md) (session lifecycle in the shell)
**Domain:** frontend
**Depends on:** `plat-audit-model`, `fe-auth-client` — both must have reported done
**Load skill:** `multi-agent-execution` (`.claude/skills/multi-agent-execution/SKILL.md`)
**Read first:** [`docs/frontend.md`](../../../frontend.md) (all of it) ·
[`docs/design-system.md`](../../../design-system.md) (§7 is your definition of done) ·
[`docs/i18n-guidelines.md`](../../../i18n-guidelines.md) (§7 likewise) ·
[`../contract.md`](../contract.md) §1, §8

## Task

Build the client-side flows AUTH-001 needs that LAND-001's modal does not cover, and teach the
shell about the session states run 1 introduces.

Four screens, one banner, and the routes that reach them:

1. **Verify email** — lands from the emailed link with a token in the URL, calls `verifyEmail`,
   shows success or the "this link is invalid or expired" state.
2. **Request a password reset** — email field → `requestPasswordReset` → a neutral confirmation
   that is **identical whether or not the address exists** (FR-08). The copy must not say
   "check your inbox, we found your account"; it says a reset link has been sent if an account
   exists.
3. **Set a new password** — token from the URL + a new password → `confirmPasswordReset`, then
   send the user to log in. Every existing session was just killed (FR-07/FR-33), so there is
   nothing to auto-log-in to.
4. **Confirm an OAuth link** — token from the emailed link → `confirmOAuthLink`, then log in.
   Reached after `/?auth_error=oauth_link_required` (contract §4.3).
5. **Unverified banner** — a persistent, dismissible-per-session notice for an authenticated but
   unverified account, saying what verifying unlocks, and the disabled state on write controls
   that the server would reject with `AUTH_EMAIL_UNVERIFIED`.

Plus the shell work: routes for the above, `?auth_error=` handling extended with
`oauth_link_required` and `oauth_failed`, and `AuthContext` carrying `emailVerified` so the
banner and disabled states have something to read.

## You own (exclusive write access)

- `apps/web/src/features/auth/**` (new feature slice)
- `apps/web/src/app/**` (`Router.tsx`, `AuthContext.tsx`, `pages/`, `ErrorBoundary.tsx`)
- `apps/web/src/i18n/locales/en-US.json`
- `apps/web/src/i18n/locales/vi-VN.json`

## Read-only context

- `docs/stories/auth-epic/contract.md` §1 (what each endpoint returns and can fail with), §8.1
  (the `AuthApi` methods you call), §8.3 (i18n ownership — you own the SPA catalogs)
- `apps/web/src/api/auth.ts` + `auth.types.ts` — the client `fe-auth-client` just finished
- `apps/web/src/i18n/errorMessages.ts` — the code→key map. It already references
  `errors.auth_email_unverified` and `errors.auth_link_invalid`; **you add those two keys** to
  both catalogs (contract §8.3).
- `apps/web/src/features/landing/**` — read `AuthModal.tsx` for the established form patterns,
  loading states and error rendering. **Do not modify it or import from it** (`frontend.md` §3:
  no sideways imports between slices — if something is genuinely shared it moves up, and moving
  it is not in this slice's scope, so copy the pattern rather than the module).
- `apps/web/src/styles/tokens.css` — every design value you may use

## Do not touch

- `apps/web/src/features/landing/**` — frozen this run
- `apps/web/src/api/**` — `fe-auth-client` and `plat-audit-model` own it
- `apps/web/src/i18n/errorMessages.ts` and `apps/web/src/i18n/voice-critical-keys.json` —
  not yours
- `apps/web/src/styles/**` — if a token you need doesn't exist, that is a real gap: report it,
  and use the nearest existing token rather than a raw value
- `package.json` / `package-lock.json`
- Anything under `services/` or `libs/`

## Contract you implement against

### Endpoints you call (contract §1)

| Call | Success | Failure you must render |
|---|---|---|
| `verifyEmail({token})` | 200 `{status:"verified"}` | 400 `AUTH_LINK_INVALID` → "this link is invalid, expired, or already used", with a path forward |
| `requestPasswordReset({email})` | 202 `{status:"sent"}` **always** | 429 `AUTH_RATE_LIMITED` → the countdown pattern LAND-001 established |
| `confirmPasswordReset({token, password})` | 200 `{status:"reset"}` | 400 `AUTH_LINK_INVALID`; 400 `VALIDATION_FAILED` with `details[].rule` ∈ `min_length` / `breached` |
| `confirmOAuthLink({token})` | 200 `{status:"linked"}` | 400 `AUTH_LINK_INVALID` |

`VALIDATION_FAILED` may carry `details: [{field, rule}]`, and may not — clients must tolerate its
absence (contract §1.1). Map `rule` to your own copy; **do not render the server's `message`**.
`renderServerMessage()` is not permitted anywhere in this slice: no story clause allows it, and
the unknown-code fallback in `errorMessages.ts` already handles the case it exists for.

### The rules that are checked, not just recommended

- **No hardcoded user-facing string.** Not a label, error, `aria-label`, `alt`, `title` or
  `placeholder`. Every one resolves through a key present in **both** catalogs.
- **Every number goes through `Intl.NumberFormat`** with the active locale — including a
  rate-limit countdown. `vi-VN` inverts the separators; a raw `toFixed()` in the DOM is a defect.
- **Every design value comes from `tokens.css`.** No raw hex, px or ms in a component.
- **Layouts are designed for Vietnamese**, which runs 10–30% longer and stacks diacritics
  vertically. Check every screen at `vi-VN` before calling it done; an English-only pass is not a
  finished design.
- Client-side validation before any request: email shape, password ≥ 10
  (`PASSWORD_MIN_LENGTH`, `EMAIL_PATTERN` are exported from `auth.types.ts`). Submit stays
  disabled until both pass.

### Password-reset copy and FR-08

The confirmation after `requestPasswordReset` is the one piece of copy in this slice with a
security requirement attached. It must read the same for a registered and an unregistered
address, and it must not imply the account was found. Write it once, use it for every outcome,
and do not add a "we couldn't find that email" branch — there is no response that would tell you
that, by design.

## Acceptance-criteria scenarios this slice covers with automated tests

| Scenario | Test cases | Side | Test kind | Location |
|---|---|---|---|---|
| unverified account is read-only on its own data | AUTH-001 TC-09 | client | component — an authenticated, unverified session renders the banner (asserted by translation key) and the write control is `disabled`; a verified session renders neither | `apps/web/src/features/auth/UnverifiedBanner.test.tsx` |

Title the `it(...)` with the scenario verbatim (`frontend.md` §7) — the phase-3 audit matches
titles to `acceptance-criteria.md`. Assert **by translation key**, using a test i18n instance
whose `t` returns the key: `screen.getByText("auth.unverified.banner")`, never English copy.

Also required, as this slice's own conformance tests:

- verify-email: success state, and `AUTH_LINK_INVALID` rendering its key with a way forward
- password-reset request: the response state is identical for two different emails — assert the
  same key renders for both (this is FR-08 as a client test)
- password-reset confirm: `VALIDATION_FAILED` with `rule: "breached"` and with
  `rule: "min_length"` render distinct, correct keys; with `details` absent, a generic key
- both locales: every screen renders with `vi-VN` active, with numbers formatted through the
  shared `Intl` helpers
- routes: each new path renders its screen, and a token-less visit to a token-consuming route
  renders the invalid-link state rather than calling the API

## Done when

- [ ] The scenario above has a passing test named after it
- [ ] All five surfaces exist and are reachable by route
- [ ] Every conformance test in the list above passes
- [ ] Both catalogs contain every new key, including `errors.auth_email_unverified` and
      `errors.auth_link_invalid`, and parity passes — the check is in
      `docs/stories/LAND-001-public-landing-page/contract.md` §5 (Node one-liner; Python may not
      be installed here)
- [ ] No raw hex/px/ms in any component you wrote; no hardcoded user-facing string; no
      `renderServerMessage()` call
- [ ] `docs/design-system.md` §7 and `docs/i18n-guidelines.md` §7 walked, with anything you could
      not satisfy named in your report
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

The tempting one here is adding a field to an endpoint's response because a screen would be
nicer with it — a resend-verification endpoint, or a "was this address registered" flag. The
first does not exist in run 1 (see `plan.md` §7); the second cannot exist without breaking FR-08.

## Report back

Write the report to `docs/stories/auth-epic/reports/fe-auth-flows.md`.

1. **Changed:** files touched, and what each change does
2. **Verified:** commands run and their actual results, with a scenario → test → result line for
   the scenarios table and each conformance test
3. **Could not do:** anything blocked, and why — including any design-system or i18n check you
   could not satisfy
4. **Noticed:** problems outside your scope. Report them, don't fix them.
