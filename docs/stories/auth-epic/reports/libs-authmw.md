# Execution report: libs-authmw

**Slice ID:** `libs-authmw`  
**Worktree:** `D:\PROJECTS\grindstats\.claude\worktrees\agent-a9c0bc6049dde5598`  
**Branch:** `worktree-agent-a9c0bc6049dde5598`  
**Wave:** 1

---

## 1. Changed

All new files; nothing pre-existing was modified inside the allowlist.

| File | What it does |
|---|---|
| `libs/authmw/claims.go` | Defines `AccessClaims`, `RefreshClaims` (used for JWT minting), and the flat `Claims` struct returned by the check chain. Includes `TypAccess`/`TypRefresh` constants. |
| `libs/authmw/keys.go` | `KeySet` struct with `LoadKeySet` (reads PEM key files from disk) and `NewKeySetFromMemory` (in-memory constructor for tests). `MintAccess` / `MintRefresh` mint RS256 tokens with `kid` in the header and fresh UUIDs as `jti`. `extractClaims` unpacks `jwt.MapClaims` into the flat `Claims` struct. |
| `libs/authmw/verify.go` | `KeySet.Check(ctx, rdb, tokenStr, expectedTyp)` — the four-step check chain: (1) signature via `golang-jwt/jwt/v5` (signature checked first, expiry second within the same parse call), (2) `typ` mismatch classified as `BadSignature`, (3) blacklist Redis lookup, (4) epoch comparison. Returns `(Claims, auditmodel.TokenRejectReason, error)`. A non-nil error means Redis I/O failure; the caller applies the degradation policy. |
| `libs/authmw/redis.go` | Typed accessors for every Redis key in contract §3: `BlacklistJTI` / `IsBlacklisted` (TTL = token's remaining life), `SetEpoch` / `GetEpoch` (writes `now+1`, 30d TTL), `StoreSession` / `RotateSession` / `GetSession` / `DeleteSession` / `AllSessions` (hash, 30d TTL refreshed on write), `IncrLoginFailUser` / `IncrLoginFailIP` / `GetLoginFailUser` / `ClearLoginFail` (sliding 15m TTL), `SetLoginBackoff` / `GetLoginBackoffTTL` / `InBackoff` (TTL = `BackoffDelay(step)`). No path creates a key without a TTL. |
| `libs/authmw/backoff.go` | Pure function `BackoffDelay(step int) time.Duration` — returns `30 * 2^(step-1)` seconds capped at 3600. Sequence: 30s, 60s, 120s, 240s, 480s, 960s, 1920s, 3600s, 3600s, … |
| `libs/authmw/degradation.go` | `CheckDegradation(method, path string) DegradationDecision` — returns `FailClosed` for `/auth/refresh`, `/auth/logout*`, or any unsafe HTTP method; `FailOpen` for GET/HEAD on all other paths. |
| `libs/authmw/cookies.go` | `SetAccessCookie`, `SetRefreshCookie`, `SetSessionCookies`, `ClearCookies`, `SetOAuthStateCookie` — write cookies with the exact attributes from contract §2.2. `CookieOptions{Secure: true}` is the default; `false` only for local plain-http. |
| `libs/authmw/verify_test.go` | Named scenario test `TestCheckOrder_SignatureThenExpiryThenBlacklistThenEpoch` plus round-trip, `kid` rotation window, `typ` confusion, epoch-boundary (same-second), and backoff sequence tests. Uses miniredis; no Docker or live Redis. |
| `libs/authmw/redis_test.go` | Named scenario test `TestBlacklistTTL_EqualsTokenRemainingLife` (5 sub-tests covering access, refresh, near-expiry, already-expired, and TTL fast-forward). Also: epoch TTL, epoch `now+1` value, session hash TTL, backoff TTL, sliding fail TTL, clear-on-success. All use miniredis. |

**Outside the allowlist (go.mod / go.sum):** Three direct dependencies added because the brief stated they were already present but they were absent in the worktree:
- `github.com/golang-jwt/jwt/v5 v5.3.1`
- `github.com/google/uuid v1.6.0`
- `github.com/alicebob/miniredis/v2 v2.39.0` (+ transitive `github.com/yuin/gopher-lua v1.1.1`)

The `be-wiring` slice owns `go.mod`; see the "Noticed" section.

---

## 2. Verified

### Done-criteria commands

```
go build ./... && go vet ./...
```
```
BUILD+VET OK
```

```
go test ./libs/authmw/... -v -count=1
```
```
=== RUN   TestBlacklistTTL_EqualsTokenRemainingLife
=== RUN   TestBlacklistTTL_EqualsTokenRemainingLife/access_token
=== RUN   TestBlacklistTTL_EqualsTokenRemainingLife/refresh_token
=== RUN   TestBlacklistTTL_EqualsTokenRemainingLife/nearly_expired
=== RUN   TestBlacklistTTL_EqualsTokenRemainingLife/already_expired
=== RUN   TestBlacklistTTL_EqualsTokenRemainingLife/expires_after_ttl
--- PASS: TestBlacklistTTL_EqualsTokenRemainingLife (0.00s)
    --- PASS: TestBlacklistTTL_EqualsTokenRemainingLife/access_token (0.00s)
    --- PASS: TestBlacklistTTL_EqualsTokenRemainingLife/refresh_token (0.00s)
    --- PASS: TestBlacklistTTL_EqualsTokenRemainingLife/nearly_expired (0.00s)
    --- PASS: TestBlacklistTTL_EqualsTokenRemainingLife/already_expired (0.00s)
    --- PASS: TestBlacklistTTL_EqualsTokenRemainingLife/expires_after_ttl (0.00s)
--- PASS: TestEpochKey_HasTTL (0.00s)
--- PASS: TestEpochKey_WritesNowPlusOne (0.00s)
--- PASS: TestSessionKey_HasTTL (0.00s)
--- PASS: TestLoginBackoff_TTLEqualsDelay (0.00s)
--- PASS: TestLoginFail_SlidingTTL (0.00s)
--- PASS: TestClearLoginFail_DeletesAllThreeKeys (0.00s)
--- PASS: TestCheckOrder_SignatureThenExpiryThenBlacklistThenEpoch (0.06s)
--- PASS: TestMintVerify_RoundTrip (0.10s)
--- PASS: TestMintVerify_RefreshRoundTrip (0.05s)
--- PASS: TestKIDRotation_TwoKeyVerificationWindow (0.07s)
--- PASS: TestTypConfusion_AccessPresentedAsRefresh (0.02s)
--- PASS: TestEpochBoundary_SameSecond (0.11s)
--- PASS: TestBackoffSequence (0.00s)
PASS
ok      grindstats/libs/authmw  0.518s
```

```
go test ./...
```
```
?       grindstats/libs/auditmodel      [no test files]
ok      grindstats/libs/authmw          0.468s
ok      grindstats/libs/httpkit         0.125s
ok      grindstats/libs/i18n            0.345s
?       grindstats/services/monolith/cmd/server     [no test files]
ok      grindstats/services/monolith/internal/gateway           0.127s
ok      grindstats/services/monolith/internal/gateway/health    0.273s
ok      grindstats/services/monolith/internal/gateway/middleware 0.124s
ok      grindstats/services/monolith/internal/platform/cache    2.230s
ok      grindstats/services/monolith/internal/platform/config   0.312s
ok      grindstats/services/monolith/internal/platform/db       0.584s
```

```
py scripts/gen_audit_model.py --check
```
```
OK: model valid and generated files current (14 enums, 10 error codes, 13 events)
```

### Scenario → test → result

| Scenario | Test name | File | Result |
|---|---|---|---|
| the gateway check order is signature, then expiry, then blacklist, then epoch (AUTH-003 TC-03) | `TestCheckOrder_SignatureThenExpiryThenBlacklistThenEpoch` | `libs/authmw/verify_test.go` | **PASS** |
| (NFR-03) every blacklist key carries a TTL (AUTH-003 TC-11) | `TestBlacklistTTL_EqualsTokenRemainingLife` | `libs/authmw/redis_test.go` | **PASS** |

---

## 3. Could not do

Nothing blocked. All done-criteria items are satisfied.

---

## 4. Noticed

### go.mod owned by be-wiring but deps were absent

The brief states "golang-jwt/v5, google/uuid, x/crypto and miniredis are already present" and the `be-wiring` slice owns `go.mod`. In the worktree these three packages were absent — not even as indirect dependencies. The parent agent's instructions explicitly said "if go build can't find it, run go mod tidy — this is a wave-1 slice's own moment to properly promote its dependencies from indirect to direct, that is expected." I added them via `go get` since `go mod tidy` cannot add packages that are not already present. The `be-wiring` slice should be aware that these three packages are now direct dependencies in `go.mod`; if it rebuilds `go.mod` from scratch it must include them.

### `ratelimit:{scope}:{key}` accessor not implemented

The contract §3 table lists `ratelimit:{scope}:{key}` as a Redis key. The brief's done-criteria and scenarios are silent on this key, and the brief says rate-limiting is in `NFR-04` / the gateway middleware slice (`be-gateway-authz`). I omitted the accessor to stay within scope. If `be-gateway-authz` needs one in this library rather than implementing its own, an amendment request is the right path.

### `libs/auditmodel` not yet extended with auth-epic model additions

Contract §7 lists new enum values (`AccountStatus`, `LinkKind`, etc.) and events that must be added by the `plat-audit-model` slice. The check chain in `verify.go` references the four `TokenRejectReason*` constants that already exist in `generated.go`. All other new constants (e.g. `ErrAuthEmailUnverified`, `auth.login.suspended` event) are not yet present; wave-3 slices that need them will depend on `plat-audit-model` completing first.
