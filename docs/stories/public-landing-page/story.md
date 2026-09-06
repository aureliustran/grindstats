# Public landing page for unauthenticated visitors

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
- **Likeness constraint:** the silhouette is an original GrindStats asset. It must not
  depict, be modeled on, or be recognizable as any real person or existing fictional
  character. This is a hard requirement on the asset, not a style preference.

## Open questions

- **Pricing section:** currently specced as an on-page anchor with placeholder tiers.
  Should the nav item be hidden entirely until subscription-service exists (Phase 9),
  rather than pointing at placeholder content?
- **Session length control:** SRS-AUTH-001 fixes refresh lifetimes at 30 days absolute /
  14 days idle with no user control. Do we want a "keep me signed in" affordance at
  login, which would require an SRS revision, or leave it out?
