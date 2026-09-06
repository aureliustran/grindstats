# Flow: Viewing and revoking my own sessions

A small flowchart — the whole story is one read path and one write path, both gated by
the same ownership check.

```mermaid
flowchart TD
    A[Authenticated request arrives<br/>with sub from token] --> B{Which operation?}

    B -->|GET /sessions| C[Query user_sessions:sub only<br/>— never another user's]
    C --> D[Mark the entry matching<br/>this request's own jti as<br/>'this device']
    D --> E[Return list:<br/>device label, created-at,<br/>last-seen, current-device flag]

    B -->|DELETE /sessions/id| F{Does session id belong<br/>to sub making the request?}
    F -->|No — foreign or nonexistent id| G[Reject 404/403 —<br/>no confirmation either way<br/>of whether id exists]
    F -->|Yes| H[Blacklist that session's<br/>refresh jti, TTL = remaining life]
    H --> I[Remove entry from<br/>user_sessions]
    I --> J[Other sessions for this user<br/>are untouched]
```

## Notes on the non-obvious branches

- **`F` returns the same rejection whether the ID belongs to someone else or doesn't exist
  at all.** Distinguishing "not yours" from "doesn't exist" would let a caller enumerate
  valid session IDs by watching which error comes back — the same enumeration-protection
  instinct as the registration and login stories, applied here to session IDs instead of
  emails. *(Scenario: a user cannot revoke another user's session.)*
- **`C` scopes the query by `sub` at the data layer, not by filtering a full list
  afterward.** The distinction matters: filtering after the fact means the query itself
  touched other users' data, which is the kind of bug that becomes a real leak the moment
  someone forgets the filter on a new code path. Scoping the query itself makes the leak
  structurally impossible rather than dependent on remembering a filter.
  *(Scenario: listing sessions never includes another user's sessions.)*
- **`H` reuses the exact blacklist mechanism from `auth-session-refresh-logout`.** This
  diagram doesn't introduce a new way to invalidate a session — revoking one here is
  identical in mechanism to what happens to the current session on `/logout`, just targeted
  at a `jti` that isn't necessarily the caller's own current one.
