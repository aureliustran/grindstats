# Audit logging, enumerations & error codes

**Status:** authoritative.
**Model file:** [`libs/auditmodel/model.yaml`](../libs/auditmodel/model.yaml) — the single
source of truth. Generated consumers are never hand-edited.
**Server message catalogs:** [`libs/i18n/locales/`](../libs/i18n/) — keyed by error code.
**Frontend catalogs:** `apps/web/src/i18n/locales/` — separate, and deliberately not copies.
**Generator:** `python3 scripts/gen_audit_model.py` (`--check` for CI).
**DB storage format:** compact 8-char codes, not the readable strings — see §6.
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
  `"In progress"`. Values are written to the database and to append-only audit records — as a
  generated compact code, not the literal string; see §6 — so renaming one invalidates every
  stored row and every historical record that used it. There is no migration that fixes an
  audit log.
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

## 6. Compact DB codes

Error codes, audit event names, and enum values are readable strings
(`AUTH_INVALID_CREDENTIALS`, `auth.login.failed`, `RoutineStatus.active`) everywhere they're
*used* — in Go, in TypeScript, in the API response, in a human reading a query result. But the
tables that store them (the audit log especially) are append-only and high-volume, and repeating
a long string in every indexed row and every foreign-keyed reference is waste that compounds. So
what's actually persisted in the DB for an error code, an audit event name, or an enum value is a
compact 8-character code, and the readable string is a generated, in-process lookup away — never
a second source of truth.

**This is a storage-layer optimization only.** It changes nothing about section 4's rule that
error codes are the stable external identifier, section 3's rule that audit records answer
"who did what to whom," or section 2's rule that an enum value is a permanent identifier.
Clients still switch on `AUTH_INVALID_CREDENTIALS`; application code still compares against
`RoutineStatus.active`; nothing compact ever reaches an API response, a rendered message, or a
line of Go/TypeScript business logic. The translation happens once, at the boundary where a row
is read out of the DB, before it's handed to any handler, response serializer, audit-message
renderer, or in-memory domain object.

### Format

```
AUTH0001
└──┘│└─┘
 │  │ └── 3-digit sequence number, zero-padded, unique per (prefix, kind)
 │  └──── kind digit: 0 = error code, 1 = audit event, 2 = enum value
 └─────── 4-letter prefix, derived from the readable name
```

8 characters, fixed width. `AUTH_INVALID_CREDENTIALS` might compact to `AUTH0001`;
`auth.login.failed` might compact to `AUTH1003`; `RoutineStatus.active` might compact to
`ROUT2001`. All three can encode different things (`0`/`1`/`2`) even while sharing a prefix — the
prefix says "this is roughly in the auth/routine area to a human skimming a DB row," the (kind,
sequence) pair is what actually guarantees uniqueness.

- **Prefix**: uppercase, first 4 letters of the name's first segment.
  - Error code: the part before the first `_` — `VALIDATION_FAILED` → `VALI`.
  - Audit event: the part before the first `.` — `admin.action.performed` → `ADMI`.
  - Enum value: the **enum name**, not the value — `RoutineStatus.active` → `ROUT`,
    `LoginFailureReason.bad_password` → `LOGI`. The enum name is what's derived from, because
    enum values themselves are short and repeat across enums (`active` alone appears in both
    `RoutineStatus` and, differently spelled per-enum, other status enums) — deriving from the
    value would make the prefix nearly useless as a skimming aid and wouldn't help uniqueness
    anyway, since that's the sequence number's job regardless.

  Two different readable names — of the same or different kinds — can land on the same prefix
  (`AUTH_INVALID_CREDENTIALS` and a hypothetical `AUTHOR_DENIED` both start `AUTH`; so could an
  `AdminAction` enum value under kind `2`) — that's fine, the (kind, sequence) pair still makes
  every full code unique, and nobody is expected to reconstruct the readable name from the
  prefix by eye.
- **Kind digit**: `0` for every entry under `error_codes:`, `1` for every entry under `events:`,
  `2` for every value under any enum in `enums:`. Reserved so a compact code alone tells you
  which table/lookup to translate it through, without touching the DB row it came from.
- **Sequence**: assigned by the generator, not hand-written. Per distinct (prefix, kind) pair,
  the next new entry gets the next unused 3-digit number in declaration order. For enum values,
  this means the sequence is scoped to (enum-name-derived prefix, `2`) — so all values of one
  enum share a prefix and get consecutive sequence numbers as they're added, while a same-named
  value in a *different* enum (`active` in `RoutineStatus` vs. `active` in `OccurrenceStatus`,
  different prefixes `ROUT`/`OCCU`) never collides in the first place. Never reused, even if the
  entry it belonged to is later removed — a removed value's compact code is retired, not
  recycled, so an old row referencing it doesn't silently start meaning something new.

### Assignment and generation

`model.yaml` keeps the readable name (`AUTH_INVALID_CREDENTIALS`, `auth.login.failed`,
`RoutineStatus.active`) as the entry's key — engineers never write a compact code by hand. The
generator assigns one the first time an entry is generated and then treats it as permanent:
`libs/auditmodel/generated.go` and `apps/web/src/api/generated/audit.ts` each carry a
bidirectional map (readable ↔ compact) as part of their generated output, alongside the
constants section 1 already describes. Regenerating after adding a new entry only ever *appends*
new mappings — an existing readable name's compact code never changes, because that would
invalidate every row that already carries it, in the audit log or in any domain table storing
the enum's compact form.

**The DB never needs its own lookup table.** Translation happens through the generated in-process
map, in the backend service that read the row — the same discipline as `renderServerMessage()`
being the one sanctioned place a server message reaches the client. A raw SQL query against the
audit table, or against any domain table with an enum-typed column, sees only compact codes;
anyone needing readable output reads it through the service layer, not by joining against the
database directly.

### Rules

- **Compact codes are a storage detail, not a wire format.** They never appear in an API
  response, an audit `message` rendering, application business logic (`if status == "active"`
  compares against the readable constant, translated on read — never against `ROUT2001`), or
  anything a client or a human reads.
- **The generator assigns and owns the mapping. Never hand-edit a compact code**, for the same
  reason the rest of `generated.go` and `audit.ts` are never hand-edited.
- **A compact code is permanent once assigned** — it is written to append-only audit rows and to
  every domain table storing that enum's compact form, and there is no migration that re-codes
  history. This sits alongside, and does not relax, section 2's existing rule that an enum
  *value* itself is a permanent identifier — renaming the readable value is still breaking,
  independent of what its compact code happens to be.
- **Removing an error code, event, or enum value retires its compact code.** The generator must
  not reassign a retired (prefix, kind, sequence) to a new entry, even one with an identical
  prefix or, for enums, the same value spelling reintroduced later.
- **Adding a value to an existing enum only ever appends a new compact code** to that enum's
  (prefix, `2`) sequence — it never touches the compact codes already assigned to that enum's
  other values, so this stays compatible with section 2's "adding a value is usually safe"
  guidance.

---

## 7. Open tension to resolve before Phase 2

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
