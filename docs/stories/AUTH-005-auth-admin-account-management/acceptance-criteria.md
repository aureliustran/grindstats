# Acceptance criteria: Administering user accounts

## Acceptance criteria (summary)

- [ ] A SystemAdmin can suspend and unsuspend any user account
- [ ] Suspension immediately logs the user out everywhere and blocks subsequent login
- [ ] A SystemAdmin can force-logout a user without suspending the account
- [ ] A SystemAdmin can view a user's sessions and security events, never credentials or
      token values
- [ ] A SystemAdmin can grant or revoke the SystemAdmin role, itself a SystemAdmin-only
      action, not exposed in the SPA
- [ ] Granting or revoking the role invalidates the target's existing tokens
- [ ] Every admin action produces an undeletable, append-only audit record
- [ ] A `User` token hitting any `/admin/*` route gets 403, not 404
- [ ] No admin endpoint can read or modify a user's health data

## Acceptance criteria (scenarios)

### Scenario: suspending a user immediately ends their sessions and blocks login

**Given** a user has active sessions on two devices
**When** a SystemAdmin suspends that account
**Then** the user's blacklist epoch is set to now, invalidating both sessions immediately
**And** a subsequent login attempt by that user fails with `AUTH_ACCOUNT_SUSPENDED`
**And** an audit record is written naming the admin, the action, the target, and a
timestamp

### Scenario: unsuspending restores the ability to log in, but not old sessions

**Given** a previously suspended account is unsuspended
**When** the user attempts to log in
**Then** login succeeds and a new session is issued
**And** the sessions that were active before suspension remain invalid — unsuspending
does not resurrect them

### Scenario: force-logout ends sessions without suspending

**Given** a user has active sessions and the account is in good standing
**When** a SystemAdmin force-logs-out that user
**Then** the user's epoch is set to now, invalidating current sessions
**And** the account is not suspended — the user can log in again immediately
**And** an audit record is written for the force-logout action

### Scenario: admin can view sessions and security events but never credentials

**Given** a target user has a login history including some failures
**When** a SystemAdmin requests that user's sessions and security events
**Then** device labels, timestamps, and event types (login failure, refresh replay) are
returned
**And** no password, password hash, or raw token value appears anywhere in the response

### Scenario: granting SystemAdmin invalidates the target's existing tokens

**Given** a `User`-role account is currently logged in with a valid access token
**When** another SystemAdmin grants that account the SystemAdmin role
**Then** the target's epoch is set to now
**And** the previously issued access token — still carrying `role: User` — is invalidated
**And** the target must obtain a new token pair (log in again, or refresh if still within
session lifetime) to receive a token carrying the updated role claim

### Scenario: revoking SystemAdmin immediately removes elevated access

**Given** an account currently holds the SystemAdmin role
**When** a SystemAdmin revokes that role
**Then** the target's epoch is set to now, invalidating tokens carrying the old role claim
**And** any subsequent admin action attempted with a stale token fails the epoch check
before it fails the role check

### Scenario: role grant/revoke is not reachable through the SPA UI

**Given** I am authenticated as a SystemAdmin using the web application
**When** I look for a way to change another user's role
**Then** no such control exists in the SPA — this action is available only directly
against the API

### Scenario: every admin action writes an undeletable audit record

**Given** any SystemAdmin action has just been performed (suspend, unsuspend,
force-logout, role change)
**When** the audit record for it is inspected
**Then** it includes the acting admin's `sub`, the action, the target user, a timestamp,
and a request ID
**And** no API endpoint exists that can delete or modify that record

### Scenario: a User token is forbidden, not not-found, on admin routes

**Given** I am authenticated with role `User`
**When** I call any `/admin/*` endpoint
**Then** the response is 403 `AUTH_FORBIDDEN`
**And** it is not 404 — the route's existence is not hidden by pretending it doesn't exist

### Scenario: admin scope cannot reach a user's health data through any admin endpoint

**Given** I am authenticated as a SystemAdmin
**When** I inspect the full set of `/admin/*` endpoints available to me
**Then** none of them can read or write a target user's metrics, meals, training logs,
routines, or chat history
**And** attempting to construct such a request (e.g. reusing a metrics endpoint path under
`/admin/`) fails, because no such route exists — not because of a runtime permission check
alone
