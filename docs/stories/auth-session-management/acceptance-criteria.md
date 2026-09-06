# Acceptance criteria: Viewing and revoking my own sessions

## Acceptance criteria (summary)

- [ ] A user can list their own active sessions with device label, created-at, and
      "this device" marker
- [ ] A user can revoke any one of their own sessions by ID
- [ ] Revoking a session blacklists that session's refresh `jti`
- [ ] A user cannot list or revoke another user's session under any circumstance
- [ ] Revoking a session does not affect the user's other sessions

## Acceptance criteria (scenarios)

### Scenario: listing sessions shows every active device with the current one marked

**Given** I am logged in on my phone and my laptop
**When** I call `GET /sessions` from my laptop
**Then** the response includes both sessions with their device labels and created-at times
**And** the laptop entry is marked as the current device
**And** the phone entry is not marked as current

### Scenario: revoking a specific session ends only that session

**Given** I have two active sessions, A (laptop) and B (phone)
**When** I call `DELETE /sessions/{B's id}` from my laptop
**Then** session B's refresh `jti` is blacklisted
**And** a subsequent refresh attempt from the phone fails
**And** session A on the laptop remains fully valid and untouched

### Scenario: a revoked session's access token stops working within its natural lifetime

**Given** session B was just revoked while its access token still had a few minutes of
life left
**When** a request from device B is checked against the blacklist before that access
token's natural expiry
**Then** it is rejected immediately
**And** in the worst case it stops working no later than its original expiry, even if no
check happens to occur before then

### Scenario: a user cannot revoke another user's session

**Given** I know or guess a session ID that does not belong to me
**When** I call `DELETE /sessions/{that id}`
**Then** the request is rejected (404 or 403, not a silent no-op that could leak whether
the ID exists)
**And** the session, if it exists and belongs to someone else, is not affected

### Scenario: listing sessions never includes another user's sessions

**Given** other users have active sessions
**When** I call `GET /sessions`
**Then** only sessions belonging to my own `sub` are returned, regardless of any parameter
in the request

### Scenario: an empty session list is possible without being an error

**Given** all of my other sessions have already ended (expired, logged out, or revoked)
and only my current one remains
**When** I call `GET /sessions`
**Then** the response lists exactly my current session, not an empty list and not an error
— "no other sessions" is a normal, expected state
