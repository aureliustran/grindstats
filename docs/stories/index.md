# User stories

One folder per story under `docs/stories/<CODE>-<slug>/`, each containing `story.md`,
`acceptance-criteria.md`, `test-cases.md` and `diagram.md`. `<CODE>` is area-prefixed
(`LAND-`, `AUTH-`, ...) and sequential within its area — see each area's counter below
before assigning the next one.

| Code | Slug | Summary | Status |
|------|------|---------|--------|
| [LAND-001](LAND-001-public-landing-page/story.md) | public-landing-page | Landing page for unauthenticated visitors: daily-regimen hero, marketing menu, and an auth modal with email/password and Google sign-in | draft |

## Epic: Authentication & session management

Derived from `docs/srs-authentication.md` (SRS-AUTH-001). See
[auth-epic-overview.md](auth-epic-overview.md) for how these five relate and connect to
`LAND-001-public-landing-page`'s auth modal.

| Code | Slug | Actor | Summary | Status |
|------|------|-------|---------|--------|
| [AUTH-001](AUTH-001-auth-registration/story.md) | auth-registration | Unauthenticated visitor | Create an account, verify email, reset password | draft |
| [AUTH-002](AUTH-002-auth-login/story.md) | auth-login | Visitor with an account | Log in and receive a secure session | draft |
| [AUTH-003](AUTH-003-auth-session-refresh-logout/story.md) | auth-session-refresh-logout | Authenticated user | Silent refresh with replay detection; logout / logout-all | draft |
| [AUTH-004](AUTH-004-auth-session-management/story.md) | auth-session-management | Authenticated user | List and revoke my own sessions | draft |
| [AUTH-005](AUTH-005-auth-admin-account-management/story.md) | auth-admin-account-management | SystemAdmin | Suspend accounts, view sessions/events, grant/revoke admin role | draft |

## Next available code per area

- `LAND-` → next is `LAND-002`
- `AUTH-` → next is `AUTH-006`

Update this list whenever a new story is added — it's what stops two stories from
colliding on the same code.
