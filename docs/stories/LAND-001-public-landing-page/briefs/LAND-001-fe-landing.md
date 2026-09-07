# Executor brief: LAND-001-fe-landing

**Slice ID:** LAND-001-fe-landing
**Story / spec:** `docs/stories/LAND-001-public-landing-page/story.md`,
`acceptance-criteria.md`, `test-cases.md`, `diagram.md`
**Domain:** frontend (feature slice)
**Depends on:** none — may start immediately. Runs in parallel with `LAND-001-fe-shell`.
**Read first:** `docs/shared-contract.md`, `docs/frontend.md`,
`.claude/skills/multi-agent-code-execution/references/frontend-microfrontend.md`,
`docs/stories/LAND-001-public-landing-page/contract.md` (frozen feature contract),
**`docs/design-system.md` and `docs/i18n-guidelines.md` in full** — they are binding on
every line you write. Then `apps/web/src/styles/tokens.css` and `globals.css` (read them
end to end; the utilities `.u-display`, `.u-num`, `.u-visually-hidden`, `.u-skip-link`,
`.u-content` and the keyframes `fade-in` / `modal-enter` already exist for you).

Repo root: `D:\PROJECTS\grindstats`. App: `apps/web` (Vite + React 18 + TS + Tailwind 3,
scaffolded, `npm install` done). No Python on this machine.

## Task

Build the public landing page as a feature slice at `apps/web/src/features/landing/`,
exported as `LandingPage(props: LandingPageProps)` from `index.ts` (replace the
placeholder; **keep the export name and the `types.ts` file unchanged**).

Contents, all per `story.md` / `acceptance-criteria.md`:

1. **Header** — sticky; brand (`common.app_name`); anchors to `#features`, `#how-it-works`,
   `#pricing` (`landing.nav.*`); **Log in** (`landing.nav.login`) and **Get started**
   (`landing.nav.get_started`, the single accent-colored control in the header). Below the
   `md` breakpoint the anchors + auth actions collapse into a hamburger
   (`aria-expanded`, `aria-controls`, labels `landing.nav.menu_open` / `menu_close`), which
   closes on item select and on Escape. A skip link (`landing.nav.skip_to_content`) to
   `#main` is the first focusable element. No link anywhere targets an authenticated route.
2. **Hero** — `<main id="main">` opens with the four regimen lines
   (`landing.hero.regimen_*`) stacked in the display face (`.u-display`, `text-hero`), the
   punchline (`landing.hero.punchline`), the aside (`landing.hero.aside`), the subhead, and
   the primary CTA (`landing.hero.cta_primary`) which opens the auth modal on the Sign up
   tab. Behind the type: **an original silhouette figure** you draw as inline SVG
   (`Silhouette.tsx`): faceless, solid `currentColor` fill and/or outline linework only,
   a generic athletic figure mid-stride or mid-push-up. It must not be modeled on or
   recognizable as any real person or existing character (design-system §5). `alt` /
   `aria-label` = `landing.hero.silhouette_alt`, or `aria-hidden` if purely decorative with
   the alt text supplied elsewhere. Add `assets/PROVENANCE.md` stating it is an original
   GrindStats asset, hand-authored as SVG in this slice, derived from nothing.
3. **Sections** — `#features` (four items from `landing.features.*`), `#how-it-works`
   (three steps `landing.how_it_works.*`), `#pricing` (`landing.pricing.heading`,
   `placeholder`, CTA opening the modal on Sign up). Plain, severe, monochrome. Numbers in
   copy — none are required; if you render any, use `formatNumber` from `../../i18n`
   inside a `.u-num` element.
4. **Auth modal** (`AuthModal.tsx`) — `role="dialog"`, `aria-modal`, `aria-labelledby` →
   `landing.auth.dialog_label`; tabs `landing.auth.tab_login` / `tab_signup`
   (`role="tablist"` pattern); fields email + password with labels (`email_label`,
   `password_label`, `email_placeholder`); submit (`submit_login` / `submit_signup`);
   divider (`divider`); **Continue with Google** (`google_cta`) which does
   `window.location.assign(authApi.googleAuthorizeUrl())`. Behaviour:
   - Focus trap (Tab/Shift-Tab wrap), close on Escape / backdrop / close button
     (`common.close`), **return focus to the opener**; page scroll position untouched
     (do not unmount the page; lock body scroll while open if you like, via a class — no
     raw values). Use `useEffect` + refs; no new dependency.
   - Client-side validation before any request: email must match `EMAIL_PATTERN`,
     password length ≥ `PASSWORD_MIN_LENGTH` (both from `../../api/auth.types`). Field
     errors `landing.auth.error_email_invalid`, `error_password_short` (with `count`,
     formatted via `formatNumber`). Submit disabled until valid; no call made otherwise.
   - Login: `authApi.login` → on success call `props.onAuthenticated({ csrfToken:
     res.csrf_token })` and close. On `ApiRequestError`: map `failure.error.code` through
     `errorMessageKey()` from `../../i18n/errorMessages` (never `renderServerMessage`);
     clear the password, keep the email; `AUTH_RATE_LIMITED` additionally disables submit
     for `retryAfterSeconds ?? 30` seconds (countdown optional; if shown, through
     `formatNumber`); `{kind:"network"}` → `error_unreachable`, modal stays open, email kept.
     Unknown code → fall back to `failure.error.message` (the sanctioned unknown-code path).
   - Sign up: `authApi.register` → on 202 show `landing.auth.signup_next_step` in place of
     the form, with a way to switch to the Log in tab. Same error handling as login.
   - Switching tabs clears the other tab's errors and fields. Loading states disable the
     submit and always terminate (`common.loading` as accessible text if you show a spinner).
   - Modal enters with `modal-enter` (CSS handles reduced motion); any JS-driven motion or
     smooth scrolling must check `matchMedia("(prefers-reduced-motion: reduce)")`.
5. **Notice** — when `props.notice === "oauth_cancelled"`, render a non-blocking
   `role="status"` bar with `landing.auth.error_google_cancelled` and a dismiss button
   (`common.close`) that calls `props.onNoticeDismiss?.()`.
6. **Document title** is the shell's job; do not set it.

Import rule: only `../../styles/*` (nothing to import there beyond what's global),
`../../i18n/*`, `../../api/*`, and your own subtree. **Never** `../../app/*`, never
`react-router-dom`. Anchors are plain `<a href="#…">`.

i18n: all `landing.*` keys you need already exist in both catalogs — use them verbatim.
You may add further `landing.*` keys **to both catalogs in the same change** if genuinely
needed; do not alter existing wording (hero keys are voice-critical). Tokens: if a value
you need is missing from `tokens.css`, add the token there **with a comment**, never inline.

## You own (exclusive write access)

- `apps/web/src/features/landing/**` (except `types.ts`, which is frozen contract)
- `apps/web/src/i18n/locales/en-US.json` and `vi-VN.json` — additions under `landing.*` only
- `apps/web/src/styles/tokens.css` — additive token declarations only, each with a comment

## Read-only context

- `apps/web/src/api/auth.types.ts`, `apps/web/src/api/auth.ts` (placeholder body; the
  shell executor replaces it — you only import `authApi`), `apps/web/src/i18n/*`,
  `apps/web/src/styles/globals.css`, `apps/web/tailwind.config.js`
- `docs/stories/LAND-001-public-landing-page/*`

## Do not touch

- `apps/web/src/app/**`, `apps/web/src/App.tsx`, `apps/web/src/api/auth.ts`,
  `apps/web/src/api/mock/**` — owned by `LAND-001-fe-shell`
- `apps/web/src/features/landing/types.ts`, `apps/web/src/api/auth.types.ts` — frozen contract
- `apps/web/src/styles/globals.css`, `apps/web/src/i18n/index.ts`, `errorMessages.ts`,
  `voice-critical-keys.json`, `apps/web/package.json` (no new dependencies), anything under `docs/`

## Contract you implement against

`docs/stories/LAND-001-public-landing-page/contract.md` §1, §2, §4, §5, §6 and the two
type files. Restated essentials: `authApi.login → {csrf_token}`; `authApi.register →
{status:"pending_verification"}` always (show `signup_next_step`, never auto-login);
failures are `ApiRequestError` with `failure: {kind:"http", status, error:{code,message},
retryAfterSeconds?} | {kind:"network"}`; `authApi.googleAuthorizeUrl()` is a URL to
navigate to. Props: `onAuthenticated({csrfToken})`, `notice`, `onNoticeDismiss`.
Section ids `features`, `how-it-works`, `pricing`; main id `main`.

## Done when

- [ ] `cd apps/web && npm run build` passes (the placeholder `authApi` throws at runtime only; that is expected until the shell lands)
- [ ] To view your work before the shell exists, temporarily render `<LandingPage onAuthenticated={console.log} />` from `src/App.tsx` **in your local run only and revert it before reporting** — `App.tsx` is not yours
- [ ] AC scenarios verifiable without a server all pass by manual check: hero + menu content (TC-01), no authenticated links (TC-02), hamburger at 375px (TC-03), anchor scroll + reduced-motion (TC-04/05), modal open/close/focus-return/scroll kept (TC-10), focus trap both directions (TC-11), tab switch clears state (TC-12), client validation blocks requests (TC-17/18), dark theme (TC-21)
- [ ] `?lng=vi` at 375 / 768 / 1440 px: no clipped, truncated or overlapping text; stacked diacritics visible in the display face (TC-27/28)
- [ ] No raw hex / px / ms in your files: `grep -rnE "#[0-9a-fA-F]{3,8}\b|[0-9]px|[0-9]ms" apps/web/src/features/landing` returns nothing (SVG `viewBox`/path data excepted — keep the SVG geometry unitless)
- [ ] No hardcoded user-facing strings (grep your JSX for quoted English outside `t()`); `renderServerMessage` is not used
- [ ] i18n parity: run the Node one-liner from `contract.md` §5 from the repo root → `i18n parity ok`
- [ ] `git status` shows changes only inside the allowlist (and `App.tsx` reverted)

## Stop condition — read this before starting

If you find the contract wrong, ambiguous, or insufficient for this task:

> **Stop work on the affected surface. Write an amendment request using
> `.claude/skills/multi-agent-code-execution/assets/amendment-request-template.md`
> (save it as `docs/stories/LAND-001-public-landing-page/briefs/AMENDMENT-fe-landing.md`).
> Do not proceed on that surface.**

Do not fix the contract yourself, and do not implement around it. Continue any part of
your slice that doesn't depend on the disputed shape.

## Report back

1. **Changed:** files touched, and what each change does
2. **Verified:** which commands you ran and their results — not what you expect to pass;
   list which TC numbers you manually exercised
3. **Could not do:** anything blocked, and why
4. **Noticed:** problems outside your scope. Report them, don't fix them.
