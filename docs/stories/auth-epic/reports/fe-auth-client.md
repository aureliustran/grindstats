# Execution report: fe-auth-client

**Slice ID:** `fe-auth-client`
**Domain:** frontend
**Status:** Complete — all done-criteria passed

---

## 1. Changed

| File | What changed |
|---|---|
| `apps/web/src/api/auth.types.ts` | Added `ValidationDetail`, extended `ApiError` with optional `details?: ValidationDetail[]`, added request/response types for `verifyEmail`, `requestPasswordReset`, `confirmPasswordReset`, and `confirmOAuthLink`, and extended `AuthApi` with the four new methods while preserving original signatures. |
| `apps/web/src/api/http.ts` | Created low-level HTTP transport wrapper enforcing `credentials: "include"`, `Accept-Language`, `X-CSRF-Token` on unsafe non-exempt endpoints, in-memory CSRF token handling, envelope unwrapping (`{ data: ... }` / `{ error: ... }`), and single-flight reactive refresh on 401 `AUTH_INVALID_TOKEN`. |
| `apps/web/src/api/auth.ts` | Switched default `authApi` export to `realImpl` (backed by `http.ts`), while preserving `mockImpl` under `VITE_AUTH_MOCK=true`. Updated `mockImpl` to support all 9 `AuthApi` methods with matching unwrapped responses. |
| `apps/web/src/api/auth.test.ts` | Added comprehensive automated test suite with 10 passing tests covering AUTH-002 TC-01 and all conformance requirements. |
| `apps/web/package.json` | Installed devDependencies (`vitest`, `jsdom`, `@testing-library/react`, etc.) and configured test script per approved Phase 0 pre-step. |
| `apps/web/vite.config.ts` | Configured Vitest environment with `jsdom` and globals. |

---

## 2. Verified

### Automated Tests: `cd apps/web && npx vitest run`

```
 RUN  v5.0.0 D:/PROJECTS/grindstats/apps/web

 Test Files  1 passed (1)
      Tests  10 passed (10)
   Start at  16:31:11
   Duration  3.48s
```

| Scenario / Obligation | Test Name | Result |
|---|---|---|
| AUTH-002 TC-01 | `it("successful login issues cookie-only tokens and a CSRF token")` | PASS |
| Envelope unwrapping & error parsing | `it("unwraps success envelope { data: ... } and parses error envelope into ApiRequestError")` | PASS |
| Transport headers & credentials | `it("includes Accept-Language and credentials: include on every request")` | PASS |
| CSRF exclusion & inclusion | `it("includes X-CSRF-Token on logout, and excludes it on unauthenticated endpoints and refresh")` | PASS |
| `me()` 401 handling | `it("me() resolves null on 401 rather than throwing")` | PASS |
| Retry-After parsing | `it("parses Retry-After into retryAfterSeconds, defaulting to 30 if absent")` | PASS |
| Transport failure mapping | `it("maps transport failure and network errors to ApiFailure{kind: 'network'}")` | PASS |
| Single-flight reactive refresh | `it("handles single-flight refresh: two concurrent 401s produce exactly 1 refresh call and retry once")` | PASS |
| Refresh failure terminal handling | `it("handles failed refresh: produces zero retries and rejects waiters")` | PASS |
| Mock swap-compatibility | `it("mock implementation returns same unwrapped shapes as real client")` | PASS |

### TypeScript Check: `cd apps/web && npx tsc --noEmit`
Exited with 0 (clean).

### Production Build: `cd apps/web && npm run build`
```
✓ 1632 modules transformed.
dist/index.html                   1.05 kB
dist/assets/index-DhnloeoD.css   29.11 kB
dist/assets/index-CT1x1Pbd.js   300.21 kB
✓ built in 4.01s
```

---

## 3. Could not do

None. All requirements, scenarios, and conformance obligations were completed and verified inside the allowlist (with testing dependencies installed as an approved pre-step).
