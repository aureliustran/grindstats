# Test cases: API gateway skeleton with health and readiness probes

**Story:** [`GATE-001`](story.md) · **Criteria:** [`acceptance-criteria.md`](acceptance-criteria.md)

Handler tests run through `net/http/httptest` against the real Gin router with fake
dependency checkers (`docs/backend.md` §7). TC-11 is the only case requiring live
containers and is gated behind `GRINDSTATS_TEST_DB`.

| ID | Title | Type | Linked scenario | Preconditions | Steps | Test data | Expected result | Priority |
|----|-------|------|-----------------|---------------|-------|-----------|-----------------|----------|
| TC-01 | Liveness answers with every dependency down | Happy path | liveness answers while every dependency is unreachable | Router built with checkers that fail the test if invoked | 1. `GET /healthz` | no headers | 200; body reports alive; no checker was invoked | P1 |
| TC-02 | Readiness ready when all dependencies healthy | Happy path | readiness reports ready when all dependencies are healthy | Both fakes return nil | 1. `GET /readyz` | — | 200; status `ready` | P1 |
| TC-03 | Redis down keeps the instance in rotation | Edge | readiness stays ready when only the optional dependency is unreachable | Postgres fake nil; Redis fake returns error | 1. `GET /readyz`<br>2. Read captured log | Redis error `dial tcp: refused` | 200; status `degraded`; log warning names Redis | P1 |
| TC-04 | Postgres down takes the instance out of rotation | Negative | readiness reports not-ready when the required dependency is unreachable | Postgres fake returns error; Redis fake nil | 1. `GET /readyz` | — | 503; envelope shape `{error:{code,message}}`; code `SERVICE_UNAVAILABLE` | P1 |
| TC-05 | Both dependencies down reports not-ready | Negative | readiness reports not-ready when the required dependency is unreachable | Both fakes return errors | 1. `GET /readyz` | — | 503; code `SERVICE_UNAVAILABLE`; required failure wins over degraded | P2 |
| TC-06 | Failure message follows `Accept-Language` | Edge | readiness failure renders its message in the request locale | Postgres fake returns error | 1. `GET /readyz` with `Accept-Language: en-US`<br>2. Same with `vi-VN`<br>3. Same with `de-DE` | three locales | Each `message` equals its catalog string; `de-DE` falls back to `en-US`; `code` identical across all three | P1 |
| TC-07 | Failure body discloses no dependency detail | Negative | readiness failure does not disclose which dependency failed | Postgres fake returns an error containing host, port and driver text | 1. `GET /readyz`<br>2. Read captured log | error text `pgx: dial 10.0.1.7:5432 failed` | Body contains none of the host, port, driver name or raw error; log contains all of it | P1 |
| TC-08 | Hung dependency is abandoned at its timeout | Edge | a hung dependency does not hang the probe | Postgres fake blocks until its context is cancelled | 1. `GET /readyz`<br>2. Measure elapsed | check timeout 2s | Returns shortly after the timeout, not on the fake's own schedule; 503; no goroutine left blocked | P1 |
| TC-09 | Server boots with Postgres unavailable | Edge | the server starts when a dependency is unavailable at boot | Postgres unreachable at construction | 1. Build the server<br>2. `GET /healthz`<br>3. `GET /readyz`<br>4. Make Postgres reachable<br>5. `GET /readyz` | — | Construction returns no fatal error; step 2 → 200; step 3 → 503; step 5 → 200 with no restart | P1 |
| TC-10 | Panic becomes an envelope, not a dropped connection | Negative | a panic in a handler returns the error envelope | Test-only route registered that panics | 1. `GET` the panicking route<br>2. `GET /healthz` | panic value `boom` | 500; code `INTERNAL_ERROR`; stack in log; step 2 still 200 | P1 |
| TC-11 | Readiness against live containers | Happy path | readiness reports ready when all dependencies are healthy | `docker compose up -d`; `GRINDSTATS_TEST_DB=1` | 1. Connect real Postgres and Redis<br>2. `GET /readyz` | compose defaults | 200; status `ready` | P2 |
| TC-12 | Request ID echoed when supplied | Happy path | a caller-supplied request ID is echoed, and generated when absent | Router built | 1. `GET /healthz` with `X-Request-ID` | `test-req-42` | Response header equals `test-req-42`; log line carries it | P2 |
| TC-13 | Request ID generated when absent | Edge | a caller-supplied request ID is echoed, and generated when absent | Router built | 1. `GET /healthz` with no `X-Request-ID`<br>2. Repeat | — | Both responses carry a non-empty ID; the two differ | P2 |
| TC-14 | Probes need no credentials and no version prefix | Happy path | probes are unauthenticated and unversioned | Router built | 1. `GET /healthz`<br>2. `GET /readyz`<br>3. `GET /api/v1/healthz` | no auth header | Steps 1–2 served; step 3 → 404 | P2 |
| TC-15 | Chain order registered, auth stages absent | Regression | the chain leaves the auth stages unbuilt but positioned | Router built | 1. Inspect the registered middleware chain | — | Recovery, request ID, logging, CORS, locale present in that order; no auth, CSRF, rate-limit or role stage registered | P3 |
| TC-16 | Every returnable code is declared and generated | Regression | (contract conformance, `shared-contract.md` §5) | Generator available | 1. `python3 scripts/gen_audit_model.py --check`<br>2. `python3 scripts/check_i18n_parity.py`<br>3. Grep gateway for literal code strings | — | Generator reports no diff; catalogs in parity across both locales; no hand-written code literal in `internal/gateway/` | P1 |

## Coverage notes

Every scenario in `acceptance-criteria.md` maps to at least one case above; TC-05 and TC-07
add data variations to scenarios that carry more than one meaningful failure shape.

TC-08 asserts against the timeout rather than the fake's delay deliberately — asserting only
that the request eventually returns would pass even with no timeout implemented at all.

TC-06 asserts the `code` is identical across locales in the same test that asserts the
messages differ. Checking them in separate tests would let a regression that localizes the
code itself pass both.
