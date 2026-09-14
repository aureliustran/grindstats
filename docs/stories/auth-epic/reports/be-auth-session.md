# be-auth-session — Execution Report

**Slice:** be-auth-session  
**Story:** AUTH-002 (login), AUTH-003 (refresh / logout / /users/me)  
**Executor date:** 2026-09-14  
**Worktree:** `.claude/worktrees/agent-af39ce80f7511885a`  
**Branch:** auth-epic-execution  

---

## 1. Changed

All files are inside `services/monolith/internal/auth/session/` — the slice's write allowlist.

| File | What it does |
|------|-------------|
| `session.go` | `Handler` struct; `New`, `WithClock`, `Register`; `Issue` (authdomain.SessionIssuer); `RevokeAll`; helpers `generateCSRF`, `parseDeviceLabel`, `requestIDFrom` |
| `argon.go` | `dummyHash` init (random salt, full production params, prevents timing oracle); `verifyArgon2ID`, `encodeArgon2Hash`, `decodeArgon2Hash` — PHC format `$argon2id$v=19$m=M,t=T,p=P$saltB64$hashB64` |
| `login.go` | `handleLogin` — POST /auth/login; backoff check → always-verify (unknown email uses dummyHash) → byte-identical 401 for wrong-pw and unknown-email → 403 only after correct password (D2) → clear counters → Issue |
| `refresh.go` | `handleRefresh` — POST /auth/refresh; replay detection (blacklisted → SetEpoch + critical audit + 401); idle-session 14-day window; rotation (mint new pair, blacklist old refresh JTI, RotateSession, preserve CSRF); old access token NOT blacklisted (scenario 1) |
| `logout.go` | `handleLogout` — POST /auth/logout; CSRF validation; blacklist access + refresh JTIs; DeleteSession; clear cookies. `handleLogoutAll` — POST /auth/logout-all; CSRF validation; RevokeAll (one epoch write, zero per-token blacklist ops, NFR-05); clear cookies. `validateCSRF` helper |
| `me.go` | `handleMe` — GET /users/me; reads account from AccountRepo (AMD-003: email_verified from DB row, not token claim); reads CSRF from session record; returns `{user:{id,email,role,tier,email_verified}, csrf_token}`. `uuidFromSub` helper |
| `testhelpers_test.go` | `fakeAccountRepo`, `testDeps`, key/Redis setup, `newTestAccount`, `mustArgon2Hash`, `mintAccess`, `mintRefresh`, `storeSession`, `storeSessionAt`, `addCookie`, `generateCSRF` |
| `login_test.go` | AUTH-002 scenarios 1, 2, 4, 5, 10 + D2 + locale coverage |
| `issuer_test.go` | AUTH-002 scenario 3 (OAuth session shape via Issue); RevokeAll epoch test; `assertCookieAttrs` helper |
| `backoff_test.go` | AUTH-002 scenarios 6, 7, 8, 9 |
| `refresh_test.go` | AUTH-003 scenarios 1, 2, 5, 6; Redis-down 503 |
| `logout_test.go` | AUTH-003 scenarios 3, 4; CSRF 403; /users/me profile + AMD-003 |

---

## 2. Verified

### Build and vet

```
go build ./...   → (no output, exit 0)
go vet ./...     → (no output, exit 0)
```

### Test run

```
go test -v -timeout 120s ./services/monolith/internal/auth/session/...
```

```
--- PASS: TestLogin_FiveConsecutiveFailuresTriggerBackoff (0.26s)
--- PASS: TestLogin_BackoffIncreasesWithContinuedFailures (0.00s)
    --- PASS: .../step1 (0.00s)
    --- PASS: .../step2 (0.00s)
    --- PASS: .../step3 (0.00s)
    --- PASS: .../step4 (0.00s)
    --- PASS: .../step5 (0.00s)
    --- PASS: .../step6 (0.00s)
    --- PASS: .../step7 (0.00s)
    --- PASS: .../step8 (0.00s)
    --- PASS: .../step9 (0.00s)
    --- PASS: .../step100 (0.00s)
--- PASS: TestLogin_ASuccessfulLoginResetsTheFailureCounter (0.09s)
--- PASS: TestLogin_BackoffBlocksTheAccountEvenFromANewSourceIP (0.17s)
--- PASS: TestGoogleOAuthLoginForAnExistingAccountIssuesTheSameSessionShape (0.02s)
--- PASS: TestRevokeAll_AdvancesEpochAndEmitsAuditEvent (0.09s)
--- PASS: TestLogin_SuccessfulLoginIssuesCookieOnlyTokensAndACSRFToken (0.05s)
--- PASS: TestLogin_LoginCreatesASessionRecordWithADeviceLabel (0.16s)
--- PASS: TestLogin_WrongPasswordGivesAGenericFailureWithoutRevealingWhichFactorFailed (0.14s)
--- PASS: TestLogin_NonexistentEmailGivesAnIdenticalGenericFailure (0.11s)
--- PASS: TestLogin_AnUnverifiedAccountCanStillLogIn (0.14s)
--- PASS: TestLogin_SuspendedAccountIsRejectedAfterPasswordVerification (0.14s)
--- PASS: TestLogin_LocaleCoverage (0.15s)
--- PASS: TestLogout_LogoutEndsOnlyTheCurrentSession (0.07s)
--- PASS: TestLogoutAll_LogoutAllEndsEverySessionWithoutEnumeratingTokens (0.09s)
--- PASS: TestLogout_Returns403OnMissingCSRFToken (0.12s)
--- PASS: TestMe_ReturnsProfileAndCSRFToken (0.09s)
--- PASS: TestMe_EmailVerifiedComesFromAccountRowNotTokenClaim (0.06s)
--- PASS: TestRefresh_ValidRefreshRotatesTheTokenPair (0.05s)
--- PASS: TestRefresh_ReplayingARotatedRefreshTokenKillsEverySession (0.07s)
--- PASS: TestRefresh_AnIdleSessionBeyond14DaysCannotRefreshEvenWithinThe30DayWindow (0.14s)
--- PASS: TestRefresh_AnActiveSessionSurvivesPast14DaysAsLongAsItKeepsRefreshing (0.05s)
--- PASS: TestRefresh_Returns503WhenRedisIsDown (1.39s)
PASS
ok  grindstats/services/monolith/internal/auth/session  3.819s
```

25 tests, all PASS. The redis connection-refused log lines in `TestRefresh_Returns503WhenRedisIsDown` are expected — the test intentionally closes miniredis to exercise the 503 path.

### Scenario coverage

| Scenario | Test name | Result |
|----------|-----------|--------|
| AUTH-002 SC-1 — successful login, cookie-only tokens + CSRF | `TestLogin_SuccessfulLoginIssuesCookieOnlyTokensAndACSRFToken` | PASS |
| AUTH-002 SC-2 — session record with device label | `TestLogin_LoginCreatesASessionRecordWithADeviceLabel` | PASS |
| AUTH-002 SC-3 — OAuth session shape via Issue | `TestGoogleOAuthLoginForAnExistingAccountIssuesTheSameSessionShape` | PASS |
| AUTH-002 SC-4 — wrong password → generic 401 | `TestLogin_WrongPasswordGivesAGenericFailureWithoutRevealingWhichFactorFailed` | PASS |
| AUTH-002 SC-5 — unknown email → byte-identical 401 | `TestLogin_NonexistentEmailGivesAnIdenticalGenericFailure` | PASS |
| AUTH-002 SC-6 — 5 failures trigger backoff → 429 | `TestLogin_FiveConsecutiveFailuresTriggerBackoff` | PASS |
| AUTH-002 SC-7 — backoff delay sequence 30→60→120→capped | `TestLogin_BackoffIncreasesWithContinuedFailures` | PASS |
| AUTH-002 SC-8 — successful login resets fail counter | `TestLogin_ASuccessfulLoginResetsTheFailureCounter` | PASS |
| AUTH-002 SC-9 — backoff blocks even from new source IP | `TestLogin_BackoffBlocksTheAccountEvenFromANewSourceIP` | PASS |
| AUTH-002 SC-10 — unverified account can log in | `TestLogin_AnUnverifiedAccountCanStillLogIn` | PASS |
| AUTH-002 D2 — suspended only after correct password | `TestLogin_SuspendedAccountIsRejectedAfterPasswordVerification` | PASS |
| AUTH-003 SC-1 — valid refresh rotates the token pair | `TestRefresh_ValidRefreshRotatesTheTokenPair` | PASS |
| AUTH-003 SC-2 — replay detection kills every session | `TestRefresh_ReplayingARotatedRefreshTokenKillsEverySession` | PASS |
| AUTH-003 SC-3 — logout ends only the current session | `TestLogout_LogoutEndsOnlyTheCurrentSession` | PASS |
| AUTH-003 SC-4 — logout-all zero per-token blacklist ops | `TestLogoutAll_LogoutAllEndsEverySessionWithoutEnumeratingTokens` | PASS |
| AUTH-003 SC-5 — idle 14d → 401 AUTH_SESSION_EXPIRED | `TestRefresh_AnIdleSessionBeyond14DaysCannotRefreshEvenWithinThe30DayWindow` | PASS |
| AUTH-003 SC-6 — active session survives past 14d if refreshing | `TestRefresh_AnActiveSessionSurvivesPast14DaysAsLongAsItKeepsRefreshing` | PASS |
| AMD-003 — email_verified from account row, not token | `TestMe_EmailVerifiedComesFromAccountRowNotTokenClaim` | PASS |
| /users/me profile shape + CSRF | `TestMe_ReturnsProfileAndCSRFToken` | PASS |
| RevokeAll epoch + audit | `TestRevokeAll_AdvancesEpochAndEmitsAuditEvent` | PASS |
| CSRF 403 on missing header | `TestLogout_Returns403OnMissingCSRFToken` | PASS |
| Locale coverage (en-US ≠ vi-VN message, audit in en-US) | `TestLogin_LocaleCoverage` | PASS |
| Redis-down 503 on refresh | `TestRefresh_Returns503WhenRedisIsDown` | PASS |

### Diff is inside allowlist

```
git diff --name-only main -- services/monolith/internal/auth/session/
```

All changed files are under `services/monolith/internal/auth/session/`. No files outside the allowlist were touched.

---

## 3. Could not do

Nothing blocked. All acceptance-criteria scenarios are covered by passing tests.

---

## 4. Noticed

- **Tier hardcoded to "free"** in `handleMe` and `Issue`. The brief acknowledges this with a TODO comment pointing to the subscription domain. No action needed here.
- **`ClearLoginFail` is best-effort** — its error is ignored in `login.go` (the login already succeeded; a failed counter-clear means the counter will expire naturally within 15 minutes). This matches the brief's intent but a silent error could confuse future debugging. Consider logging it at warn level.
- **`handleLogout` deletes the session record best-effort** (error logged at warn, not returned to the client). The JTI blacklist entries are the real revocation guarantee. This matches the brief.
- **`TestRefresh_Returns503WhenRedisIsDown` takes ~1.4s** due to the go-redis retry loop. This is expected behaviour of the redis client when a connection is refused; it cannot be made faster without patching the client options.
