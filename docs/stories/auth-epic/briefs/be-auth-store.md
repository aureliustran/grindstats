# Executor brief: be-auth-store

**Slice ID:** `be-auth-store`
**Story / spec:** [AUTH-001](../../AUTH-001-auth-registration/story.md) ·
[AUTH-002](../../AUTH-002-auth-login/story.md) · SRS-AUTH-001 §3.1, SEC-04
**Domain:** backend
**Depends on:** `plat-audit-model`, `db-migrations` — both must have reported done
**Load skill:** `multi-agent-execution` (`.claude/skills/multi-agent-execution/SKILL.md`)
**Read first:** [`docs/backend.md`](../../../backend.md) §2, §3, §7 ·
[`docs/shared-contract.md`](../../../shared-contract.md) ·
[`../contract.md`](../contract.md) §4, §5

## Task

Write the `authdomain` package — the types, interfaces and sentinel errors the whole auth domain
is expressed in — and the pgx repositories behind them.

**This slice exists so wave 3 can be three parallel slices.** `be-auth-credentials`,
`be-auth-session` and `be-gateway-authz` all import `authdomain` and none of them import each
other; their concrete implementations meet only at wave 4's composition root. Every interface
you write here is a seam three other agents are about to build against sight-unseen, so
transcribe contract §5 faithfully rather than improving it. If a signature genuinely cannot work,
that is an amendment request — and a fast one, because wave 3 has not started.

Wave 3 writes **no SQL**. Everything that touches `auth.*` is yours.

## You own (exclusive write access)

- `services/monolith/internal/auth/authdomain/**`
- `services/monolith/internal/auth/store/**`

## Read-only context

- `docs/stories/auth-epic/contract.md` §5 (the interfaces, verbatim) and §4 (the tables behind
  them)
- `infra/db/migrations/**` — the schema as actually shipped by `db-migrations`. If it disagrees
  with contract §4, **stop and escalate**; do not code against whichever you prefer.
- `libs/auditmodel/generated.go` — `Role`, `AccountStatus`, `LinkKind`, `LinkedProvider`, and the
  readable ↔ compact maps
- `docs/audit-and-errors.md` §6 — enum columns store compact codes; translation happens at the
  boundary where a row is read, which is *this* package and nowhere else
- `services/monolith/internal/platform/db/db.go` — the pool you receive
- `docs/backend.md` §2 — the ownership rule you are the enforcement point for

## Do not touch

- `infra/db/migrations/**` — `db-migrations` owns every DDL statement. If you need a column that
  isn't there, escalate; do not add a migration.
- `services/monolith/internal/auth/{credentials,oauth,hibp,mailer,session,integration}/**` —
  wave 3 and 4 slices
- `services/monolith/internal/gateway/**`
- `libs/**` — all four library slices are done and frozen for consumers
- `go.mod` / `go.sum`

## Contract you implement against

[`../contract.md`](../contract.md) §5 gives `Account`, `AccountRepo`, `OAuthIdentityRepo`,
`LinkTokenRepo`, `SessionIssuer`, `AccountProvisioner` and the four sentinel errors verbatim.
Transcribe all of them into `authdomain`, including the two interfaces this slice does **not**
implement (`SessionIssuer` is `be-auth-session`'s, `AccountProvisioner` is
`be-auth-credentials`'s) — they live here so neither of those slices has to import the other.

Implement in `store`: `AccountRepo`, `OAuthIdentityRepo`, `LinkTokenRepo`.

The details that carry real requirements, not just plumbing:

- **`ByEmail` takes an already-lowercased address and returns `(nil, nil)` when absent**, not an
  error. Callers must be able to treat "no such account" as an ordinary branch, because
  FR-08 requires that branch to be indistinguishable from the found-but-wrong-password branch —
  and a code path that returns an error is a code path that will eventually log or respond
  differently.
- **`Create` returns `ErrEmailTaken` on a unique violation**, detected from the constraint, not
  from a pre-check `SELECT`. A check-then-insert has a race, and the race's loser is a 500 in the
  middle of the one response that must never vary (FR-08).
- **Enum columns are `char(8)` compact codes** (`audit-and-errors.md` §6). Translate readable
  ↔ compact inside this package. Nothing outside it ever sees a compact code — `Account.Role` is
  `auditmodel.Role`, never `"ROLE2001"`.
- **`LinkTokenRepo.Issue` returns the raw token and stores only its SHA-256** (SEC-04). The raw
  value exists in memory long enough to be emailed and is never persisted or logged.
- **`LinkTokenRepo.Consume` validates and marks consumed atomically** — one statement with a
  `WHERE consumed_at IS NULL AND expires_at > now()` and a `RETURNING`, not a read followed by a
  write. Two concurrent uses of a single-use token must produce exactly one success, which is
  AUTH-001's "verification token is single-use" under concurrency.
- **`Consume` returns `ErrLinkInvalid` for unknown, expired and already-consumed alike.** It may
  return the `LinkRejectReason` alongside for the caller's *audit* record, but the error value
  must not let a caller accidentally branch into three different responses. One error, three
  reasons, one code (`AUTH_LINK_INVALID`).

## Acceptance-criteria scenarios this slice covers with automated tests

No scenario's *Then* is observable at the repository layer — every one of them is observed
through a handler, which is why the scenarios sit with wave 3. Your required tests:

| What | Kind | Location |
|---|---|---|
| `Create` then `ByEmail` round-trips an account, with role and status surviving the compact-code translation | integration, env-gated `GRINDSTATS_TEST_DB` | `services/monolith/internal/auth/store/accounts_test.go` |
| a second `Create` with a case-variant address returns `ErrEmailTaken` (FR-01) | integration, same gate | ” |
| `Issue` stores only a hash — the raw token appears nowhere in the row | integration, same gate | `.../store/link_tokens_test.go` |
| `Consume` succeeds once and returns `ErrLinkInvalid` on the second attempt, including when both run concurrently | integration, same gate | ” |
| `Consume` returns `ErrLinkInvalid` for an expired token, and the row is left unconsumed | integration, same gate | ” |
| `ByEmail` returns `(nil, nil)`, not an error, for an unknown address | integration, same gate | `.../store/accounts_test.go` |

Gate them behind `GRINDSTATS_TEST_DB` so `go test ./...` stays green without a database
(`backend.md` §7). If no database is available to you, say exactly that in your report — do not
convert them into tests that assert nothing.

## Done when

- [ ] `authdomain` contains every type, interface and error in contract §5, transcribed, with
      doc comments that say *why* where the contract gives a reason
- [ ] `store` implements the three repositories, and no other package in the repo writes SQL
      against `auth.*`
- [ ] Every test above passes with a database, and the suite is green without one
- [ ] No compact code escapes the `store` package — grep your own diff for `char(8)`-shaped
      literals outside it
- [ ] Builds: `go build ./... && go vet ./...`; suite green: `go test ./...`
- [ ] Nothing modified outside the allowlist above (check your own diff)

## Stop condition — read this before starting

If you find the contract wrong, ambiguous, or insufficient for this task:

> **Stop work on the affected surface. Write an amendment request using
> `.claude/skills/multi-agent-execution/assets/amendment-request-template.md`. Do not proceed on
> that surface.**

Do not fix the contract yourself, and do not implement around it. Other slices are being built
against the current version right now; a local fix produces two implementations that each
believe they conformed, and nothing detects the mismatch until runtime.

You are the highest-leverage place for this in the whole run: wave 3 has not started, and three
briefs quote your interfaces. An amendment raised now costs a paragraph. The same problem found
in wave 3 costs three re-dispatches.

## Report back

Write the report to `docs/stories/auth-epic/reports/be-auth-store.md`.

1. **Changed:** files touched, and what each change does
2. **Verified:** commands run and their actual results, including whether a database was
   available for the gated tests
3. **Could not do:** anything blocked, and why
4. **Noticed:** problems outside your scope — in particular any disagreement you found between
   contract §4 and the migrations as shipped. Report it, don't fix it.
