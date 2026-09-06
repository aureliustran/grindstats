# Flow: Silent session refresh, rotation, and logout

This story is about coordination between actors over time — the SPA, the gateway (the only
Redis-consulting component), a downstream service, and Redis — so it's a sequence diagram
rather than a flowchart, unlike `auth-registration` and `auth-login` where branching logic
was the point.

```mermaid
sequenceDiagram
    participant SPA as React SPA
    participant GW as Gateway
    participant R as Redis
    participant SVC as Downstream service

    Note over SPA,GW: Normal authenticated request
    SPA->>GW: Request + access token cookie
    GW->>GW: Verify signature, expiry
    GW->>R: Check blacklist:{jti}, epoch
    R-->>GW: Not blacklisted, iat >= epoch
    GW->>SVC: Forward request (sub, role trusted)
    SVC->>SVC: Verify signature + expiry only<br/>(no Redis call — FR-21)
    SVC-->>GW: Response
    GW-->>SPA: Response

    Note over SPA,GW: Silent refresh
    SPA->>GW: POST /refresh (refresh token cookie)
    GW->>GW: Verify signature, expiry
    GW->>R: Check blacklist:{jti}, epoch
    alt refresh token already blacklisted (replay)
        R-->>GW: jti found in blacklist
        GW->>R: Set user_blacklist_epoch = now
        GW->>GW: Log WARN: refresh replay detected
        GW-->>SPA: 401 — every session now invalid
    else idle > 14 days
        R-->>GW: iat older than idle threshold
        GW-->>SPA: 401 AUTH_SESSION_EXPIRED
    else valid
        R-->>GW: Not blacklisted, within epoch and idle window
        GW->>GW: Mint new access + refresh pair, new jtis
        GW->>R: Blacklist old refresh jti (TTL = remaining life)
        GW-->>SPA: New token pair as httpOnly cookies
    end

    Note over SPA,GW: Logout (this device only)
    SPA->>GW: POST /logout
    GW->>R: Blacklist current access + refresh jti (TTL = remaining life)
    GW->>GW: Remove entry from user_sessions
    GW-->>SPA: Clear both cookies

    Note over SPA,GW: Logout-all (every device)
    SPA->>GW: POST /logout-all
    GW->>R: Set user_blacklist_epoch = now (single write)
    GW-->>SPA: Current device's cookies cleared
    Note over GW,R: No per-token writes — every OTHER device's<br/>next request fails the epoch check in R independently

    Note over GW,R: Redis unreachable
    SPA->>GW: POST /refresh or /logout
    GW->>R: (unreachable)
    GW->>GW: Log loudly
    GW-->>SPA: Fail CLOSED — request rejected
    SPA->>GW: GET own-data (read-only)
    GW->>R: (unreachable)
    GW->>GW: Log loudly, skip blacklist/epoch check
    GW->>SVC: Forward anyway (signature/expiry only)
    SVC-->>GW: Response
    GW-->>SPA: Fail OPEN — response delivered
```

## Notes on the non-obvious branches

- **The downstream service never talks to Redis, in any branch.** Every Redis interaction
  in this diagram happens at the gateway. This is the "pay for statelessness once, at the
  edge" design from blueprint §8 — it's what keeps `AUTH_INVALID_TOKEN` cheap to check on
  every internal service call. *(Scenario: downstream services trust the gateway without
  their own Redis check.)*
- **Replay sets the epoch, not just a single blacklist entry.** Presenting an
  already-rotated refresh token doesn't just fail that one request — it revokes every
  session belonging to the user, including the session that legitimately holds the new,
  valid pair from the rotation that already happened. That's deliberately aggressive:
  the system can't tell which side (attacker or legitimate holder) has which token, so it
  trusts neither. *(Scenario: replaying a rotated refresh token kills every session.)*
- **Logout-all is one write, and other devices find out independently.** The diagram shows
  no message from the gateway to any other device — logout-all doesn't push a revocation,
  it simply changes the value that every future request's epoch check reads. This is what
  makes it O(1) regardless of how many devices are logged in. *(Scenario: logout-all ends
  every session without enumerating tokens.)*
- **The Redis-unreachable branch shows two different failure directions on purpose.**
  State-changing calls (refresh, logout) fail closed; a GET fails open. This asymmetry is
  the explicit trade-off in NFR-02 — a fitness dashboard staying readable outweighs a few
  seconds of unenforced revocation on reads, but nothing that changes state is ever allowed
  to proceed unchecked. *(Scenario: Redis unreachable fails closed on refresh and open on
  reads.)*
