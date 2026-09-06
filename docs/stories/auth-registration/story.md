# Account registration

**As an** unauthenticated visitor who has decided to use GrindStats
**I want** to create an account with email + password, or with my Google account, and
verify it's really me
**So that** I can start tracking my own data under an identity only I control

## Description

Registration is the front door referenced by the public-landing-page story's auth modal
(`docs/stories/public-landing-page/`), and this story specifies what happens once the
visitor submits the Sign up tab or completes Google consent: password acceptance rules,
role safety, email verification, account-enumeration protection, OAuth-to-local account
linking, and password reset. This story deliberately does **not** cover what happens after
an account exists and the visitor comes back to sign in — that's `auth-login`. Source:
SRS-AUTH-001 §3.1 (FR-01..08).

The single rule everything else here defends: **a registrant can never end up with any
role but `User`.** No request field, header, or payload value influences role assignment
(FR-04) — `SystemAdmin` is granted only by an existing SystemAdmin through the admin story,
never through any path a visitor controls.

## In scope

- Email + password registration, email uniqueness (case-insensitive)
- Password acceptance: ≥10 characters, checked against a breached-password list (HIBP
  k-anonymity), no mandatory composition rules
- Google OAuth2 registration (authorization-code flow + PKCE), auto-creating a `User`
  account on first login
- Linking an OAuth login to an existing local account of the same email, requiring proof
  of control (login or verified-email confirmation) rather than silent merge
- Email verification: single-use token, 24h expiry; unverified accounts may log in but are
  read-only on their own data until verified
- Password reset: request → single-use emailed token (1h expiry) → new password; completing
  a reset sets the user's blacklist epoch (logs out everywhere)
- Account-enumeration protection across registration, login, and reset endpoints —
  identical responses whether or not an email exists
- Role safety: every registration path produces role `User`, unconditionally

## Out of scope

- What happens at login for an already-registered account (`auth-login`)
- Session/token issuance mechanics beyond "an account now exists" (`auth-login`)
- Listing or revoking sessions (`auth-session-management`)
- SystemAdmin granting or revoking roles (`auth-admin-account-management`)
- OAuth *linking* of non-identity accounts (Qwen API key, POS) — subscription domain,
  blueprint §8
- Apple OAuth (blueprint notes Apple "later"; Google only for now)
- Email delivery infrastructure itself (assumed to exist and be reliable)

## Dependencies / assumptions

- `POST /api/v1/auth/register`, `GET /api/v1/auth/oauth/google[/callback]`,
  `POST /api/v1/auth/verify-email`, `POST /api/v1/auth/password-reset/request`,
  `POST /api/v1/auth/password-reset/confirm` (SRS-AUTH-001 §3.7)
- Passwords hashed with argon2id (bcrypt cost ≥12 fallback), never logged (SEC-01)
- Verification and reset tokens are single-use and hashed at rest (SEC-04)
- OAuth uses `state` + PKCE with an exact-match redirect URI allowlist (SEC-05)
- Completing a password reset triggers `auth-session-refresh-logout`'s logout-all behavior
  (FR-33) — this story specifies that it happens, not how logout-all itself works
- An HIBP-style breached-password check requires outbound network access from
  auth-service; local dev must not hard-depend on it being reachable (see Open questions)

## Open questions

- **HIBP check is Priority S ("reject or warn on match")** — which behavior ships first,
  a hard reject or a warning that lets registration proceed? The SRS leaves both on the
  table.
- **What does "read-only" mean operationally for an unverified account?** The SRS says
  unverified accounts are read-only on their own data (FR-03) but doesn't enumerate which
  write endpoints are blocked — is this enforced centrally in the gateway/authz middleware,
  or per-service?
- **Resending a verification email** isn't specified — is there a resend endpoint, a
  cooldown, and does registering again with the same unverified email resend rather than
  reject?
