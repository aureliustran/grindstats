# Amendment request: session.Handler.Register mixes public and protected routes

**Raised by:** `be-wiring`
**Date:** 2026-09-14
**Blocks:** Correct route-group placement for `/auth/logout`, `/auth/logout-all`, and `/users/me`

## What the contract currently says

`docs/stories/auth-epic/contract.md` §1 specifies:

- `/api/v1/auth/{register,login,verify-email,password-reset/request,password-reset/confirm, oauth/google,oauth/google/callback,oauth/link/confirm,refresh}` — reachable with **no** access token
- `/api/v1/auth/{logout,logout-all}` — access token **and** CSRF required
- `/api/v1/users/me` — access token required, no CSRF (safe method)

Contract §6 states: "Public routes opt out [of auth middleware] by route group, not by a path list."

## What's wrong with it

`session.Handler.Register(r gin.IRouter)` mounts all five session routes (login, refresh, logout, logout-all, /users/me) onto a single `gin.IRouter`. There is no mechanism to split the registration across two route groups.

The composition root (`be-wiring`, wave 4) cannot correctly place:
- login and refresh on `srv.V1` (public — no auth middleware), AND
- logout, logout-all, and /users/me on `srv.Protected` (auth+CSRF+verified-write)

...using a single `Register` call. Registering on V1 makes the protected routes non-functional (handlers call `authmw.ClaimsFromContext` which is only populated by `middleware.Auth`; without it, every call to logout/logout-all/users/me returns 401 regardless of whether the caller has a valid token). Registering on Protected breaks login and refresh (the auth middleware rejects requests with no access cookie before the handler runs).

Calling `Register` twice (once on V1, once on Protected) causes gin to panic on duplicate routes because Protected is `v1.Group("")` with the same path prefix.

## What I propose instead

Add two exported registration methods to `session.Handler`:

```go
// RegisterPublic mounts the unauthenticated session routes (login, refresh)
// onto r. Called by be-wiring on srv.V1.
func (h *Handler) RegisterPublic(r gin.IRouter) {
    r.POST("/auth/login", h.handleLogin)
    r.POST("/auth/refresh", h.handleRefresh)
}

// RegisterProtected mounts the authenticated session routes (logout,
// logout-all, /users/me) onto r. Called by be-wiring on srv.Protected.
func (h *Handler) RegisterProtected(r gin.IRouter) {
    r.POST("/auth/logout", h.handleLogout)
    r.POST("/auth/logout-all", h.handleLogoutAll)
    r.GET("/users/me", h.handleMe)
}
```

The existing `Register(r gin.IRouter)` can be kept for backward compatibility in tests, or deprecated and replaced.

The composition root in `main.go` would then call:
```go
sessionHandler.RegisterPublic(srv.V1)
sessionHandler.RegisterProtected(srv.Protected)
```

## Who else this affects

- `be-auth-session` — the slice that owns `session.Handler`. Must add `RegisterPublic` and `RegisterProtected`. The existing `Register` method can remain (it is used by session's own unit tests).
- `be-wiring` (this slice) — composition root must be updated to call the new methods instead of `Register`. The call site in `main.go` is already documented with a TODO referencing this amendment.
- Phase 3 testers: the e2e test stages for logout, logout-all, and /users/me will not pass until this amendment is implemented. Those stages are annotated with "BLOCKED: AMD-005" in `e2e_test.go`.

## What I did in the meantime

- All other wiring is complete: credentials, OAuth, config, key-generation scripts, docker-compose, .env.example.
- Session routes are registered on `srv.V1` as a pragmatic workaround. Login and refresh work correctly. Logout, logout-all, and /users/me return 401 for all requests (including authenticated ones) because no auth middleware populates claims.
- Integration tests are written in full. The enumeration test (TC-16) exercises login and register and passes. The e2e test is written and compiles; affected stages are explicitly marked as blocked and will fail until this amendment is implemented.
- `main.go` carries a comment at the `sessionHandler.Register(srv.V1)` call citing this amendment.

---

## Instructor decision

**Decision:** **Accepted, exactly as proposed.**

**Reasoning:** This is a partition gap, not a contract dispute — `contract.md` §1 already says
which routes need auth and which don't; the gap was that `session.Handler` exposed no way for
the composition root to place them on different gin groups. The workaround `be-wiring` found in
the meantime (registering everything on the public group) is worse than doing nothing: it silently
leaves `logout`, `logout-all` and `/users/me` completely unauthenticated rather than merely
non-functional, since `ClaimsFromContext` returns `ok=false` and every handler correctly treats
that as `401` — the *current* behavior in `be-wiring`'s worktree happens to fail safe (401 for
everyone) only because each handler checks `ok` before trusting anything, not because the routing
is correct. It had to be fixed before this could ship, not filed for later.

The fix is exactly what the amendment proposed: two new exported methods,
`RegisterPublic` (login, refresh) and `RegisterProtected` (logout, logout-all, `/users/me`),
alongside the original `Register` (kept, since `be-auth-session`'s own unit tests build their
router with a single group and set claims via test middleware — changing that test harness
wasn't necessary to fix the wiring bug). No signature on any handler function changed; no
behavior inside a handler changed. Implemented directly by the instructor rather than
re-dispatching `be-auth-session`, matching how AMD-002's generator gap and AMD-004's confirmed-
independently-twice `httpkit` gap were handled: small, mechanical, no design ambiguity, and the
originating slice's tests (all 25) re-verified green with no changes needed to any of them.

**Contract updated:** No change to `contract.md` — §1's per-route auth requirements were already
correct; this fixes the Go API surface that implements them, not the contract itself.

**Slices to re-verify:** `be-auth-session` — re-verified: `go build`, `go vet`, and its full test
suite (25 tests) pass unchanged after the split. `be-wiring` — its `main.go` must call
`RegisterPublic(srv.V1)` / `RegisterProtected(srv.Protected)` instead of the single blocked
`Register(srv.V1)` call; done as part of integrating `be-wiring`'s report. The e2e test stages
marked "BLOCKED: AMD-005" are now unblocked and expected to exercise real auth-protected
behavior for logout/logout-all/`/users/me`, pending a live Postgres+Redis to actually run them.

**Notified:** Recorded here for `be-wiring`'s integration and for phase 3, whose testers should
no longer expect the "logout/logout-all/me are unauthenticated" behavior `be-wiring`'s own report
described — that description was accurate for the worktree as reported, not for the integrated
result.
