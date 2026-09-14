# Executor report: be-auth-credentials

**Slice:** `be-auth-credentials`
**Branch:** `auth-epic-execution`
**Worktree:** `D:\PROJECTS\grindstats\.claude\worktrees\agent-aa4cd8dcccc450e85`
**Date:** 2026-09-14

---

## 1. Changed

| File | What it does |
|---|---|
| `services/monolith/internal/auth/hibp/client.go` | HIBP k-anonymity HTTP client. `New(enabled, timeout)` for production. `NewWithURL(enabled, timeout, baseURL)` for tests pointing at a mock server. Fail-open: returns `(false, err)` on network/HTTP errors; callers skip the check on any error (D3). `AUTH_HIBP_ENABLED=false` → always `(false, nil)`. |
| `services/monolith/internal/auth/hibp/hibp_test.go` | Unit tests for hit, miss, unreachable, disabled, and padding-entry cases. Uses `httptest.Server` to avoid live HIBP calls. |
| `services/monolith/internal/auth/mailer/mailer.go` | `Mailer` interface with `SendVerification`, `SendPasswordReset`, `SendOAuthLink`. `DevMailer` logs deep-links to the structured log (opt-in via `AUTH_DEV_MAILER=true`). Never logs a raw token in production. |
| `services/monolith/internal/auth/credentials/argon2.go` | `PasswordHasher` with argon2id hash/verify and `TimingDummyVerify`. `DefaultArgon2Params` (t=3, m=64MB, p=4) for production. `FastArgon2Params` (t=1, m=8MB, p=1) for tests. Pre-computes a dummy hash at construction for timing equalization (FR-08, §1.1). |
| `services/monolith/internal/auth/credentials/handler.go` | `Handler` implementing endpoints #1–4, #7 (register, verify-email, password-reset/request, password-reset/confirm, oauth/link/confirm). `Register(r gin.IRouter)` for wiring. Contains the `writeValidationError` interim helper (see Noticed). Emits all required audit events. Timing equalization implemented: argon2id hash always runs before `ErrEmailTaken` on register; `TimingDummyVerify` runs on password-reset/request regardless of address existence. |
| `services/monolith/internal/auth/credentials/provisioner.go` | `Provisioner` implementing `authdomain.AccountProvisioner`. `ProvisionFromOAuth` checks identity row → checks local account → creates User account with verified email → links identity → emits `auth.register.succeeded` with `via=provider`. Returns `ErrLinkRequired` when a local account owns the email without an identity row. |
| `services/monolith/internal/auth/credentials/testfakes_test.go` | Shared test fakes: `fakeAccountRepo`, `fakeOAuthIdentityRepo`, `fakeLinkTokenRepo`, `fakeSessionIssuer`, `fakeHIBP`, `fakeMailer`. All implement the `authdomain` interfaces exactly; compile-time checks at bottom. |
| `services/monolith/internal/auth/credentials/register_test.go` | Handler tests for TC-01,02,03,04,05,TC-06,TC-14 plus locale-rendering test. |
| `services/monolith/internal/auth/credentials/verify_test.go` | Handler tests for TC-10, TC-11. |
| `services/monolith/internal/auth/credentials/reset_test.go` | Handler tests for TC-12, TC-13, TC-15. |
| `services/monolith/internal/auth/oauth/handler.go` | `Handler` implementing endpoints #5, #6 (authorize + callback). Authorization-code + PKCE (S256). State+verifier stored in HMAC-SHA256-signed `gs_oauth_state` cookie. Callback validates state, exchanges code, fetches Google userinfo, calls `AccountProvisioner.ProvisionFromOAuth`. On `ErrLinkRequired` → issues `oauth_link` token, emails it, emits `auth.oauth.link_required`, redirects to `/?auth_error=oauth_link_required`. `SetTestEndpoints` / `SetTestHTTPClient` allow test injection without live Google calls. |
| `services/monolith/internal/auth/oauth/testfakes_test.go` | Shared test fakes for the oauth package. |
| `services/monolith/internal/auth/oauth/provision_test.go` | Service-level tests for TC-07 plus existing-linked and ErrLinkRequired cases. |
| `services/monolith/internal/auth/oauth/callback_test.go` | Handler-level tests for TC-08 plus user-denied, bad-state, and authorize-flow cases. |

**Self-heal copies** (stale worktree, see Noticed):
- `libs/auditlog/*.go` — copied verbatim from `D:/PROJECTS/grindstats/libs/auditlog/`
- `libs/authmw/*.go` — copied verbatim from `D:/PROJECTS/grindstats/libs/authmw/`
- `libs/auditmodel/generated.go`, `libs/auditmodel/model.yaml` — copied verbatim
- `libs/i18n/locales/en-US.json`, `libs/i18n/locales/vi-VN.json` — copied verbatim (main checkout had `AUTH_EMAIL_UNVERIFIED` and `AUTH_LINK_INVALID` entries added by `plat-audit-model` slice; worktree was behind)
- `go.mod`, `go.sum` — initial copy from main; then `go get golang.org/x/oauth2` added that dependency and `go mod tidy` ran. Final `go.mod` has `golang.org/x/oauth2 v0.37.0` added to `require`.

---

## 2. Verified

### Commands

```
go build ./... && go vet ./...
```
Output: (no output — clean)

```
go test -timeout 180s ./...
```
Output:
```
ok  grindstats/libs/auditlog        0.508s
ok  grindstats/libs/authmw          0.776s
ok  grindstats/libs/httpkit         0.245s
ok  grindstats/libs/i18n            0.373s
ok  grindstats/services/monolith/internal/auth/credentials  0.468s
ok  grindstats/services/monolith/internal/auth/hibp         0.967s
ok  grindstats/services/monolith/internal/auth/oauth        0.218s
ok  grindstats/services/monolith/internal/auth/store        0.159s  (DB tests skipped — no GRINDSTATS_TEST_DB)
ok  grindstats/services/monolith/internal/gateway           0.201s
ok  grindstats/services/monolith/internal/gateway/health    0.364s
ok  grindstats/services/monolith/internal/gateway/middleware 0.228s
ok  grindstats/services/monolith/internal/platform/cache    2.468s
ok  grindstats/services/monolith/internal/platform/config   0.273s
ok  grindstats/services/monolith/internal/platform/db       0.817s
```
All tests green.

### Scenario → test → result

| Scenario | Test name | Result |
|---|---|---|
| registering with a new email creates a User account (TC-01) | `TestRegister_registering_with_a_new_email_creates_a_User_account` | PASS |
| email uniqueness is case-insensitive (TC-02) | `TestRegister_email_uniqueness_is_case_insensitive` | PASS |
| password below the minimum length is rejected (TC-03) | `TestRegister_password_below_the_minimum_length_is_rejected` | PASS |
| a breached password is rejected or flagged — hit (TC-04) | `TestRegister_a_breached_password_is_rejected_or_flagged_hit` | PASS |
| a breached password is rejected or flagged — miss (TC-05) | `TestRegister_a_breached_password_is_rejected_or_flagged_miss` | PASS |
| a breached password is rejected or flagged — unreachable → allowed (TC-04 D3) | `TestRegister_a_breached_password_is_rejected_or_flagged_unreachable` | PASS |
| an injected role field is ignored on registration (TC-06) | `TestRegister_an_injected_role_field_is_ignored_on_registration` | PASS |
| registration response does not confirm email is already registered (TC-14) | `TestRegister_registration_response_does_not_confirm_an_email_is_already_registered` | PASS (byte-equal assert) |
| verification token expires after 24 hours (TC-10) | `TestVerifyEmail_verification_token_expires_after_24_hours` | PASS |
| verification token is single-use (TC-11) | `TestVerifyEmail_verification_token_is_single_use` | PASS (account remains verified) |
| password reset completes and logs out every session (TC-12) | `TestReset_password_reset_completes_and_logs_out_every_session` | PASS (RevokeAll called once) |
| expired or already-used reset token is rejected (TC-13) | `TestReset_expired_or_already_used_reset_token_is_rejected` | PASS (RevokeAll NOT called, password unchanged) |
| password-reset request does not confirm whether the email exists (TC-15) | `TestReset_password_reset_request_does_not_confirm_whether_the_email_exists` | PASS (byte-equal assert) |
| first Google login auto-creates an account (TC-07) | `TestProvision_first_Google_login_auto_creates_an_account` | PASS |
| OAuth login matching an existing local account requires proof of control (TC-08) | `TestCallback_OAuth_login_matching_an_existing_local_account_requires_proof_of_control` | PASS (no session, redirect=oauth_link_required, audit event written, token emailed) |
| HIBP hit returns true (unit, TC-04) | `TestIsBreached_hit` | PASS |
| HIBP miss returns false (unit, TC-05) | `TestIsBreached_miss` | PASS |
| HIBP unreachable returns error (unit) | `TestIsBreached_unreachable` | PASS |
| HIBP disabled returns false (unit) | `TestIsBreached_disabled` | PASS |
| Locale: VALIDATION_FAILED message differs between en-US and vi-VN, audit is en-US | `TestRegister_locale_renders_different_messages_for_validation_error` | PASS |

### Enumeration pairs asserted byte-equal

- TC-14 (register, new vs taken email): `assert.Equal(t, w1.Body.String(), w2.Body.String())` ✓
- TC-15 (password-reset/request, known vs unknown email): `assert.Equal(t, wKnown.Body.String(), wUnknown.Body.String())` ✓

### AccountProvisioner / SessionIssuer interface compliance

- `credentials.Provisioner` implements `authdomain.AccountProvisioner` — confirmed by compile-time check `var _ authdomain.AccountProvisioner = (*Provisioner)(nil)` in provision_test.go.
- `SessionIssuer` is consumed only through the interface; no peer wave-3 package imported.

### Register(r gin.IRouter) exists

`credentials.Handler.Register` and `oauth.Handler.Register` both exist; nothing in either package edits `main.go` or gateway files.

---

## 3. Could not do

Nothing blocked. All scenarios covered and passing.

The `writeValidationError` local helper (see Noticed) is an acknowledged interim measure; no amendment was required to unblock the work.

---

## 4. Noticed

### Worktree self-heal (expected, not a contract question)

The worktree was forked from a stale base — `libs/auditlog/`, `libs/authmw/`, `libs/auditmodel/generated.go`, `libs/auditmodel/model.yaml`, `libs/i18n/locales/en-US.json`, `libs/i18n/locales/vi-VN.json` were missing or outdated. All were copied verbatim from `D:/PROJECTS/grindstats` (the main checkout, branch `auth-epic-execution`). No content was invented — these are the real, already-merged files. The `go.mod` / `go.sum` were also copied as a base, then `golang.org/x/oauth2` was added by `go get` and `go mod tidy`.

### libs/httpkit.Error has no details parameter (known gap)

`httpkit.Error(c, code)` produces `{"error":{"code","message"}}` with no way to attach a `details` array. The `VALIDATION_FAILED` contract shape requires `details:[{field,rule}]` for `min_length` and `breached`. A local helper `writeValidationError` in `credentials/handler.go` produces the identical envelope shape using `c.AbortWithStatusJSON` directly. It is clearly marked as interim and points at the missing httpkit feature. A tester should flag this for an httpkit amendment in a future run.

### No `auth.register.succeeded` for regular registration (separate from OAuth)

The event is emitted correctly on the `/auth/register` path and on `ProvisionFromOAuth` (with `via=provider`). No gap — confirmed by TC-01 assertion.

### `POST /auth/oauth/link/confirm` audit event not specified

The contract §7 does not list an audit event for a successful `oauth_link` confirm. None was emitted. If one is needed, an amendment to the audit model should be filed for run 2.
