# Executor brief: plat-audit-model

**Slice ID:** `plat-audit-model`
**Story / spec:** [AUTH-001](../../AUTH-001-auth-registration/story.md) ·
[AUTH-002](../../AUTH-002-auth-login/story.md) ·
[AUTH-003](../../AUTH-003-auth-session-refresh-logout/story.md) — you implement none of their
behavior; you declare the vocabulary all three are written in.
**Domain:** platform (backend + frontend generated consumers)
**Depends on:** none — may start immediately
**Load skill:** `multi-agent-execution` (`.claude/skills/multi-agent-execution/SKILL.md`)
**Read first:** [`docs/shared-contract.md`](../../../shared-contract.md) ·
[`docs/audit-and-errors.md`](../../../audit-and-errors.md) (all of it — it is the spec for this
slice) · [`../contract.md`](../contract.md) §7 and §8.3

## Task

Declare run 1's new enums, error codes and audit events in `libs/auditmodel/model.yaml`,
regenerate both consumers, and add the message-catalog entries the new error codes require on
the server side plus the code→key mappings on the SPA side.

Everything downstream of you — three backend slices, a middleware slice and two frontend slices
— imports what you generate. You are wave 1 and nothing waits on anything else, so finish
cleanly: a missing enum value here becomes a stop-and-escalate for someone else in wave 3.

**You add no behavior.** No handler, no middleware, no component. If you find yourself writing
an `if`, you are outside your slice.

## You own (exclusive write access)

- `libs/auditmodel/model.yaml`
- `libs/auditmodel/generated.go` (generator output — you run the generator, you do not hand-edit it)
- `apps/web/src/api/generated/audit.ts` (same)
- `libs/i18n/locales/en-US.json`
- `libs/i18n/locales/vi-VN.json`
- `apps/web/src/i18n/errorMessages.ts`
- `scripts/gen_audit_model.py` — only if a genuine generator gap blocks you; say so in your report

## Read-only context

- `docs/audit-and-errors.md` — §2 (enum rules), §3 (event shape), §4 (why codes are coarse),
  §5 (the working procedure), §6 (compact codes)
- `docs/stories/auth-epic/contract.md` §7 (what to add), §8.3 (who owns which catalog)
- `libs/i18n/i18n.go` — how the server renders a message from a code
- `libs/httpkit/envelope.go` — how a code becomes a response
- `apps/web/src/i18n/locales/*.json` — read to see the key shape; **another slice owns them**

## Do not touch

- `apps/web/src/i18n/locales/{en-US,vi-VN}.json` — owned by `fe-auth-flows` this run. You will
  reference two keys that do not exist there yet (see below). That is expected and correct.
- `libs/i18n/i18n.go`, `libs/httpkit/**` — not part of this run
- Anything under `services/`, `apps/web/src/api/auth*`, `apps/web/src/features/`
- Any document under `docs/` (the contract is frozen — see the stop condition)

## Contract you implement against

From [`../contract.md`](../contract.md) §7. Additive only: no existing key is renamed, no enum
value removed, no compact code retired.

**Enums**

```yaml
AccountStatus:
  description: Whether an account may authenticate (FR-42). Run 1 reads it; run 2 writes it.
  values:
    active: Normal. May log in.
    suspended: Suspended by a SystemAdmin. Epoch advanced; login refused.

LinkKind:
  description: What a single-use emailed link authorizes. One table, one lifecycle (SEC-04).
  values:
    email_verification: Confirms control of the address (FR-03, 24h).
    password_reset: Authorizes setting a new password (FR-07, 1h).
    oauth_link: Authorizes linking an OAuth identity to an existing local account (FR-06, 1h).

LinkRejectReason:
  description: >
    Why a link token was refused. INTERNAL — all three map to one error code, because
    telling a caller which one applies tells them whether the token was ever real.
  values:
    unknown: No such token.
    expired: Past its expiry.
    already_consumed: Single-use token presented a second time.
```

**Error codes**

```yaml
AUTH_EMAIL_UNVERIFIED:
  http_status: 403
  description: >
    An account that has not verified its email attempted a state-changing request
    (FR-03). Enforced once, in gateway middleware — see auth-epic/contract.md D4.

AUTH_LINK_INVALID:
  http_status: 400
  description: >
    A verification, password-reset or OAuth-link token that is unknown, expired or
    already consumed. Deliberately one code for all three LinkRejectReason values,
    for the same reason AUTH_INVALID_TOKEN is one code for four reject reasons: a
    distinguishable response tells an attacker whether a token ever existed.
```

**Events**

| Event | actor | outcome | severity | fields | error_code |
|---|---|---|---|---|---|
| `auth.login.suspended` | anonymous | denied | warn | `user_id` (id), `source_ip` (string) | `AUTH_ACCOUNT_SUSPENDED` |
| `auth.email.verified` | user | success | info | `user_id` (id) | — |
| `auth.link.rejected` | anonymous | failure | warn | `kind` (LinkKind), `reason` (LinkRejectReason), `user_id` (id, optional), `source_ip` (string) | `AUTH_LINK_INVALID` |
| `auth.password_reset.requested` | anonymous | success | info | `user_id` (id, optional — absent for an unknown address), `source_ip` (string) | — |
| `auth.oauth.link_required` | anonymous | denied | warn | `user_id` (id), `provider` (LinkedProvider), `source_ip` (string) | — |
| `auth.write_blocked_unverified` | user | denied | info | `user_id` (id), `path` (string), `request_id` (id) | `AUTH_EMAIL_UNVERIFIED` |

Message templates are yours to write, in the style of the existing ones, and every
`{placeholder}` must name a declared field — the generator fails the build otherwise. They are
**en-US storage renderings** and are never localized (`audit-and-errors.md` §1a).

**One existing entry changes, in one way only:** add a `note:` to
`LoginFailureReason.account_suspended` recording that contract decision D2 retired its use —
login now emits `auth.login.suspended` instead. Do **not** remove the value: its compact code is
permanent (`audit-and-errors.md` §6).

**Catalogs.** `libs/i18n/locales/{en-US,vi-VN}.json` are keyed by error code and need entries
for `AUTH_EMAIL_UNVERIFIED` and `AUTH_LINK_INVALID` in **both** locales — the generator fails
if a locale is missing a code. Write real Vietnamese, not English in a `vi-VN` file
(`docs/i18n-guidelines.md`); these are the strings a non-browser client shows a user.

`apps/web/src/i18n/errorMessages.ts` is `Record<ErrorCode, string>`, so it stops type-checking
the moment you regenerate — that break is the feature. Map the two new codes to
`errors.auth_email_unverified` and `errors.auth_link_invalid`. **Those keys will not exist in
the SPA catalogs when you finish**; `fe-auth-flows` adds them (contract §8.3). Do not add them
yourself and do not "fix" the reference.

## Acceptance-criteria scenarios this slice covers with automated tests

None. This slice declares vocabulary; no acceptance-criteria scenario is observable in it. Its
correctness is checked by the generator's own validations and by the consumers that stop
compiling if it is wrong — which is exactly why it is wave 1 and alone in owning these files.

## Done when

- [ ] `model.yaml` contains all three enums, both error codes and all six events above, and the
      `note:` on `LoginFailureReason.account_suspended`
- [ ] Both generated files are regenerated and committed, and no compact code assigned before
      this run has changed (diff the maps — an existing readable name whose compact code moved
      is a bug that invalidates stored rows)
- [ ] Both server locales carry entries for both new codes
- [ ] `errorMessages.ts` maps both new codes
- [ ] Generator is clean: **`py scripts/gen_audit_model.py --check`**
      *(the interpreter on this machine is `py`, Python 3.13.3 — `python3` and `python` are
      Microsoft Store stubs that will fail confusingly. This check is runnable, so it is
      required; there is no acceptable reason to skip it. See
      [AMD-001](../amendments/AMD-001-frontend-test-tooling.md).)*
- [ ] Go builds: `go build ./... && go vet ./...`
- [ ] Go tests pass: `go test ./...`
- [ ] `cd apps/web && npx tsc --noEmit` — expect it to **fail only** on missing SPA catalog
      keys if your local i18n typing checks catalogs; report the exact failure rather than
      adding keys you do not own
- [ ] Nothing modified outside the allowlist above (check your own diff)

## Stop condition — read this before starting

If you find the contract wrong, ambiguous, or insufficient for this task:

> **Stop work on the affected surface. Write an amendment request using
> `.claude/skills/multi-agent-execution/assets/amendment-request-template.md`. Do not proceed on
> that surface.**

Do not fix the contract yourself, and do not implement around it. Other slices are being built
against the current version right now; a local fix produces two implementations that each
believe they conformed, and nothing detects the mismatch until runtime.

Continue any part of your slice that doesn't depend on the disputed shape.

Two things in particular that are **not** yours to decide, and are amendment requests if you
think they are wrong: adding a *third* error code to split `AUTH_LINK_INVALID`, and changing
which event carries which `error_code`.

## Report back

Write the report to `docs/stories/auth-epic/reports/plat-audit-model.md`.

1. **Changed:** files touched, and what each change does
2. **Verified:** which commands you ran and their results — not what you expect to pass
3. **Could not do:** anything blocked, and why
4. **Noticed:** problems outside your scope. Report them, don't fix them.
