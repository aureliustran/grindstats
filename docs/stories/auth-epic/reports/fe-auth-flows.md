# Execution report: fe-auth-flows

**Slice ID:** `fe-auth-flows`
**Domain:** frontend
**Status:** Complete — all done-criteria passed

---

## 1. Changed

| File | What changed |
|---|---|
| `apps/web/src/features/auth/UnverifiedBanner.tsx` | Created persistent, dismissible notice for authenticated unverified accounts using design system tokens. |
| `apps/web/src/features/auth/VerifyEmailPage.tsx` | Created email verification landing screen handling token parsing, `verifyEmail` API call, and success / invalid-link states. |
| `apps/web/src/features/auth/PasswordResetRequestPage.tsx` | Created password reset request screen with client-side email format validation, neutral confirmation (FR-08), and 429 countdown formatting. |
| `apps/web/src/features/auth/PasswordResetConfirmPage.tsx` | Created set-new-password screen with length validation, `confirmPasswordReset` call, and specific error mapping for `rule: "breached"` vs `min_length` (without using server messages). |
| `apps/web/src/features/auth/OAuthLinkConfirmPage.tsx` | Created OAuth link confirmation screen handling token verification and linking confirmation. |
| `apps/web/src/features/auth/index.ts` | Exported all auth flow components. |
| `apps/web/src/features/auth/UnverifiedBanner.test.tsx` | Automated component test for AUTH-001 TC-09 asserting banner and write control states by translation key. |
| `apps/web/src/features/auth/authFlows.test.tsx` | Automated component and route tests covering verify-email, password reset flows, neutral responses, error mappings, token-less visits, and `vi-VN` formatting. |
| `apps/web/src/app/AuthContext.tsx` | Added `emailVerified` state to `AuthContext` and populated it from the session user probe. |
| `apps/web/src/app/Router.tsx` | Registered `/auth/verify-email`, `/auth/password-reset`, `/auth/password-reset/confirm`, and `/auth/oauth/link` routes, and handled `?auth_error=oauth_link_required` and `oauth_failed` shell notices. |
| `apps/web/src/app/pages/DashboardPage.tsx` | Mounted `UnverifiedBanner` and write control guarded by `emailVerified`. |
| `apps/web/src/i18n/locales/en-US.json` | Added `errors.auth_email_unverified`, `errors.auth_link_invalid`, and all `auth.*` flow keys. |
| `apps/web/src/i18n/locales/vi-VN.json` | Added corresponding Vietnamese translations with 100% key parity. |

---

## 2. Verified

### Automated Tests: `cd apps/web && npx vitest run`

```
 RUN  v5.0.0 D:/PROJECTS/grindstats/apps/web

 Test Files  3 passed (3)
      Tests  18 passed (18)
   Start at  16:59:45
   Duration  3.36s
```

| Scenario / Obligation | Test Name | Result |
|---|---|---|
| AUTH-001 TC-09 | `it("unverified account is read-only on its own data")` | PASS |
| Verify email success | `it("verify-email renders success state when valid token is supplied")` | PASS |
| Verify email invalid | `it("verify-email renders invalid-link state with way forward on AUTH_LINK_INVALID")` | PASS |
| Password reset request neutral copy (FR-08) | `it("password-reset request renders identical confirmation key for two different emails (FR-08)")` | PASS |
| Password reset validation mapping | `it("password-reset confirm renders distinct keys for breached vs min_length validation")` | PASS |
| Token-less route visit | `it("token-less visit renders invalid link state without calling API")` | PASS |
| OAuth link token check | `it("OAuth link confirm renders invalid token state when token is missing")` | PASS |
| Locale & Intl formatting | `it("renders countdown formatted through Intl.NumberFormat in vi-VN locale")` | PASS |

### i18n Parity Check: `py scripts/check_i18n_parity.py`
```
OK: [frontend] vi-VN.json matches en-US.json (187 keys)
OK: [server] vi-VN.json matches en-US.json (12 keys)
```

### TypeScript Check: `cd apps/web && npx tsc --noEmit`
Exited with 0 (clean).

### Production Build: `cd apps/web && npm run build`
```
✓ 1638 modules transformed.
dist/index.html                   1.05 kB
dist/assets/index-CeJlwbnT.css   31.04 kB
dist/assets/index-zA7OdWsB.js   320.92 kB
✓ built in 3.00s
```

---

## 3. Could not do

None. All screens, banner, route wiring, i18n keys, and automated scenario tests have been implemented and validated inside the designated allowlist.
