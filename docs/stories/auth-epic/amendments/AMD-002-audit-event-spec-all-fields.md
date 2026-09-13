# Amendment request: Add AllFields to AuditEventSpec

**Raised by:** `libs-auditlog`
**Date:** 2026-09-13
**Blocks:** undeclared-field validation in `libs/auditlog.validate`

## What the contract currently says

`libs/auditmodel/generated.go` exports `AuditEventSpec` with:

```go
type AuditEventSpec struct {
    Actor          ActorType
    Outcome        Outcome
    Severity       Severity
    Message        string
    RequiredFields []string
    ErrorCode      ErrorCode
}
```

`RequiredFields` lists only the fields that must be supplied; optional fields
(those declared in `model.yaml` with `optional: true`) are not represented.

## What's wrong with it

The brief (`libs-auditlog.md`) requires the writer to reject "a field the event
never declared." To do that, the writer must know the complete set of declared
fields — required **and** optional.

With only `RequiredFields`, the writer can check that required fields are
present, but cannot determine whether a field the caller passed was declared at
all. An undeclared field (a typo, a copy-paste error, a field from the wrong
event) would silently reach the DB rather than failing at the call site.

Concrete case: `auth.login.failed` requires `["reason", "source_ip"]` and
optionally declares `"user_id"`. If a caller passes `"usr_id"` (typo), the
writer has no way to detect it with the current struct — it sees only that the
two required fields are present.

## What I propose instead

Add `OptionalFields []string` to `AuditEventSpec` (or a combined `AllFields
[]string` that lists required + optional). The generator already knows which
fields are optional from the `optional: true` annotation in `model.yaml`.

```go
type AuditEventSpec struct {
    Actor          ActorType
    Outcome        Outcome
    Severity       Severity
    Message        string
    RequiredFields []string
    OptionalFields []string  // NEW — fields with optional: true in model.yaml
    ErrorCode      ErrorCode
}
```

## Who else this affects

- `libs-authmw` — reads AuditEventSpec to build middleware claims; unaffected
  by adding a field.
- `be-auth-credentials`, `be-auth-session`, `be-auth-store`, `be-gateway-authz`
  — these are wave-3 slices that call `Write()`. They benefit from the stricter
  validation but do not directly read AuditEventSpec.
- Any future slice that reads `AuditEventSpec` directly would gain access to
  optional-field info without any API change.

No finished slice is built against `OptionalFields` (it doesn't exist yet), so
there are no back-compat breaks.

## What I did in the meantime

The `libs/auditlog` package implements a local `eventAllFields` table (in
`writer.go`) that lists required + optional fields for every currently-declared
event. This table fills the gap and passes all tests. It is marked with a
comment referencing this amendment so it can be removed once the generated
struct gains `OptionalFields`.

The table is updated whenever a new event is added (it already includes the
contract §7 events that plat-audit-model will ship). The downside is that it
duplicates knowledge from `model.yaml` into a second location — exactly the
coupling the single-source-of-truth rule exists to prevent.

---

## Instructor decision

**Decision:** **Accepted, as proposed.**

**Reasoning:** This is a generator/tooling gap, not a contract-surface dispute — nothing in
`contract.md` specifies `AuditEventSpec`'s exact Go shape, so accepting this doesn't move the
frozen contract or its commit hash. The addition is purely additive (a new field on a struct
literal), the generator already computes exactly this set for `RequiredFields` — the fix is
the same one-line filter inverted (`fs.get("optional")` instead of `not fs.get("optional")`) —
and no finished slice reads the old shape in a way a new field could break. The interim
workaround was implemented correctly: confined to `libs-auditlog`'s own allowlist, marked with
a removal pointer to this amendment, and it did not touch `libs/auditmodel/**` or invent a
second definition of the *contract* — it duplicated field names, which model.yaml already
"owns" as the source of truth, purely as a local cache pending the real fix. That is the
allowed form of "continue what doesn't depend on the disputed shape": the writer's behavior
(reject undeclared fields) shipped on schedule without pre-empting the instructor's decision
on the struct's shape.

**Contract updated:** No change to `contract.md` (contract freeze commit unchanged).
`scripts/gen_audit_model.py` gains `OptionalFields []string` on `AuditEventSpec`, populated
from each event's `optional: true` fields — the same filter already used for `RequiredFields`,
inverted. Regenerated: `libs/auditmodel/generated.go` gains the field on every event entry;
`apps/web/src/api/generated/audit.ts` is **unchanged** (the SPA never writes audit records, so
it has no equivalent struct); `libs/auditmodel/compact_codes.json` is **unchanged** (diffed
line-by-line — zero lines differ, confirming no compact code moved).

**Slices to re-verify:** None beyond `libs-auditlog` itself, which is being integrated against
the corrected generated file directly (its local `eventAllFields` shadow table is removed as
part of that integration, per its own plan). `plat-audit-model` had already reported done and
is a **finished slice built against the changed surface** in the sense that its generator run
produced the file — but its own deliverable (the enums, codes and events in `model.yaml`) is
untouched, and the struct-shape change is downstream tooling it doesn't consume. No
re-verification of `plat-audit-model`'s own output was needed; re-running the generator
confirmed zero diff outside the new field. `libs-authmw` reads `AuditEventSpec` for its
own purposes and was already told in its brief that this field would be additive and safe —
no action needed there either, and it has not yet reported done.

**Notified:** Recorded here for every future reader; no other in-flight slice was consuming
the old shape at the time of this decision.
