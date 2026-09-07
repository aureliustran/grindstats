# Public landing page for unauthenticated visitors

**Code:** `LAND-001`

**As an** unauthenticated visitor arriving at the GrindStats root URL
**I want** a landing page that states what the product is and lets me log in or create an account without leaving the page
**So that** I can understand the premise and start tracking in a single step, instead of bouncing between marketing and auth screens

## Description

The landing page is the only surface an unauthenticated visitor can reach — every
other route in GrindStats sits behind authentication. It therefore does two jobs at
once: communicate the product's premise, and convert. The premise is relentless daily
repetition, measured: the hero states a training regimen — **100 PUSH-UPS / 100 SIT-UPS
/ 100 SQUATS / 10KM RUN**, with **EVERY SINGLE DAY** as the punchline — rendered as
stacked, heavy, monochrome typography, with an original faceless silhouette figure
(solid black, outline linework only) behind it. That motif is not decoration: the daily
checklist it evokes is literally the feature GrindStats ships (see the routine
occurrence model, blueprint §6), so the hero is a promise the product keeps.

Authentication happens in a modal over the page rather than on separate routes, so a
visitor who has decided to sign up never loses the page that convinced them. The modal
offers email/password and "Continue with Google". The menu is marketing-only — a
visitor in this state has no app destinations, so none are shown.

## In scope

- Hero section: regimen typography, "EVERY SINGLE DAY" punchline, deadpan supporting
  aside, original silhouette figure, primary "Get started" call to action
- Supporting marketing sections: Features, How it works, Pricing (placeholder tiers)
- Header menu: anchor links to the above sections, plus Log in / Get started actions;
  collapses to a hamburger menu below the mobile breakpoint
- Auth modal with Log in and Sign up tabs, opened from any auth action on the page
- Email/password login and registration against auth-service
- "Continue with Google" OAuth2 entry point
- Client-side field validation, server error surfacing, and loading/disabled states
- Redirect to the dashboard on successful authentication
- Redirect straight to the dashboard if an already-authenticated visitor hits `/`
- Responsive layout, light/dark theming, keyboard accessibility, modal focus trap,
  and respect for `prefers-reduced-motion`

## Out of scope

- Password reset and email verification flows — their own stories
  (SRS-AUTH-001 FR-06, FR-07, FR-32)
- The dashboard itself, and every authenticated destination
- Real pricing data and subscription checkout — deferred to roadmap Phase 9
  (subscription-service); this story ships static placeholder tiers only
- CMS- or config-managed marketing copy; copy is hard-coded for now
- Social providers other than Google
- Native/mobile client behavior (no cookie jar) — see HANDOFF.md §12.2

## Dependencies / assumptions

- auth-service exposes `POST /api/v1/auth/register`, `POST /api/v1/auth/login`, and
  `GET /api/v1/auth/oauth/google` per SRS-AUTH-001 §3.7
- Tokens ride in httpOnly, Secure, SameSite=Lax cookies, so the SPA never reads a token
  directly; state-changing calls carry the double-submit `X-CSRF-Token` header
- The React SPA (`apps/web`) provides routing and an auth context under `src/app/`
- Roadmap sequencing: Phase 1 delivers a stubbed login; real JWT + Redis + Google OAuth2
  arrives in Phase 2. The landing page ships against the stub and swaps to real auth
  without markup changes — only the auth context implementation moves.
- **Silhouette:** an original, hand-authored GrindStats asset (see run 2 below and
  `docs/design-system.md` §5). The landing page also uses licensed third-party media
  (hero video, pricing art, Pinterest embed) alongside it.

## Implementation notes (run 2, 2026-09-06)

Full visual redesign ported from `template/` into the real app: glass-morphism nav/cards,
scroll-reveal, animated counters, a Numbers (worked-example) section, and an expanded
Pricing section — none of that touches the contract or the auth flow from run 1.

The hero backdrop is real *One Punch Man* footage, the Pricing section includes real
illustrated OPM character art, and the Quote section embeds a live Pinterest pin. All
three ship in `apps/web`, not just `template/` (see `docs/design-system.md` §5).

New tokens: `--glass-bg`, `--glass-border`, `--glass-shadow`, `--spot` (tokens.css), plus
`media-fg` / `media-scrim` in `tailwind.config.js` — fixed, non-themed colors, sanctioned
only for content overlaid on the hero's photo/video backdrop (see the comment there).

## Implementation notes (run 1, 2026-09-06)

Feature contract, scope decision and amendments: [`contract.md`](contract.md). Executor
briefs: [`briefs/`](briefs/). Delivered: the React slice (`apps/web/src/features/landing/`),
the shell (`apps/web/src/app/`) and a **mock** auth client (`apps/web/src/api/auth.ts`)
implementing the full SRS-AUTH-001 request/response shapes; no backend yet.

Story amendments recorded in the contract that affect the acceptance criteria above:

- **Registration does not auto-authenticate** (contract §0.1). SRS FR-08 wins over the
  "new visitor registers and reaches the dashboard" scenario: `POST /register` returns the
  same neutral 202 whether or not the email exists, the modal shows the "check your email"
  next step, and the visitor logs in via the Log in tab. TC-06 is satisfied through login.
- **Session probe** is `GET /api/v1/users/me` → `{ user, csrf_token }` (contract §0.2,
  §0.4) — not in SRS §3.7; needs adding to the SRS / user-service spec.
- **Google OAuth** is exercised against a stand-in consent page at `/__mock/oauth/google`
  until auth-service exists (contract §0.3). TC-08/09 pass against the stand-in.

## Open questions

- **Pricing section:** currently specced as an on-page anchor with placeholder tiers.
  Should the nav item be hidden entirely until subscription-service exists (Phase 9),
  rather than pointing at placeholder content?
- **Session length control:** SRS-AUTH-001 fixes refresh lifetimes at 30 days absolute /
  14 days idle with no user control. Do we want a "keep me signed in" affordance at
  login, which would require an SRS revision, or leave it out?
