# Acceptance criteria: Audit Log & Error Code Model

**Code:** `PLAT-001`

## Acceptance criteria (summary)

- [ ] Adding a new enum value, audit event, or error code requires editing only `model.yaml`;
      running the generator produces both the Go and TypeScript consumers from that one edit.
- [ ] The generator fails the build (non-zero exit, no output files written/updated) when a
      message template references a field the event doesn't declare.
- [ ] The generator fails the build when an audit event or error code reference doesn't resolve
      to something declared in the model.
- [ ] The generator fails the build when a field name looks like it carries a secret
      (password, token, hash, api_key, etc.).
- [ ] The generator fails the build when an enum value isn't `lower_snake_case` or an enum name
      isn't `PascalCase`.
- [ ] The generator fails the build when an error code has no `http_status`, or carries message
      text directly in the model.
- [ ] The generator fails the build when an error code has no en-US server-catalog entry, or
      when a non-source locale is missing a code the source locale defines.
- [ ] The generator flags (fails, or reports depending on CI mode) catalog entries for error
      codes that no longer exist in the model.
- [ ] `--check` fails when the checked-in generated files are stale relative to `model.yaml`.
- [ ] Multiple audit events are permitted to declare the same `error_code` — this is not
      flagged as a duplicate or a conflict.
- [ ] A generated Go consumer exposes the new enum/event/error-code as a typed constant usable
      in an exhaustive `switch`, so an unhandled new value fails Go compilation at the call site
      that needs to handle it.
- [ ] A generated TypeScript consumer exposes the same information as a typed union / map, so
      an unhandled new error code fails the SPA's type-check at `errorMessages.ts`.
- [ ] `libs/i18n/locales/` and `apps/web/src/i18n/locales/` are validated independently for
      coverage by `check_i18n_parity.py`, and a missing key in either fails that check without
      requiring the two catalogs' content to match.
- [ ] Hand-editing `libs/auditmodel/generated.go` or `apps/web/src/api/generated/audit.ts`
      is not the sanctioned path — the next generator run overwrites manual edits, and
      `--check` in CI catches drift before merge.
- [ ] Every `error_codes` entry, every `events` entry, and every value under every enum in
      `enums:` gets a generator-assigned 8-character compact code: 4-letter prefix + a kind
      digit (`0` for error codes, `1` for audit events, `2` for enum values) + a 3-digit
      sequence number, unique per (prefix, kind).
- [ ] For an error code or audit event, the prefix derives from the entry's own first name
      segment. For an enum value, the prefix derives from its **enum's** name, not the value —
      so `RoutineStatus.active` and `OccurrenceStatus.active` (same value spelling, different
      enums) get different prefixes and never collide.
- [ ] A compact code, once assigned to a readable name, never changes on subsequent generator
      runs — regenerating after unrelated model changes leaves every existing mapping intact.
- [ ] Removing an `error_codes`/`events` entry or an enum value retires its compact code; the
      generator never reassigns a retired (prefix, kind, sequence) to a different entry,
      including one that happens to derive the same 4-letter prefix or reuses the same value
      spelling under a different enum.
- [ ] Adding a new value to an existing enum only assigns a compact code to the new value — it
      never changes the compact codes already assigned to that enum's other values.
- [ ] The bidirectional readable↔compact map is emitted into both `generated.go` and
      `audit.ts` as part of the normal generation step — no separate command or DB migration is
      required to produce it.
- [ ] No compact code appears in an API response body, a rendered audit `message`, an SPA
      string, or application business logic — translation back to the readable form happens
      before a row reaches a handler, response serializer, message renderer, or a comparison
      against a readable enum constant.

## Acceptance criteria (scenarios)

### Scenario: engineer adds a new error code end-to-end
**Given** a backend engineer is implementing a feature that needs a new external failure code
**And** no existing error code in `model.yaml` covers the case
**When** they add an `error_codes` entry with a name, `http_status`, and description (no message
text), add an en-US entry to `libs/i18n/locales/`, add entries to every other server locale, add
a mapping in `apps/web/src/i18n/errorMessages.ts` and a key in both SPA locale catalogs, then run
`python3 scripts/gen_audit_model.py`
**Then** `libs/auditmodel/generated.go` and `apps/web/src/api/generated/audit.ts` are regenerated
to include the new code
**And** the Go build and the TypeScript type-check both succeed
**And** `scripts/gen_audit_model.py --check` and `scripts/check_i18n_parity.py` both pass

### Scenario: several audit events share one error code deliberately
**Given** `auth.login.failed` is declared with `reason: {enum: LoginFailureReason}` and
`error_code: AUTH_INVALID_CREDENTIALS`
**And** a second, distinct audit event also declares `error_code: AUTH_INVALID_CREDENTIALS`
**When** the generator runs
**Then** it completes successfully with no duplicate-error-code warning or failure
**And** both events remain independently queryable by their own dotted name and `reason` field
in the audit record, while the external response is identical for both

### Scenario: message template references an undeclared field
**Given** an audit event declares `fields: { user_id: {type: id} }`
**And** its `message` template is `"Action performed on {target_id}"`, where `target_id` is not
one of the declared fields
**When** `python3 scripts/gen_audit_model.py` runs
**Then** the generator exits non-zero
**And** no generated file is updated
**And** the failure output names the event and the unresolved placeholder

### Scenario: field name looks like a secret
**Given** an engineer declares an audit event field named `password` or `access_token`
**When** the generator runs
**Then** it rejects the model with a non-zero exit before generating any output
**And** the rejection message identifies the offending field name

### Scenario: error code missing a locale entry
**Given** a new error code `NUTR_ESTIMATE_STALE` is added to `model.yaml` with `http_status` and
a description
**And** an en-US message entry exists in `libs/i18n/locales/en-US.json`
**But** the `vi-VN` locale file has no entry for `NUTR_ESTIMATE_STALE`
**When** `python3 scripts/gen_audit_model.py --check` runs in CI
**Then** the check fails
**And** the failure names the missing locale and the missing code

### Scenario: stale generated file caught by --check
**Given** `model.yaml` was edited to add a new enum value
**And** `libs/auditmodel/generated.go` was not regenerated afterward
**When** `python3 scripts/gen_audit_model.py --check` runs
**Then** it fails, reporting that the generated file is out of date with the model
**And** it makes no changes to the working tree (check-only mode)

### Scenario: orphaned catalog entry after an error code is removed
**Given** an error code `LEGACY_CODE_X` previously existed and still has entries in
`libs/i18n/locales/` and the SPA catalogs
**But** `LEGACY_CODE_X` has since been removed from `model.yaml`
**When** the generator runs
**Then** it flags the orphaned catalog entries so they can be deliberately deleted
**And** it does not silently regenerate a reference to a code that no longer exists

### Scenario: hand-edited generated file is reverted
**Given** an engineer directly edits `libs/auditmodel/generated.go` instead of `model.yaml`
**When** `python3 scripts/gen_audit_model.py` is next run (locally or in CI)
**Then** the hand-edited content is overwritten to match what `model.yaml` declares
**And** if this happens in CI before the edit was regenerated, `--check` fails first, catching
the drift before merge rather than silently discarding the edit post-merge

### Scenario: unresolved enum reference in an audit event field
**Given** an audit event declares a field as `{enum: TypoedEnumName}`
**And** no enum named `TypoedEnumName` exists in the model's `enums` section
**When** the generator runs
**Then** it exits non-zero, naming the event, the field, and the unresolved enum reference
**And** no partial output is written

### Scenario: new error code and new audit event each get a distinct compact code
**Given** `model.yaml` has no prior entry with the prefix `NUTR`
**And** a new error code `NUTR_ESTIMATE_STALE` and a new audit event `nutr.estimate.expired`
are both added
**When** the generator runs
**Then** `NUTR_ESTIMATE_STALE` is assigned a compact code matching `NUTR0\d{3}` (kind digit `0`)
**And** `nutr.estimate.expired` is assigned a compact code matching `NUTR1\d{3}` (kind digit `1`)
**And** the two compact codes are different from each other despite sharing the same prefix

### Scenario: compact code is stable across unrelated regeneration
**Given** `AUTH_INVALID_CREDENTIALS` already has an assigned compact code from a prior generator
run
**When** an unrelated new error code is added and the generator runs again
**Then** `AUTH_INVALID_CREDENTIALS`'s compact code in the regenerated output is byte-for-byte
identical to what it was before
**And** the new error code receives a compact code that was not previously in use

### Scenario: removed entry's compact code is retired, not recycled
**Given** an error code `LEGACY_CODE_X` had compact code `LEGA0007`
**And** `LEGACY_CODE_X` is removed from `model.yaml`
**When** a new, unrelated error code that also derives the prefix `LEGA` is added and the
generator runs
**Then** the new entry receives a sequence number greater than `007` for the `(LEGA, 0)` pair
**And** `LEGA0007` is not reassigned to the new entry

### Scenario: compact code never reaches the API response
**Given** a stored audit row and a stored error condition both carry only their compact codes
**When** a service reads the row and constructs an API error response or an audit query result
for a human
**Then** the response body and the rendered audit output both show the readable name
(`AUTH_INVALID_CREDENTIALS`, `auth.login.failed`), never the compact code
**And** the translation happens via the generated in-process map, not a database join

### Scenario: enum value's compact prefix derives from the enum name, not the value
**Given** `RoutineStatus` has a value `active` and `OccurrenceStatus` also has a value `active`
**When** the generator assigns compact codes to both
**Then** `RoutineStatus.active`'s compact code has prefix `ROUT`
**And** `OccurrenceStatus.active`'s compact code has prefix `OCCU`
**And** the two compact codes are entirely different, despite both source values being the
literal string `active`

### Scenario: adding a new value to an existing enum doesn't disturb existing codes
**Given** `TaskStatus` already has values `unchecked` and `checked`, each with an assigned
compact code
**When** a new value `skipped` is added to `TaskStatus` and the generator runs
**Then** `unchecked` and `checked` keep their existing compact codes unchanged
**And** `skipped` receives a new compact code with prefix `TASK`, kind digit `2`, and the next
unused sequence number for `(TASK, 2)`

### Scenario: a domain table storing an enum sees only the compact code
**Given** a `routines` table has a `status` column typed to store `RoutineStatus` values
**When** application code writes `RoutineStatus.active` to a new row
**Then** the persisted value in the column is the compact code (e.g. `ROUT2001`), not the string
`active`
**And** when that row is read back through the service layer, the value presented to business
logic and to any exhaustive switch is the readable constant `RoutineStatus.active`, translated
via the generated map
