# Flow: Public landing page for unauthenticated visitors

This flowchart shows only behavior established in `story.md` and `acceptance-criteria.md`.
Each branch is traceable to a scenario; the notes below name the scenario where a branch
exists for a non-obvious reason.

```mermaid
flowchart TD
    A[Visitor requests /] --> B{Valid session cookies?}
    B -->|Yes| C[Redirect to dashboard]
    B -->|No| D[Render landing page:<br/>regimen hero, silhouette,<br/>marketing sections, menu]

    D --> E{Visitor action}
    E -->|Selects a menu anchor| F{prefers-reduced-motion?}
    F -->|Yes| G[Jump to section, no animation]
    F -->|No| H[Smooth-scroll to section]
    G --> D
    H --> D

    E -->|Narrow viewport:<br/>activates hamburger| I[Expand / collapse menu]
    I --> D

    E -->|Log in / Get started| J[Open auth modal over page<br/>focus trapped, page state kept]

    J --> K{Modal action}
    K -->|Escape / backdrop / close| L[Close modal,<br/>restore focus and scroll position]
    L --> D
    K -->|Switch tab| M[Clear other tab's errors and fields]
    M --> K

    K -->|Continue with Google| N[Redirect to Google consent]
    N --> O{Consent granted?}
    O -->|No| P[Return to landing page,<br/>no session, non-blocking notice]
    P --> D
    O -->|Yes| Q[OAuth callback to auth-service]
    Q --> R

    K -->|Submit email + password| S{Client-side validation passes?}
    S -->|No| T[Show field errors,<br/>keep submit disabled,<br/>send no request]
    T --> K
    S -->|Yes| U[POST /api/v1/auth/login<br/>or /register]

    U --> V{auth-service response}
    V -->|Reachable failure:<br/>bad credentials or duplicate email| W[Show generic message,<br/>no enumeration signal]
    W --> K
    V -->|429 rate limited| X[Show too-many-attempts message,<br/>disable submit for cool-down]
    X --> K
    V -->|Unreachable / timeout| Y[End loading state,<br/>show retry message,<br/>keep modal open and email intact]
    Y --> K
    V -->|Success| R[Set httpOnly session cookies]

    R --> Z[Dismiss modal, redirect to dashboard]
```

## Notes on the non-obvious branches

- **`B` — session check before render.** The landing page is never the final destination
  for an authenticated visitor; the redirect happens ahead of render rather than as a
  flash of marketing content. *(Scenario: already-authenticated visitor is redirected away
  from the landing page.)*
- **`W` — one node for two different failures.** Bad credentials and duplicate
  registration deliberately converge on the same generic response, because branching them
  into distinct messages is exactly the account-enumeration leak SRS-AUTH-001 §5 forbids.
  *(Scenarios: invalid credentials do not reveal whether the email is registered;
  registering an already-registered email does not confirm the account exists.)*
- **`S` before `U` — validation gates the network call.** Malformed input never reaches
  auth-service, which keeps the rate limiter measuring real attempts rather than typos.
  *(Scenario: client-side validation blocks a malformed submission before any request.)*
- **`Y` — the unreachable branch returns to the modal, not to an error page.** The visitor
  keeps their entered email and the page they were converted by. *(Scenario: auth-service
  being unreachable surfaces an error instead of hanging.)*
- **`L` — closing restores both focus and scroll.** The modal is an overlay, not a route,
  so dismissing it must return the visitor to precisely where they were. *(Scenarios: auth
  modal opens over the page and can be dismissed without losing position; focus stays
  inside the modal while it is open.)*
- **`R` is reached from two paths.** Google OAuth and email/password converge on the same
  cookie-setting step, which is what lets the SPA stay ignorant of how the visitor
  authenticated. *(Scenarios: visitor authenticates with Google; returning visitor logs in
  and reaches the dashboard.)*
