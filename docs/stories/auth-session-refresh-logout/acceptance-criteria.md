# Acceptance criteria: Silent session refresh, rotation, and logout

## Acceptance criteria (summary)

- [ ] Refresh checks signature, expiry, blacklist, and epoch in that exact order
- [ ] A successful refresh mints a new token pair and immediately blacklists the old
      refresh `jti`
- [ ] Presenting an already-blacklisted refresh token sets the user's epoch and kills every
      session
- [ ] A refresh-replay event is logged at WARN or above
- [ ] Logout blacklists the current access and refresh `jti`s and clears both cookies
- [ ] Logout-all sets the epoch in O(1) with no per-token operation
- [ ] Refresh fails with `AUTH_SESSION_EXPIRED` after 14 days of idle time even within the
      30-day absolute window
- [ ] Downstream services never call Redis for auth checks
- [ ] Redis being unreachable fails closed on refresh/logout and fails open on GET
- [ ] Every blacklist key carries a TTL and requires no manual cleanup

## Acceptance criteria (scenarios)

### Scenario: valid refresh rotates the token pair

**Given** I hold a valid, non-expired, non-blacklisted refresh token within my epoch
**When** I call `/refresh`
**Then** I receive a new access token and a new refresh token, each with a new `jti`
**And** my previous refresh token's `jti` is immediately blacklisted with a TTL equal to
its remaining life
**And** my previous access token continues to work until it naturally expires (rotation
does not retroactively kill the still-live access token)

### Scenario: replaying a rotated refresh token kills every session

**Given** I have already rotated once, so my old refresh token's `jti` is blacklisted
**When** that old refresh token is presented again to `/refresh`
**Then** the request is rejected with 401
**And** my `user_blacklist_epoch` is set to now, invalidating every session I have —
including the legitimately-rotated one just issued
**And** a WARN-level security event is logged noting the replay

### Scenario: the gateway check order is signature, then expiry, then blacklist, then epoch

**Given** an access token that would fail more than one check simultaneously — e.g. an
expired token whose `jti` is also blacklisted
**When** it is presented to the gateway
**Then** the response and logged reason correspond to whichever check fails first in the
order signature → expiry → blacklist → epoch
**And** the response code is 401 `AUTH_INVALID_TOKEN` regardless of which check failed

### Scenario: downstream services trust the gateway without their own Redis check

**Given** a request has passed the gateway's full check chain and been forwarded
**When** a downstream service package processes it
**Then** that service verifies only the token's signature and expiry
**And** it makes no call to Redis
**And** it uses the `sub` and `role` claims as forwarded, without re-deriving them

### Scenario: logout ends only the current session

**Given** I am logged in on two devices
**When** I call `/logout` from device A
**Then** device A's access and refresh `jti`s are blacklisted and its cookies cleared
**And** device A's entry is removed from `user_sessions`
**And** device B's session remains fully valid and untouched

### Scenario: logout-all ends every session without enumerating tokens

**Given** I am logged in on three devices
**When** I call `/logout-all` from any one of them
**Then** my `user_blacklist_epoch` is set to now in a single O(1) write
**And** the next request from every device — not just the one that called logout-all —
fails the epoch check
**And** no per-device or per-token blacklist entry is written to accomplish this

### Scenario: an idle session beyond 14 days cannot refresh even within the 30-day window

**Given** my refresh token was issued 20 days ago (within the 30-day absolute lifetime)
**And** it has not been used to refresh in the last 14 days
**When** I call `/refresh`
**Then** the request is rejected with 401 `AUTH_SESSION_EXPIRED`
**And** I am required to log in again rather than being silently refreshed

### Scenario: an active session survives past 14 days as long as it keeps refreshing

**Given** my refresh token was issued 20 days ago and has refreshed every few days since
**When** I call `/refresh` again, still within the 30-day absolute lifetime
**Then** the refresh succeeds, because idle time — not absolute age — is what the 14-day
rule measures

### Scenario: Redis unreachable fails closed on refresh and open on reads

**Given** Redis is unreachable from the gateway
**When** I call `/refresh` or `/logout`
**Then** the request fails rather than proceeding unchecked
**And** the failure is logged loudly
**But when** I make a GET request to read my own data with a token that passes signature
and expiry checks
**Then** the request succeeds without the blacklist/epoch check, since availability of
reads outweighs revocation lag for that case
