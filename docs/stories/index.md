# User stories

One folder per story under `docs/stories/<slug>/`, each containing `story.md`,
`acceptance-criteria.md`, `test-cases.md` and `diagram.md`.

| Slug | Summary | Status |
|------|---------|--------|
| [public-landing-page](public-landing-page/story.md) | Landing page for unauthenticated visitors: daily-regimen hero, marketing menu, and an auth modal with email/password and Google sign-in | draft |

## Epic: Authentication & session management

Derived from `docs/srs-authentication.md` (SRS-AUTH-001). See
[auth-epic-overview.md](auth-epic-overview.md) for how these five relate and connect to
`public-landing-page`'s auth modal.

| Slug | Actor | Summary | Status |
|------|-------|---------|--------|
| [auth-registration](auth-registration/story.md) | Unauthenticated visitor | Create an account, verify email, reset password | draft |
| [auth-login](auth-login/story.md) | Visitor with an account | Log in and receive a secure session | draft |
| [auth-session-refresh-logout](auth-session-refresh-logout/story.md) | Authenticated user | Silent refresh with replay detection; logout / logout-all | draft |
| [auth-session-management](auth-session-management/story.md) | Authenticated user | List and revoke my own sessions | draft |
| [auth-admin-account-management](auth-admin-account-management/story.md) | SystemAdmin | Suspend accounts, view sessions/events, grant/revoke admin role | draft |
