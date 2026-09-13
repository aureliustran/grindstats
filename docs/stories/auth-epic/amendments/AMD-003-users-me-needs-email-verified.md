# Amendment request: `GET /users/me` must return `email_verified`

**Raised by:** the instructor, while reviewing `fe-auth-flows`'s completed work
**Date:** 2026-09-13
**Blocks:** `be-auth-session`'s `/users/me` handler (not yet dispatched)

## What the contract currently says

Contract §1, row 12:

```
| `/users/me` | GET | access | — | 200 {"data":{"user":{id,email,role,tier}, "csrf_token": string}} | ... |
```

`email_verified` exists only as an access-token **claim** (§2.1:
`{ sub, sid, role, tier, email_verified, jti, iat, exp, typ:"access" }`), read by gateway
middleware to enforce D4 (block unsafe-method writes from an unverified account). It was never
part of any HTTP response body.

## What's wrong with it

D4 was designed as a server-side enforcement mechanism, and it works for that. But
AUTH-001's "unverified account is read-only on its own data" scenario also has a **client-side**
half — `fe-auth-flows`'s brief (assigned by this instructor) requires:

> a component test: an authenticated, unverified session renders the banner ... and the write
> control is disabled

The SPA cannot read an httpOnly cookie's JWT claims. With nothing in any response body carrying
verification status, the client has no way to know whether to show the banner or disable a
control before the user attempts the write and gets a 403 back. The scenario as specified is not
implementable end-to-end without this field somewhere the SPA can read it — and `/users/me` is
the only session-shaped response that exists.

This is an oversight in the original contract, not something either frontend slice did wrong.
`fe-auth-flows` (and `fe-auth-client`, which added the type) correctly identified the gap and
worked around it safely: `CurrentUser.email_verified` is optional
(`email_verified?: boolean`), and `AuthContext` defaults it to `true` when absent
(`session.user.email_verified ?? true`) — so today, before any backend exists, every session
reads as verified and nothing regresses. But it means the feature silently never fires once a
real backend is wired up unless the field is actually added to the response, because the default
takes over permanently.

## Decision

**Accepted.** `GET /users/me`'s success response gains one field:

```
200 {"data":{"user":{id,email,role,tier,email_verified: boolean}, "csrf_token": string}}
```

This is additive (`shared-contract.md` §3: a new field is backward-compatible, no version bump)
and matches the JWT claim it's already trusted to expose — `email_verified` crosses the FR-03
boundary in the claim already; putting the same fact in the one response body that carries user
state is not a new trust boundary, just a new place the same fact is visible.

**Contract updated:** `contract.md` §1 row 12 now includes `email_verified` in the `/users/me`
response shape.

**Slices to re-verify:** `fe-auth-client` and `fe-auth-flows` — **both already conform.** The
type (`CurrentUser.email_verified?: boolean`) and the consuming logic
(`AuthContext`'s `?? true` fallback) already match this shape; no frontend change is needed. The
`?? true` fallback stays as-is even after this amendment — it is the correct behavior for a
malformed or old-shaped response, not a workaround to remove.

**`be-auth-session`'s brief is updated** (below) since it has not been dispatched — this is the
cheapest possible point to fix it, before any code exists against the old shape.

## Brief change

`docs/stories/auth-epic/briefs/be-auth-session.md`:

- The `/users/me` row in its endpoint table now reads:
  `200 {"data":{"user":{id,email,role,tier,email_verified},"csrf_token"}} `
- Added instruction: the handler reads `email_verified` from `Account.IsVerified()`
  (`authdomain.Account`, already available via `AccountRepo.ByID`) — **not** from the access
  token claim already on the request, even though the two must always agree. Reading it from the
  account record on every call is one extra field read on a repo call the handler already makes;
  reading it from the claim would work today but silently goes stale the instant a token is
  minted before verification and the account is verified afterward without a token refresh —
  the account row is the source of truth the claim is derived from, so read from the source.
