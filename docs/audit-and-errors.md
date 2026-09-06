# Audit logging, enumerations & error codes

**Status:** authoritative.
**Model file:** [`libs/auditmodel/model.yaml`](../libs/auditmodel/model.yaml) — the single
source of truth. Generated consumers are never hand-edited.
**Server message catalogs:** [`libs/i18n/locales/`](../libs/i18n/) — keyed by error code.
**Frontend catalogs:** `apps/web/src/i18n/locales/` — separate, and deliberately not copies.
**Generator:** `python3 scripts/gen_audit_model.py` (`--check` for CI).
**Companions:** [`shared-contract.md`](shared-contract.md) · [`backend.md`](backend.md) ·
[`i18n-guidelines.md`](i18n-guidelines.md)

---

## 1. One model, three outputs

Enumerations, audit events and error codes are declared **once**, in `model.yaml`, and
everything else is generated from it:

```
libs/auditmodel/model.yaml
        │
        ├──► libs/auditmodel/generated.go          Go constants, specs, validators
        └──► apps/web/src/api/generated/audit.ts   TS types, error-code → i18n key maps
```

They live together because they are the same fact viewed three ways. A login failure is an
enum value (`bad_password`), an audit event (`auth.login.failed`), and — when it reaches the
caller — an error code (`AUTH_INVALID_CREDENTIALS`). Declared separately, those three drift:
someone adds a failure mode to the code, forgets the audit event, and now a real failure is
invisible in the logs. Declared together, adding the enum value is the same edit that
registers the event.

This is the same pattern the rest of the repo already uses — `tokens.css` feeding Tailwind,
OpenAPI feeding the API client. One declaration, generated consumers, no hand-copying.

---

## 1a. Three language planes — do not conflate them

This is the rule that determines where every string lives.

| Plane | Language | Lives in | Rendered by |
|---|---|---|---|
| **Audit storage** | **Always en-US**, no exceptions | The audit record itself | The server, at write time |
| **API response** | The request's `Accept-Language` | `libs/i18n/locales/` | The server, per request |
| **Frontend display** | The browser's detected locale | `apps/web/src/i18n/locales/` | The SPA |

### Audit records are always en-US

Whatever locale the requester used, the record persisted to the audit log is English. This
is not laziness about internationalization — it is what makes the log usable at all.

An audit log written in each actor's language cannot be searched or aggregated. "Login
failed", "Đăng nhập thất bại" and whatever a third locale adds later would be three
different things to every query an investigator writes, and no amount of care at query time
recovers it, because the records are append-only: there is no migration that re-languages a
year of history.

So: **the request's `Accept-Language` affects the response only. It must never reach
storage.** The same discipline as enum values, for the same reason — stored text is an
identifier, not display copy.

(The structured fields are the real record; the rendered en-US `message` is a convenience
for a human reading a single row. Anything a query needs to filter on belongs in a field,
not in the message text.)

### The server renders its own messages

The error envelope's `message` is rendered server-side from `libs/i18n/locales/<locale>.json`
using the request's `Accept-Language`, falling back to en-US. The catalog is **keyed by the
error code itself** — there is no separate message-key indirection, so a key cannot dangle
or drift from the code it serves.

The server needs its own catalog because the SPA is not the only client: emails, webhooks,
and any future non-browser client all need a localized message, and none of them can read
the SPA's catalogs.

### The frontend keeps its own strings

The SPA renders **its own** string for a known error code by default, because it knows
context the server doesn't — which screen, which form, what the user was attempting. The
server's message must make sense with no such knowledge.

The two catalogs are therefore **not copies and are not content-synchronized.** Only their
coverage is checked, independently, by `scripts/check_i18n_parity.py`.

**The SPA displays the server's `message` only where a user story explicitly allows it** —
typically for codes a deployed frontend doesn't recognize yet (the server being ahead of the
client is a normal state, and that fallback is the main reason the server renders a message
at all). `apps/web/src/i18n/errorMessages.ts` provides `renderServerMessage()` as the single
sanctioned way to do it, so every such place is findable with one grep.

---

## 2. Enumerations

**Use an enum when you will want to count or group by the value.** "How many login failures
last week were rate-limits?" is answerable only if `reason` is a closed set. Free text
cannot be grouped, and by the time you need the answer the data is already unusable.

**Leave a field free-form when it's context for a human reading one record** — an IP, a
request ID, a device label. Constraining those buys nothing and just creates churn.

### Rules

- **An enum value is a permanent identifier, not display text.** `active`, not `Active` or
  `"In progress"`. Values are written to the database and to append-only audit records, so
  renaming one invalidates every stored row and every historical record that used it. There
  is no migration that fixes an audit log.
- **Display text comes from i18n keys**, never from the enum value. `RoutineStatus.active`
  renders through `t('routines.status.active')` — which is also how it gets a Vietnamese
  form.
- **Adding a value is usually safe; removing or renaming one is breaking.** Follow the
  expand/contract discipline in [`backend.md`](backend.md) §3.
- **Consumers must tolerate unknown values.** A frontend built before a new status existed
  must render it as unknown rather than crashing — otherwise every enum addition becomes a
  lockstep deploy.
- Values are `lower_snake_case`; enum names are `PascalCase`. The generator enforces both.

---

## 3. Audit events

An audit record answers **who did what to whom, and how did it end.** That is a different
question from what application logs answer ("what is the system doing right now"), and the
two must not be conflated: audit records are append-only, retained ≥90 days (SEC-07), and
must survive log-level changes and log rotation. A debug line does not.

### Shape

Each event declares: a dotted name, an actor type, an outcome, a severity, its fields, a
message template, and optionally the error code it surfaces.

```yaml
auth.login.failed:
  actor: anonymous
  outcome: failure
  severity: warn
  fields:
    reason: {enum: LoginFailureReason}     # aggregatable
    user_id: {type: id, optional: true}    # context
    source_ip: {type: string}              # context
  message: "Login failed ({reason}) from {source_ip}"
  error_code: AUTH_INVALID_CREDENTIALS
```

The message is a **key-value template**: every `{placeholder}` must name a declared field,
and the generator fails the build if one doesn't. That check exists because a template
referencing a field nobody supplies produces records with a literal `{user_id}` in them —
which nobody notices until they're trying to investigate an incident with the records they
have.

### Rules

- **Never log a secret.** No passwords, hashes, token values, or API keys. Log the *handle*
  — `jti`, not the token (NFR-07). The generator rejects field names that look like
  credentials; that check is a safety net, not permission to stop thinking.
- **Every event carries enough to answer "who and when"**: actor ID, target where one
  exists, request ID, timestamp.
- **Audit records are append-only.** No API path deletes or mutates one (FR-45). A
  correction is a new record.
- **Prefer one event with an enum over several near-identical events.** `admin.action.performed`
  with an `action` enum beats seven `admin.suspend` / `admin.unsuspend` events: same fields,
  and "show me everything this admin did" stays one query instead of a union of seven.
- Severity is about attention, not about how bad the user's day was. `auth.refresh.replay_detected`
  is `critical` because it means a token was probably stolen — even though the user just
  sees a login prompt.

---

## 4. Error codes: coarse on purpose

Error codes are the **external** surface. Audit events are the **internal** one. They are
deliberately different resolutions, and the mapping between them is many-to-one.

> **The most important rule in this document:** several audit events may map to one error
> code, and collapsing them is frequently a *security control* rather than a convenience.

`auth.login.failed` with `reason=unknown_email` and with `reason=bad_password` are two
distinct audit events — you need to tell them apart to investigate an attack. They map to
the **same** error code, `AUTH_INVALID_CREDENTIALS`, with identical status, wording and
timing, because SRS-AUTH-001 FR-08 and FR-14 require that a caller cannot distinguish them.
If the error code preserved the distinction, it would be an account-enumeration oracle:
an attacker submits a wrong password against a million addresses and learns which ones have
accounts.

The same applies to `auth.token.rejected`: four `TokenRejectReason` values, one
`AUTH_INVALID_TOKEN`. Telling a caller *which* check failed tells an attacker whether a
`jti` is known or whether an epoch moved.

### Rules

- **Codes are stable identifiers, not display text.** Clients switch on the code; humans
  read the message. Renaming a code is a breaking API change.
- **No message text lives in the model.** The server renders it from
  `libs/i18n/locales/<locale>.json`, keyed by the code. The generator rejects a `message` or
  `message_key` in the model, requires every code to have an en-US entry (a code with no
  entry means callers receive an empty message), requires every other locale to cover it,
  and flags catalog entries for codes that no longer exist.
- **Every code declares its HTTP status** in the model, so the same failure can't return 401
  from one endpoint and 403 from another.
- **The envelope is universal**: `{ "error": { "code": "...", "message": "..." } }`.
- **Not every audit event has an error code.** Successes don't surface errors, and some
  failures are internal-only. `error_code` is optional and should stay absent unless the
  caller genuinely learns about it.

---

## 5. Working with the model

**Adding a status value:** add it to the enum in `model.yaml`, run the generator, add the
i18n key for its display text to both catalogs. Check consumers handle it — an exhaustive
`switch` in Go or a mapped type in TS will now fail to compile, which is the point.

**Adding an audit event:** declare it with its fields and message template. Decide
deliberately whether it needs an `error_code` — most don't. Run the generator.

**Adding an error code:** declare code and `http_status` in the model; add the message to
**every** locale in `libs/i18n/locales/` (server side); add a mapping in
`apps/web/src/i18n/errorMessages.ts` plus the key it points at in both SPA catalogs; run
the generator. The SPA map is typed `Record<ErrorCode, string>`, so a new code breaks the
frontend type-check until someone decides what it should say — which is the point, since a
silently unhandled code renders nothing to the user.

Then check whether an existing code already covers the case — a proliferation of
near-identical codes makes client error handling worse, not better.

**Never** hand-edit `generated.go` or `audit.ts`. The next run reverts you silently, and
`--check` in CI will fail the build first.

### What the generator validates

Beyond emitting code, it fails on: unknown enum or error-code references, message
placeholders with no matching field, field names that look like secrets, enum values that
aren't `lower_snake_case`, error codes missing an HTTP status, message text left in the
model, error codes with no server-catalog entry, server locales missing a code the source
locale defines, catalog entries for codes that no longer exist, and generated files that are
out of date with the model.

Each of those is a mistake that is invisible in review and expensive at runtime, which is
the only good reason to spend a build step on it.

---

## 6. Open tension to resolve before Phase 2

`AUTH_ACCOUNT_SUSPENDED` conflicts with the enumeration protection this document is
otherwise built around.

SRS-AUTH-001 FR-42 says a suspended account's login attempts fail with
`AUTH_ACCOUNT_SUSPENDED`. But FR-08 and FR-14 require login responses to be indistinguishable
regardless of whether an account exists — and returning a distinct "suspended" code confirms
both that the address is registered *and* something about its state. An attacker who can
suspend-probe addresses has a working enumeration oracle.

Three ways out, none free:

1. **Return `AUTH_INVALID_CREDENTIALS` for suspended accounts too**, and tell the user out
   of band (email). Preserves enumeration protection; a suspended user gets no explanation
   in the UI, which will generate support requests.
2. **Keep the distinct code**, accepting that suspension is observable. Defensible if
   suspension is rare and manual — the oracle only works against accounts an admin already
   acted on.
3. **Return the distinct code only after correct credentials are supplied.** The prober must
   already know the password, which mostly closes the oracle. More logic in the login path,
   and it still leaks to a credential-stuffer with a valid pair.

This was surfaced by modelling the two surfaces together — it is invisible when audit events
and error codes are written in separate places, which is a fair argument for this whole
approach. **Decide before implementing auth; record the decision here and in the SRS.**
