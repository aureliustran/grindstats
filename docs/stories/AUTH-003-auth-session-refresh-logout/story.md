# Silent session refresh, rotation, and logout

**Code:** `AUTH-003`

**As an** authenticated user
**I want** my short-lived access token to renew itself silently, and to be able to end my
session on this device or on every device
**So that** I stay signed in without repeatedly re-entering credentials, while a stolen
refresh token can be detected and shut down fast

## Description

Access tokens live 10–15 minutes by design (blueprint §8) — short enough that a leaked one
does limited damage. That only works ergonomically if the SPA can silently trade a refresh
token for a new pair without the user noticing. This story specifies that trade (rotation),
the two ways a session ends deliberately (logout, logout-all), and the detection mechanism
for the case that makes rotation safe: **a rotated refresh token is single-use, so anyone
presenting an already-used one is presenting a stolen one.** This story is also where the
full gateway request-authorization check chain (FR-20) is specified, because refresh is the
one endpoint that exercises every link in that chain on every call. Source: SRS-AUTH-001
§3.3 (FR-20, FR-21) and §3.4 (FR-30..34).

## In scope

- `POST /api/v1/auth/refresh`: verify the refresh cookie (signature → expiry → blacklist →
  epoch, in that order), then mint a new access + refresh pair and immediately blacklist
  the old refresh `jti`
- Refresh-token replay detection: presenting an already-blacklisted refresh token sets the
  user's blacklist epoch (killing every session), logs a WARN-level security event, and
  returns 401
- `POST /api/v1/auth/logout`: blacklist the current access and refresh `jti`s (TTL = each
  token's remaining life), remove the session from `user_sessions`, clear both cookies
- `POST /api/v1/auth/logout-all`: set the user's blacklist epoch to now — O(1), no
  per-token enumeration — also triggered by password change and by admin suspension
- Sliding idle expiry: refresh fails with `AUTH_SESSION_EXPIRED` if the session has been
  idle beyond 14 days, even inside the 30-day absolute lifetime
- The gateway's full check order for every authenticated request: signature → expiry →
  `blacklist:{jti}` → `iat` vs `user_blacklist_epoch:{user_id}` — failing any of these
  returns 401 `AUTH_INVALID_TOKEN`
- Downstream services verifying signature and expiry only, trusting `sub`/`role` from the
  gateway without their own Redis call
- Redis-unreachable degradation: fail closed for refresh/logout (state-changing), fail open
  (signature-only) for GET endpoints, logging loudly either way

## Out of scope

- Initial login and token issuance (`auth-login`)
- Listing sessions or revoking one specific session by choice, as opposed to ending the
  current session or all sessions (`auth-session-management`)
- Admin-triggered suspension itself — this story only specifies that suspension *causes*
  logout-all; the admin action is `auth-admin-account-management`
- CSRF token issuance (specified in `auth-login`) — this story assumes it already exists
  and only notes that refresh/logout are state-changing and therefore subject to it

## Dependencies / assumptions

- The gateway is the only component that consults Redis for auth; downstream services never
  do (blueprint §8) — this is what makes FR-21 possible
- `blacklist:{jti}` keys always carry a TTL equal to the token's remaining life, so the
  blacklist self-cleans with no manual maintenance (NFR-03)
- Password reset (`auth-registration`) and admin suspension
  (`auth-admin-account-management`) both reuse this story's logout-all mechanism rather than
  each implementing their own
- Redis co-located with the gateway for latency (NFR-01: ≤5ms p95 for the full check)

## Open questions

- **Grace window on rotation:** some rotation implementations tolerate a brief window where
  the immediately-prior refresh token still works, to absorb network retries that duplicate
  a refresh call. The SRS says "immediately blacklist" with no grace window mentioned — is a
  legitimate retry storm going to be misdiagnosed as replay, and if so, is that accepted?
- **What exactly triggers the client's refresh call?** Proactively before the 10–15 min
  access token expires, or reactively after a request comes back 401? The SRS specifies
  server-side refresh behavior but not the SPA's triggering strategy — that likely belongs
  in a frontend-specific story once `apps/web`'s auth context is built.
