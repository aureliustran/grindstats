---
name: audit-log-and-error-codes
description: Define or extend audit log events, enumerations (status and other closed-set attribute values), and the error codes derived from them — all declared in one model file that generates Go and TypeScript. Use whenever adding or changing a status/type/reason value, an audit or security event, an admin action record, or an API error code; when asked to "add a status", "log this action", "what error code should this return", "add an enum", or to make failures traceable; and when reviewing whether something should be logged, aggregated, or surfaced to the caller. Also use when error codes look inconsistent across endpoints or an audit record turns out to be missing the field an investigation needed.
---

# Audit logging, enumerations & error codes

These three are one system, declared in one file. An enum value, the audit event that
records it, and the error code the caller sees are the same fact at three resolutions —
declare them apart and they drift, usually discovered during an incident when the record
you need turns out not to exist.

**Model file:** `libs/auditmodel/model.yaml`
**Generator:** `python3 scripts/gen_audit_model.py` (`--check` validates without writing)
**Full rules:** `docs/audit-and-errors.md`

Never hand-edit `libs/auditmodel/generated.go` or `apps/web/src/api/generated/audit.ts`.
The next generation run reverts you, and `--check` fails the build before that.

---

## The decision that governs everything else

Work out which of these three you're actually adding, because the rules differ sharply:

| You're adding | Because | Goes in |
|---|---|---|
| An **enum value** | You'll want to *count or group by* it | `enums:` |
| An **audit event** | Someone will need to know *this happened*, later | `events:` |
| An **error code** | The *caller* must be told and must branch on it | `error_codes:` |

Most additions are one or two of these, not all three. A new routine status is an enum and
probably nothing else. A failed permission check is an audit event *and* an error code. A
successful login is an audit event with no error code at all.

The question that resolves it: **who needs to know?** If the answer is "a dashboard, later"
it's an enum-constrained field on an event. If it's "an investigator, later" it's an event.
If it's "the client, right now" it's an error code.

---

## Enums

**Constrain a field when you'll aggregate over it.** "How many login failures were
rate-limits last week?" is answerable only if `reason` is a closed set. Leave free-form what
a human reads once in a single record — an IP, a request ID, a device label. Constraining
those buys nothing and creates churn.

When adding:

- Value is `lower_snake_case` and **permanent**. It's written to the database and to
  append-only audit records, so a rename invalidates every stored row and every historical
  record. There is no migration that fixes an audit log.
- The value is **never display text.** `active`, not `"In progress"`. Display comes from an
  i18n key, which is also how it gets a Vietnamese form.
- Add the display key to **both** locale catalogs in the same change, or the UI renders a raw
  identifier at people.
- Adding a value is usually safe; removing or renaming is breaking — expand/contract
  (`docs/backend.md` §3).
- After generating, exhaustive switches in Go and mapped types in TS will stop compiling.
  That's the feature; fix them rather than adding a default that silently swallows the new
  case.

## Audit events

An audit record answers **who did what to whom, and how did it end** — a different question
from what application logs answer. Audit records are append-only and retained ≥90 days;
a debug line is neither, so don't use one where the other is meant.

```yaml
auth.login.failed:
  description: Why a login attempt failed.
  actor: anonymous              # ActorType
  outcome: failure              # Outcome
  severity: warn                # Severity — attention warranted, not user impact
  fields:
    reason: {enum: LoginFailureReason}     # aggregatable
    user_id: {type: id, optional: true}    # context
    source_ip: {type: string}              # context
  message: "Login failed ({reason}) from {source_ip}"
  error_code: AUTH_INVALID_CREDENTIALS     # optional — most events have none
```

When adding:

- **Never a secret field.** No password, hash, token value, or API key. Log the handle —
  `jti`, never the token. The generator rejects credential-looking names, but that's a net,
  not a substitute for thinking about what you're persisting for 90 days.
- Every `{placeholder}` in `message` must name a declared field; the generator enforces it.
  A template referencing a missing field writes literal `{user_id}` into records nobody
  notices until they're mid-investigation.
- **Prefer one event with an enum over several near-identical events.**
  `admin.action.performed` with an `action` enum beats seven separate admin events: same
  fields, and "everything this admin did" stays one query.
- Severity is about operational attention. `auth.refresh.replay_detected` is `critical`
  because it means a token was probably stolen — even though the user just sees a login
  prompt.
- Ask what an investigator would need six months from now. The field that's missing is
  almost always the one that would have identified *which* thing was affected.

## Error codes

Deliberately coarser than events, because they're the external surface.

> **Several audit events may map to one error code, and collapsing them is often a security
> control rather than a convenience.**

`reason=unknown_email` and `reason=bad_password` are distinct events — you need them apart
to investigate. They share one code, `AUTH_INVALID_CREDENTIALS`, with identical status,
wording and timing, because a distinguishable response is an account-enumeration oracle
(SRS-AUTH-001 FR-08, FR-14). Same for the four `TokenRejectReason` values behind
`AUTH_INVALID_TOKEN`: telling a caller which check failed tells an attacker whether a `jti`
is known.

**So before adding a code, ask what it lets an unauthenticated caller learn.** If the honest
answer includes "whether this account exists" or "whether this token was known", collapse it
into an existing code instead.

When adding:

- Check an existing code doesn't already cover it. A proliferation of near-identical codes
  makes client handling worse, not better.
- Declare `http_status` in the model so the same failure can't be 401 here and 403 there.
- `message_key` is an **i18n key, never literal text** — and the key must exist in both
  catalogs. The generator verifies it against `en-US.json`, because a dangling key renders
  the raw key string to a user and nobody notices until that error path is hit.
- Codes are stable identifiers; renaming one is a breaking API change.

---

## Workflow

1. **Read `libs/auditmodel/model.yaml`** — the answer is often "an enum value already exists
   for this" or "this code already covers it."
2. **Decide which of the three you're adding** (table above). Don't add all three reflexively.
3. **Edit the model file only.** Keep descriptions substantive: they become the doc comments
   in generated Go, and they're what the next person reads instead of guessing.
4. **Add i18n keys** to both catalogs for anything user-visible — error messages, enum
   display text.
5. **Generate:** `python3 scripts/gen_audit_model.py`
6. **Fix what stopped compiling.** Exhaustive switches breaking on a new enum value is the
   system working.
7. **Verify:** `python3 scripts/gen_audit_model.py --check` and
   `python3 scripts/check_i18n_parity.py`
8. **Commit the model, the generated files, and the catalogs together.** Splitting them
   leaves the repo in a state where `--check` fails for whoever pulls next.

## What the generator catches

Unknown enum or error-code references · message placeholders with no matching field · field
names that look like secrets · enum values not in `lower_snake_case` · error codes missing
an HTTP status or message key · `message_key`s absent from the catalog · generated files
out of date with the model.

Each is invisible in review and expensive at runtime — the only good reason to spend a build
step on a check.

## When NOT to reach for this

- **Debug or trace logging.** That's application logging: ephemeral, verbose, not retained.
  Audit records are a permanent claim about what happened.
- **A one-off string in a UI.** That's an i18n key, nothing more.
- **A value that is genuinely open-ended** — a user-supplied label, an exercise name. Forcing
  a closed set on it means a model edit every time reality adds a case.
