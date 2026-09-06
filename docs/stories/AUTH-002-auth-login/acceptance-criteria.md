# Acceptance criteria: Login and session establishment

## Acceptance criteria (summary)

- [ ] Successful login mints an access token and refresh token, each with a unique `jti`
- [ ] Both tokens are delivered only as httpOnly, Secure, SameSite=Lax cookies
- [ ] Neither token string ever appears in a response body, URL, or JS-readable storage
- [ ] The refresh cookie is path-scoped to `/api/v1/auth`
- [ ] A session record is created in `user_sessions:{user_id}` with device label and
      timestamp
- [ ] A CSRF token is returned in the response body, separate from the cookies
- [ ] Five consecutive failed logins trigger exponential backoff starting at 30s
- [ ] A successful login resets the failure counter
- [ ] Login never reveals whether the email or the password was wrong
- [ ] Login responses do not reveal whether a given email has an account

## Acceptance criteria (scenarios)

### Scenario: successful login issues cookie-only tokens and a CSRF token

**Given** an account exists for `returning@example.com` with a known password
**When** I submit the correct email and password
**Then** an access token and a refresh token are set as httpOnly, Secure, SameSite=Lax
cookies
**And** the refresh cookie's path is scoped to `/api/v1/auth`
**And** neither token string appears anywhere in the JSON response body
**And** a CSRF token is present in the response body
**And** `document.cookie` cannot read either auth cookie

### Scenario: login creates a session record with a device label

**Given** I log in successfully from a browser with a recognizable User-Agent
**When** the login completes
**Then** an entry appears in my session list (surfaced via `auth-session-management`) with
a device label derived from that User-Agent and a created-at timestamp

### Scenario: Google OAuth login for an existing account issues the same session shape

**Given** an account already exists, previously linked to my Google identity
**When** I complete Google OAuth login
**Then** the resulting cookies, CSRF token, and session record match the shape produced by
email/password login exactly — no OAuth-specific token format or transport

### Scenario: wrong password gives a generic failure without revealing which factor failed

**Given** an account exists for `returning@example.com`
**When** I submit that email with an incorrect password
**Then** the response is a generic "email or password is incorrect" message
**And** no cookies or session record are created
**And** the failed-attempt counter for this account increments

### Scenario: nonexistent email gives an identical generic failure

**Given** no account exists for `nobody@example.com`
**When** I submit that email with any password
**Then** the response is identical in wording, shape, status code, and — as far as
observable — timing, to the wrong-password scenario above

### Scenario: five consecutive failures trigger backoff

**Given** I have failed to log in to the same account 4 times in a row
**When** I fail a 5th consecutive time
**Then** the next login attempt for that account requires waiting at least 30 seconds
**And** attempting immediately again is rejected with a message explaining the delay,
not a silent failure

### Scenario: backoff increases with continued failures

**Given** I have already triggered the initial 30s backoff and waited it out
**When** I fail again
**Then** the required delay before the next attempt increases beyond 30s
**And** the growth is exponential rather than resetting to 30s each time

### Scenario: a successful login resets the failure counter

**Given** I have 3 recorded consecutive failures for my account, below the backoff
threshold
**When** I log in successfully
**Then** the failure counter for my account resets to zero
**And** a subsequent single failed attempt does not immediately trigger backoff

### Scenario: backoff blocks the account even from a new source IP

**Given** my account is currently in a backoff window triggered from IP A
**When** a login attempt for the same account arrives from a different IP B before the
backoff window ends
**Then** that attempt is also subject to the remaining backoff, because the counter is
per-account, not solely per-IP

### Scenario: an unverified account can still log in

**Given** an account exists but has not completed email verification (per
`auth-registration`)
**When** the owner logs in with correct credentials
**Then** login succeeds and a normal session is established
**And** subsequent write restrictions are enforced by the unverified-account rule, not by
blocking login itself
