# Flow: Administering user accounts

A flowchart — the story is dominated by which check gates which action, and by the
boundary between "accounts and sessions" and "health data" that this role must never cross.

```mermaid
flowchart TD
    A[Request to /admin/*] --> B{Authenticated at all?}
    B -->|No| C[401 AUTH_INVALID_TOKEN]
    B -->|Yes| D{Role claim == SystemAdmin?}
    D -->|No, role == User| E[403 AUTH_FORBIDDEN<br/>not 404 — route existence<br/>is not hidden]
    D -->|Yes| F{Which admin action?}

    F -->|Suspend| G[Set target's blacklist epoch = now]
    G --> H[Target's active sessions<br/>invalidated immediately]
    H --> I[Subsequent login attempts by<br/>target fail: AUTH_ACCOUNT_SUSPENDED]

    F -->|Unsuspend| J[Clear suspended flag]
    J --> K[Login now permitted again]
    K --> L[Pre-suspension sessions remain<br/>invalid — epoch was already<br/>advanced, not rolled back]

    F -->|Force-logout| M[Set target's blacklist epoch = now]
    M --> N[Sessions invalidated,<br/>account NOT suspended]
    N --> O[Target can log in again<br/>immediately]

    F -->|View sessions / security events| P[Query target's user_sessions<br/>and security-event log]
    P --> Q[Strip any credential-shaped field<br/>before returning —<br/>never password, hash, or token]

    F -->|Grant/revoke SystemAdmin role| R[Update target's role claim<br/>source of truth]
    R --> S[Set target's blacklist epoch = now]
    S --> T[Any token already issued with<br/>the OLD role claim is invalidated]
    T --> U[Target must obtain a new token<br/>pair to exercise the new role]

    G --> V[Write audit record:<br/>actor, action, target, timestamp,<br/>request ID — no delete path exists]
    M --> V
    R --> V
    J --> V
```

## Notes on the non-obvious branches

- **`B` is checked before `D`.** An unauthenticated request to an admin route gets 401, not
  403 — authentication failure and authorization failure are different things, and
  collapsing them would make it impossible to tell "you're nobody" from "you're the wrong
  somebody" from the response alone. *(Test case: unauthenticated request to an admin route
  is rejected before role is even considered.)*
- **`E` returns 403, deliberately not 404.** Hiding the route's existence behind a fake 404
  is a common instinct, but the SRS explicitly rejects it here (FR-22) — likely because
  admin routes aren't secret, they're role-gated, and pretending otherwise adds obscurity
  without real security. *(Scenario: a User token is forbidden, not not-found, on admin
  routes.)*
- **`R` → `S` → `T` is the same epoch mechanism as suspend and force-logout, applied to a
  role change.** A role grant/revoke doesn't get its own bespoke invalidation — it reuses
  the same "advance the epoch" primitive from `auth-session-refresh-logout`, which is why a
  stale token fails on the epoch check specifically, before the role check ever runs.
  *(Scenarios: granting SystemAdmin invalidates the target's existing tokens; revoking
  SystemAdmin immediately removes elevated access.)*
- **There is no branch from `F` that reaches health data**, and that's not an oversight in
  the diagram — no such route exists to branch to. The boundary in FR-47 is enforced by
  what endpoints exist, not by a runtime check that could be misconfigured.
  *(Scenario: admin scope cannot reach a user's health data through any admin endpoint.)*
