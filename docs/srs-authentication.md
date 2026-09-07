# GrindStats — Software Requirements Specification
## Module: Authentication & Session Management

| | |
|---|---|
| **Document ID** | SRS-AUTH-001 |
| **Version** | 1.0 |
| **Date** | 2026-08-26 |
| **Status** | Draft |
| **Author** | Aurelius (repo owner) |
| **Applies to** | auth package of `services/monolith/` (extracted to `auth-service` at roadmap Phase 10), gateway auth middleware, React SPA auth flows |
| **Source of truth** | GrindStats blueprint §8 (Auth), §10 (Redis), §19 (Security). Where this SRS and the blueprint disagree, the blueprint wins. |

---

## 1. Introduction

### 1.1 Purpose
This document specifies the functional and non-functional requirements for
authentication, authorization, and session management in GrindStats. It is
written for the solo developer implementing the system and for any AI
assistant generating code against it.

### 1.2 Scope
In scope: local (email + password) registration and login, OAuth2 social
login (Google; Apple later), JWT issuance and verification, refresh-token
rotation, the Redis blacklist revocation strategy, logout (single device and
everywhere), role-based access control for the two system roles, active-session
listing/revocation, and the related security controls (CSRF, rate limiting,
lockout).

Out of scope (covered elsewhere): OAuth2 *linking* of external accounts (Qwen
API key, POS purchase tokens — blueprint §8, subscription domain), entitlement
tiers (`free`/`pro` — billing domain, carried in the JWT but not defined here),
email delivery infrastructure, and user-profile CRUD beyond credentials.

### 1.3 Definitions

| Term | Meaning |
|---|---|
| **Access token** | Short-lived JWT (RS256) proving identity per request |
| **Refresh token** | Long-lived JWT used only to obtain new token pairs |
| **`jti`** | Unique JWT ID claim; the unit of revocation |
| **Blacklist** | Redis key `blacklist:{jti}`; presence ⇒ token revoked |
| **Epoch** | Redis key `user_blacklist_epoch:{user_id}`; tokens issued before it are invalid |
| **Session** | One logical login on one device: a refresh-token lineage from login to logout/expiry |
| **SPA** | The React single-page application client |
| **Gateway** | The API gateway/BFF — the only component that consults Redis for auth |

### 1.4 Roles

Exactly two system roles exist. Role is a claim (`role`) in the access token.

| Role | Description |
|---|---|
| **User** | Default role for every registered account. Accesses only their own data (metrics, meals, routines, chat, subscription, sessions). |
| **SystemAdmin** | Operational role for the platform owner. Everything a User can do on their own account, plus the administrative capabilities in §3.6. Assigned only by direct database operation or by an existing SystemAdmin — never self-service, never via any registration path. |

---

## 2. Overall description

### 2.1 Context
Authentication is enforced at the gateway. The gateway verifies the JWT
signature, checks the Redis blacklist and epoch, then forwards the request;
downstream service packages verify signature only and never call Redis
(blueprint §8: statelessness preserved internally, paid for once at the edge).

### 2.2 Token model

| Property | Access token | Refresh token |
|---|---|---|
| Format | JWT, RS256 | JWT, RS256 |
| Lifetime | 10–15 min | 30 days absolute; idle timeout 14 days |
| Claims | `sub`, `role`, `tier`, `jti`, `iat`, `exp` | `sub`, `jti`, `iat`, `exp`, `typ:"refresh"` |
| Transport | httpOnly, Secure, SameSite=Lax cookie | httpOnly, Secure, SameSite=Lax cookie, path-scoped to `/api/v1/auth` |
| Revocation | blacklist / epoch | blacklist / epoch |

### 2.3 Assumptions and dependencies
- Redis is available to the gateway; PostgreSQL stores accounts.
- TLS terminates in front of the gateway (**CloudFront** in prod — there is no
  ALB in the current deployment, see [`deployment-aws.md`](deployment-aws.md);
  local compose is trusted-dev only).
- Password hashing and key management follow §5 (security requirements).
- The signing keypair is available to the auth package (private) and to all
  verifying services (public key only, distributable via config/JWKS).

---

## 3. Functional requirements

Requirement IDs are stable; do not renumber. Priority: **M** (must, Phase 1–2
per blueprint roadmap), **S** (should, same phase but droppable to a fallback),
**C** (could, later phase).

### 3.1 Registration & account

| ID | Requirement | Priority |
|---|---|---|
| FR-01 | The system shall allow registration with email + password. Email must be unique (case-insensitive). | M |
| FR-02 | Passwords shall be accepted only if ≥ 10 characters; the system shall check candidates against a breached-password list (e.g. HIBP k-anonymity API) and warn or reject on match. No composition rules (mandatory symbols etc.). | S |
| FR-03 | The system shall send a verification email with a single-use, expiring (24 h) token; unverified accounts may log in but shall be restricted to read-only use of their own data until verified. | S |
| FR-04 | Every account created through any registration path shall receive role `User`. There shall be no request parameter, header, or payload field through which a registrant can influence role assignment. | M |
| FR-05 | The system shall allow registration/login via Google OAuth2 (authorization-code flow with PKCE). First OAuth login auto-creates an account (role `User`, email from the verified OAuth claim). | M |
| FR-06 | If an OAuth email matches an existing local account, the system shall require the user to prove control of the local account (login or verified-email confirmation) before linking, rather than silently merging. | M |
| FR-07 | The system shall support a password-reset flow: request → single-use, expiring (1 h) emailed token → new password. Completing a reset shall set `user_blacklist_epoch` (log out everywhere). | M |
| FR-08 | Account enumeration shall be prevented: registration, login, and reset endpoints return indistinguishable responses whether or not the email exists. | M |

### 3.2 Login & token issuance

| ID | Requirement | Priority |
|---|---|---|
| FR-10 | On successful login (local or OAuth), the system shall mint an access token and a refresh token, each with a unique `jti`, and deliver both as httpOnly, Secure, SameSite=Lax cookies per §2.2. Tokens shall never appear in a response body, URL, or non-httpOnly storage. | M |
| FR-11 | On login, the system shall record the session in `user_sessions:{user_id}` (refresh `jti`, device label parsed from User-Agent, created-at). | S |
| FR-12 | On login, the system shall issue a CSRF token bound to the session, returned in the response body for the SPA to hold in memory and echo via `X-CSRF-Token` header. | M |
| FR-13 | Failed logins shall be counted per account and per source IP; after 5 consecutive failures the system shall require a delay (exponential backoff starting at 30 s) rather than a hard lockout. Counters reset on success. | M |
| FR-14 | Login shall never disclose which factor failed ("invalid email or password" only). | M |

### 3.3 Request authentication & authorization

| ID | Requirement | Priority |
|---|---|---|
| FR-20 | The gateway shall reject any request whose access token fails signature verification, is expired, has `jti` present in `blacklist:*`, or has `iat` earlier than `user_blacklist_epoch:{user_id}` — in that check order, with 401 and error code `AUTH_INVALID_TOKEN` (envelope per blueprint §15). | M |
| FR-21 | Downstream services shall verify signature and expiry only (no Redis). They shall trust `sub` and `role` claims from a gateway-forwarded token. | M |
| FR-22 | Authorization shall be role-checked in middleware (`libs/authmw`): endpoints declare `User` (default) or `SystemAdmin`. A `User` token on a SystemAdmin endpoint returns 403 `AUTH_FORBIDDEN`, not 404. | M |
| FR-23 | A `User` shall only ever read or write resources owned by their own `sub`. Ownership checks live in the service layer and shall not rely on client-supplied user IDs — the effective user ID is always taken from the token. | M |
| FR-24 | State-changing requests (POST/PATCH/PUT/DELETE) shall require a valid `X-CSRF-Token` matching the session's CSRF token; mismatch returns 403 `AUTH_CSRF_FAILED`. Safe methods (GET/HEAD) are exempt. | M |

### 3.4 Refresh, rotation & logout

| ID | Requirement | Priority |
|---|---|---|
| FR-30 | `POST /api/v1/auth/refresh` shall verify the refresh cookie (signature, expiry, blacklist, epoch), then mint a **new** access + refresh pair and immediately blacklist the old refresh `jti` (rotation). | M |
| FR-31 | If a blacklisted refresh token is presented, the system shall treat it as replay of a stolen token: set `user_blacklist_epoch:{user_id}` to now (revoking every session), log a security event, and return 401. | M |
| FR-32 | `POST /api/v1/auth/logout` shall blacklist the current access and refresh `jti`s (TTL = each token's remaining life), remove the session from `user_sessions`, and clear both cookies. | M |
| FR-33 | `POST /api/v1/auth/logout-all` shall set `user_blacklist_epoch:{user_id}` to now — O(1), no token enumeration. Also triggered by password change (FR-07) and by admin suspension (FR-42). | M |
| FR-34 | Sliding idle expiry: refresh shall be refused (401, `AUTH_SESSION_EXPIRED`) if the session has been idle beyond 14 days, even inside the 30-day absolute lifetime. | S |

### 3.5 Session visibility (User)

| ID | Requirement | Priority |
|---|---|---|
| FR-40 | A User shall be able to list their active sessions (device label, created-at, last-seen, "this device" marker) from `user_sessions:{user_id}`. | S |
| FR-41 | A User shall be able to revoke any single listed session; revocation blacklists that session's refresh `jti` (its access token dies naturally within ≤ 15 min, or immediately if presented and checked). | S |

### 3.6 SystemAdmin capabilities

All SystemAdmin actions shall be audit-logged (FR-45).

| ID | Requirement | Priority |
|---|---|---|
| FR-42 | A SystemAdmin shall be able to suspend/unsuspend a user account. Suspension sets the user's epoch (immediate logout everywhere) and causes subsequent logins to fail with `AUTH_ACCOUNT_SUSPENDED`. | M |
| FR-43 | A SystemAdmin shall be able to force-logout any user (set epoch) without suspending the account. | S |
| FR-44 | A SystemAdmin shall be able to view a user's session list and security events (login failures, refresh-replay incidents) — but never passwords, password hashes, or token values. | S |
| FR-45 | Every SystemAdmin action shall write an append-only audit record: actor `sub`, action, target user, timestamp, request ID. Audit records shall not be deletable via any API. | M |
| FR-46 | SystemAdmin role grant/revoke shall itself be a SystemAdmin-only action, shall not be exposed in the SPA UI (API/ops only), and shall set the target user's epoch so a new token pair with the updated role claim must be obtained. | M |
| FR-47 | A SystemAdmin shall not be able to read or modify a user's health data (metrics, meals, routines, chat) through admin endpoints. Admin scope is accounts and sessions, not content. | M |

### 3.7 API surface (summary)

All under `/api/v1/auth`, error envelope `{ "error": { "code", "message" } }`.

| Endpoint | Method | Auth | Purpose |
|---|---|---|---|
| `/register` | POST | none | FR-01 |
| `/login` | POST | none | FR-10..14 |
| `/oauth/google` + `/oauth/google/callback` | GET | none | FR-05 |
| `/verify-email` | POST | none (token in body) | FR-03 |
| `/password-reset/request`, `/password-reset/confirm` | POST | none (token in body) | FR-07 |
| `/refresh` | POST | refresh cookie | FR-30 |
| `/logout` | POST | access | FR-32 |
| `/logout-all` | POST | access | FR-33 |
| `/sessions` | GET | access | FR-40 |
| `/sessions/{id}` | DELETE | access | FR-41 |
| `/admin/users/{id}/suspend`, `/unsuspend`, `/force-logout` | POST | SystemAdmin | FR-42, FR-43 |
| `/admin/users/{id}/sessions`, `/security-events` | GET | SystemAdmin | FR-44 |
| `/admin/users/{id}/role` | PUT | SystemAdmin | FR-46 |

---

## 4. Non-functional requirements

| ID | Requirement |
|---|---|
| NFR-01 | **Latency:** gateway auth check (signature + blacklist + epoch) shall add ≤ 5 ms p95 with Redis co-located; downstream signature-only verification ≤ 1 ms p95. |
| NFR-02 | **Availability degradation:** if Redis is unreachable, the gateway shall fail **closed** for refresh/logout endpoints and fail **open** (signature-only) for read-only GET endpoints, logging loudly — availability of a fitness dashboard outweighs revocation lag on reads, but nothing state-changing proceeds unchecked. |
| NFR-03 | **Blacklist hygiene:** every `blacklist:*` key shall carry a TTL equal to the underlying token's remaining life; the blacklist shall require no manual cleanup. |
| NFR-04 | **Rate limiting:** login, register, refresh, and password-reset endpoints shall be rate-limited per IP and per account at the gateway (Redis token bucket, blueprint §10) — defaults: 10/min login, 5/min reset. |
| NFR-05 | **Scale target:** the design shall be sized for the solo-build reality (≤ 10k users) but contain no O(n)-in-users operation on the hot path (the epoch strategy exists precisely so logout-everywhere is O(1)). |
| NFR-06 | **Testability:** token issuance, verification, rotation, blacklist, and epoch logic shall live in `libs/authmw` + the auth package with unit tests covering every FR in §3.2–3.4, runnable without Docker (miniredis or equivalent). |
| NFR-07 | **Observability:** every 401/403 shall log: error code, `jti` (never the token), user ID if known, request ID. Refresh-replay events (FR-31) shall be flagged at WARN+. |

---

## 5. Security requirements

| ID | Requirement |
|---|---|
| SEC-01 | Passwords hashed with **argon2id** (fallback: bcrypt cost ≥ 12). Plaintext passwords never logged, never stored, never included in events. |
| SEC-02 | RS256 keypair: private key only in the auth package's runtime (prod: **SSM Parameter Store SecureString**, injected at deploy time — Secrets Manager remains the Phase 10 target but bills $0.40 per secret per month, see [`deployment-aws.md`](deployment-aws.md) §2; local: git-ignored file). Public key distributed to services. Key rotation supported via `kid` header + a two-key verification window. |
| SEC-03 | All auth cookies: `httpOnly`, `Secure`, `SameSite=Lax`. Refresh cookie additionally path-scoped to `/api/v1/auth`. |
| SEC-04 | Email-verification and password-reset tokens: single-use, hashed at rest, expiring per FR-03/FR-07. |
| SEC-05 | OAuth2 flows use `state` + PKCE; redirect URIs are exact-match allowlisted. |
| SEC-06 | No secrets in Docker images or the repo; `.env.example` documents names only (blueprint §16). |
| SEC-07 | Security events (failed logins, lockout backoffs, refresh replays, admin actions) retained ≥ 90 days. |

---

## 6. Acceptance criteria (representative)

1. Login sets two cookies; neither token string is present in the response body or accessible to `document.cookie`.
2. After `logout`, an immediate retry with the captured access cookie returns 401 within one gateway round-trip (blacklist hit, no waiting for expiry).
3. After `logout-all` on device A, a request from device B fails on its next call (epoch check) without any per-token blacklist writes.
4. Replaying a rotated (already-used) refresh token kills **all** of that user's sessions and produces a WARN security event.
5. A `User` token calling any `/admin/*` endpoint gets 403 `AUTH_FORBIDDEN`; the attempt is logged.
6. A SystemAdmin suspending a user immediately invalidates that user's live tokens and blocks re-login.
7. A registration request with an injected `"role": "SystemAdmin"` field creates a plain `User` account (field ignored or rejected).
8. Unit tests in `libs/authmw` pass with Redis mocked; `docker compose up` + the Phase-2 smoke script exercises criteria 1–4 end-to-end.

---

## 7. Traceability

| Blueprint section | Covered by |
|---|---|
| §8 JWT + OAuth2 mechanics | FR-05, FR-10, FR-20, FR-21, SEC-02, SEC-05 |
| §8 Cookie + Redis blacklist subsection | FR-20, FR-30..33, NFR-02, NFR-03, SEC-03 |
| §10 Redis (blacklist/session/rate-limit) | FR-11, FR-20, FR-33, NFR-03, NFR-04 |
| §15 API conventions (envelope, versioning) | §3.7, FR-20 |
| §19 Security basics | §5 throughout |
| §20 Roadmap Phases 1–2 (auth build) | Priorities column in §3 |

*Population-level disclaimer conventions, LLM guardrails, and health-data
requirements are owned by other modules' SRS documents; this document
deliberately excludes them.*
