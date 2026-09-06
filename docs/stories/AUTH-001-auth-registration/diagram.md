# Flow: Account registration

Registration branches heavily on validation and identity-linking rules, so this is a
flowchart rather than a sequence diagram — the interesting behavior is "which rule fired",
not "which service called which".

```mermaid
flowchart TD
    A[Visitor submits registration] --> B{Path?}

    B -->|Email + password| C{Email already registered?<br/>case-insensitive}
    C -->|Yes| D[Return neutral success-shaped<br/>response, no account change]
    C -->|No| E{Password length >= 10?}
    E -->|No| F[Reject: password too short]
    E -->|Yes| G{Breached-password check}
    G -->|Match, policy = reject| H[Reject: known-compromised password]
    G -->|Match, policy = warn| I[Warn, allow registrant to proceed]
    G -->|No match| J[Create account, role = User<br/>role field in payload ignored]
    I --> J
    J --> K[Send verification email:<br/>single-use token, 24h expiry]
    K --> L[Account usable but read-only<br/>until verified]

    B -->|Google OAuth| M[Redirect to Google consent<br/>state + PKCE]
    M --> N{Consent granted?}
    N -->|No| O[Return to app, no account change]
    N -->|Yes| P{Local account exists<br/>for this verified email?}
    P -->|No| Q[Auto-create User account<br/>from OAuth claim]
    P -->|Yes| R[Do not merge automatically:<br/>require login or verified-email<br/>confirmation to link]
    Q --> S[Authenticated — proceeds to auth-login]
    R --> T{Control proven?}
    T -->|Yes| S
    T -->|No| O

    L --> U{Visitor clicks verification link}
    U --> V{Token expired > 24h?}
    V -->|Yes| W[Reject: expired,<br/>offer to resend]
    V -->|No| X{Token already used?}
    X -->|Yes| Y[Reject: already used]
    X -->|No| Z[Mark verified —<br/>full write access unlocked]

    AA[Visitor requests password reset] --> AB[Return identical response<br/>whether or not email exists]
    AB --> AC{Real account? token emailed}
    AC -->|Yes| AD{Visitor submits new password<br/>with token}
    AD --> AE{Token valid: unused, < 1h old?}
    AE -->|No| AF[Reject, password unchanged,<br/>no session invalidated]
    AE -->|Yes| AG[Update password,<br/>invalidate every session<br/>logout-all]
```

## Notes on the non-obvious branches

- **`C`/`D` and `AB` — the enumeration-protection nodes.** A duplicate registration and a
  reset request for a nonexistent email both terminate in a response indistinguishable from
  the success path, per FR-08. *(Scenarios: registration response does not confirm an email
  is already registered; password-reset request does not confirm whether the email exists.)*
- **`J` — role is set unconditionally, never read from the request.** Even though the
  diagram shows "role field in payload ignored" as an annotation rather than a branch,
  that's deliberate: there is no code path in which an incoming role value changes the
  outcome. *(Scenario: an injected role field is ignored on registration.)*
- **`P`/`R`/`T` — OAuth never silently merges identities.** An OAuth login matching an
  existing local email always requires proof of control before the two identities become
  one account, closing what would otherwise be an account-takeover path (register locally
  with a victim's email, then have them "helpfully" link it via OAuth). *(Scenario: OAuth
  login matching an existing local account requires proof of control.)*
- **`L` — verified vs unverified is a state, not a gate at login.** An unverified account
  can still authenticate; the restriction is read-only access enforced afterward, which is
  why this diagram shows it as a branch reached *after* account creation rather than a
  login-time rejection. *(Scenario: unverified account is read-only on its own data.)*
- **`AG` — password reset always triggers logout-all**, not just a password change. This is
  what makes reset a recovery mechanism against a compromised session, not only a
  compromised password. *(Scenario: password reset completes and logs out every session.)*
