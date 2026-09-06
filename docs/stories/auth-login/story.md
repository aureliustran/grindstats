# Login and session establishment

**As a** visitor with an existing GrindStats account
**I want** to log in with email + password or Google, and receive a secure session
**So that** I can access my own data without my credentials or tokens ever being exposed
to anything that could steal them

## Description

This story picks up where `auth-registration` leaves off: an account already exists, and
this specifies what happens when its owner returns to sign in. It covers token issuance and
transport, brute-force protection, and the CSRF pairing that state-changing requests will
need afterward. It does not cover what those tokens are checked against on every request —
that's request authorization, folded into this story and `auth-admin-account-management` as
cross-cutting acceptance criteria rather than its own story, since it has no user-facing
trigger of its own. Source: SRS-AUTH-001 §3.2 (FR-10..14) plus the token-check-order and
CSRF requirements of §3.3 (FR-20, FR-24) as they apply at login.

## In scope

- Login with email + password, and with Google OAuth, for an existing account
- Minting an access token (10–15 min) and refresh token (30 days absolute / 14 days idle),
  each with a unique `jti`, delivered as httpOnly/Secure/SameSite=Lax cookies
  (refresh cookie path-scoped to `/api/v1/auth`)
- Never exposing a token string in a response body, URL, or non-httpOnly storage
- Recording the new session in `user_sessions:{user_id}` (refresh `jti`, device label from
  User-Agent, created-at)
- Issuing a CSRF token bound to the session, returned in the response body for the SPA to
  hold in memory and echo via `X-CSRF-Token` on state-changing requests
- Per-account and per-source-IP failed-login counting, with exponential backoff (starting
  30s) after 5 consecutive failures — not a hard lockout
- Never disclosing which factor (email vs password) failed
- Account-enumeration protection consistent with `auth-registration`'s

## Out of scope

- Account creation and email verification (`auth-registration`)
- Refreshing an existing session, rotation, and logout (`auth-session-refresh-logout`)
- Listing or revoking sessions after login (`auth-session-management`)
- The full request-authorization check chain applied to every subsequent request
  (signature → expiry → blacklist → epoch) — specified once in
  `auth-session-refresh-logout`, since refresh is where that chain is most fully exercised;
  this story only covers what login itself establishes

## Dependencies / assumptions

- `POST /api/v1/auth/login`, `GET /api/v1/auth/oauth/google[/callback]` (SRS-AUTH-001 §3.7)
- The gateway is the only component that writes to or reads `user_sessions:{user_id}`
  and issues the CSRF token; downstream services never see raw credentials
- Rate limiting at the gateway (10/min login per NFR-04) is a distinct control from the
  5-failure backoff in this story — both apply, and this story only specifies the backoff
- An account created via `auth-registration` and left unverified can still log in (per that
  story); this story does not re-decide that, only issues the session

## Open questions

- **Backoff scope:** FR-13 counts failures "per account and per source IP" — if an attacker
  distributes attempts for the same account across many IPs, does the per-account counter
  alone stop them, or does something else need to correlate across IPs?
- **Device label parsing:** FR-11 says the device label is "parsed from User-Agent" — is
  there a fallback for an unparseable or spoofed User-Agent, and does this story need to
  specify one, or is "best effort, may show as Unknown device" acceptable?
