# Viewing and revoking my own sessions

**As an** authenticated user
**I want** to see every device currently signed in to my account and end any one of them
individually
**So that** I can notice access I don't recognize and shut it down without having to log
out everywhere and re-authenticate all my own legitimate devices

## Description

`auth-session-refresh-logout` covers ending the *current* session or *all* sessions.
This story is the middle ground: read visibility into `user_sessions:{user_id}`, and the
ability to kill one entry surgically. It's the feature that makes "someone else is in my
account" actionable without being destructive to the user's own other devices. Source:
SRS-AUTH-001 §3.5 (FR-40, FR-41).

## In scope

- `GET /api/v1/auth/sessions`: list the caller's own active sessions — device label,
  created-at, last-seen, and a marker for which entry is the device making the request
- `DELETE /api/v1/auth/sessions/{id}`: revoke one specific session by blacklisting its
  refresh `jti`
- The revoked session's access token dies naturally within its remaining lifetime (≤15 min)
  if not checked against the blacklist sooner, or immediately if it happens to be checked
- A user can only list or revoke their own sessions — never another user's, regardless of
  how the session ID is obtained or guessed

## Out of scope

- Ending the current session or every session at once (`auth-session-refresh-logout`)
- An admin viewing or revoking another user's sessions (`auth-admin-account-management`) —
  this story is strictly self-service
- Real-time "someone just logged in" notifications — this is a pull (list on demand), not a
  push

## Dependencies / assumptions

- `user_sessions:{user_id}` is populated at login (`auth-login`) with device label and
  created-at; this story adds last-seen and consumes the same structure
- Revoking a session here uses the same blacklist mechanism as logout
  (`auth-session-refresh-logout`) — this story specifies *when* a single session gets
  blacklisted, not a new revocation mechanism
- Session IDs are opaque identifiers scoped to `user_sessions`, not the raw `jti` itself, so
  that listing sessions doesn't hand the client anything that could be replayed as a token

## Open questions

- **"Last-seen" isn't in the SRS's data model note** (FR-11 only mentions `jti`, device
  label, created-at) but FR-40 asks for it in the list — where does last-seen get updated:
  every authenticated request (expensive, a write per request) or only on refresh (cheaper,
  coarser)?
- **Revoking the session you're currently using via this endpoint** — is that allowed (and
  equivalent to logout), or blocked with a message to use `/logout` instead?
