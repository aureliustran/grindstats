# Administering user accounts

**As a** SystemAdmin
**I want** to suspend, force-log-out, and inspect the sessions and security events of any
user account, and to grant or revoke the SystemAdmin role
**So that** I can operate the platform — respond to abuse, compromised accounts, or support
requests — without ever being able to read or change a user's actual health data

## Description

This is the only story in the epic where the actor is `SystemAdmin` rather than `User`, and
its central constraint is a boundary, not a capability: admin scope is accounts and
sessions, never content. Every action here is append-only audit-logged, and the role itself
is never something a user can acquire through any self-service path — it's granted by
another SystemAdmin, full stop. Source: SRS-AUTH-001 §3.6 (FR-42..47), plus the
authorization-boundary requirements of §3.3 (FR-22, FR-23) as they apply to distinguishing
`User` from `SystemAdmin` scope.

## In scope

- Suspend / unsuspend a user account; suspension sets that user's blacklist epoch
  (immediate logout everywhere) and makes subsequent logins fail with
  `AUTH_ACCOUNT_SUSPENDED`
- Force-logout a user (set epoch) without suspending the account
- View a user's session list and security events (login failures, refresh-replay
  incidents) — never passwords, password hashes, or token values
- Grant or revoke the SystemAdmin role on a target account; this is itself a
  SystemAdmin-only action, not exposed anywhere in the SPA UI, and it sets the target's
  epoch so any token already issued with the old role claim stops working
- Append-only audit logging of every admin action: actor, action, target user, timestamp,
  request ID — with no API path that can delete an audit record
- A `User`-role token calling any `/admin/*` endpoint receives 403 `AUTH_FORBIDDEN` (not
  404, which would leak whether the route exists)
- Admin endpoints never expose or accept a path to a user's metrics, meals, routines, or
  chat history

## Out of scope

- Any admin UI in the SPA for role grant/revoke — FR-46 explicitly keeps this API/ops-only
- Reading, exporting, or modifying a user's health/fitness data by any admin — that's
  permanently out of scope for this role, not just for this story
- How a *first* SystemAdmin account comes to exist (presumably a direct database operation
  at deployment time) — this story assumes at least one already exists and covers only
  ongoing grant/revoke by an existing one
- Support tooling beyond what's listed above (e.g. impersonating a user's session to
  reproduce a bug) — not specified anywhere in the SRS and not assumed here

## Dependencies / assumptions

- Role is carried as a claim (`role`) in the access token and checked in `libs/authmw`
  middleware, which is what makes a 403-not-404 response possible without a per-endpoint
  ownership lookup
- Suspension and force-logout both reuse the logout-all epoch mechanism from
  `auth-session-refresh-logout` — this story specifies *when* an admin triggers it, not a
  new invalidation mechanism
- Audit records are a separate, append-only store from operational data — the SRS doesn't
  specify where, only that no API can delete one

## Open questions

- **Suspension vs. deletion:** the SRS never mentions account deletion. Is "suspend" the
  only administrative removal path, or does deletion belong to a future, separate story
  (with its own data-retention implications for health data)?
- **Security-event retention** is specified elsewhere (SEC-07: ≥90 days) but this story
  doesn't say whether an admin viewing "security events" sees the full 90-day window or a
  paginated/filtered view — worth deciding before the endpoint is built.
- **Bootstrapping the first SystemAdmin** is assumed to be a direct DB operation, but that's
  an inference from "never self-service, never via any registration path" (§1.4) rather
  than something the SRS states outright — worth confirming before relying on it.
