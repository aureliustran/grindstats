# Amendment request: Add ErrorWithDetails to httpkit

**Raised by:** `be-auth-credentials`
**Date:** 2026-09-14
**Blocks:** VALIDATION_FAILED responses with field-level `details` in credentials handlers

## What the contract currently says

`libs/httpkit/envelope.go` exports a single `Error(c *gin.Context, code auditmodel.ErrorCode)`
function that writes:

```json
{"error":{"code":"VALIDATION_FAILED","message":"..."}}
```

## What's wrong with it

The auth-epic contract §1.1 specifies that `VALIDATION_FAILED` responses carry an optional
`details` array:

```json
{"error":{"code":"VALIDATION_FAILED","message":"...","details":[{"field":"password","rule":"breached"}]}}
```

`details` is declared optional ("clients must tolerate its absence"), but the two cases where
`be-auth-credentials` must emit it — minimum-length failure and HIBP-corpus hit — have distinct
machine-readable `rule` values that the frontend must read to show the correct inline error
message on the field. Without `details`, both map to the generic `VALIDATION_FAILED` copy,
losing the per-field context.

`httpkit.Error` offers no parameter for details, so a caller has no compliant way to include
them.

## What I propose instead

Add a sibling function to `libs/httpkit`:

```go
// ValidationDetail is one element in the details array of a VALIDATION_FAILED envelope.
type ValidationDetail struct {
    Field string `json:"field"`
    Rule  string `json:"rule"`
}

// ErrorWithDetails writes a VALIDATION_FAILED envelope that includes a details array.
// The status is always 400 (the only status VALIDATION_FAILED maps to).
// An empty or nil details slice writes the same body as Error(c, ErrValidationFailed).
func ErrorWithDetails(c *gin.Context, details []ValidationDetail) { ... }
```

No other error code uses `details` in run 1, so a VALIDATION_FAILED-specific function is
the right scope, not a generic `details` parameter on `Error`.

## Who else this affects

- `be-auth-session` — login validates email format (VALIDATION_FAILED), currently no
  per-field detail needed; unaffected.
- `be-gateway-authz` — no VALIDATION_FAILED paths; unaffected.
- Frontend types in `apps/web/src/api/auth.types.ts` already declare `details?:
  ValidationDetail[]` on `ApiError` (contract §8.1), so the frontend is already shaped for
  this and needs no change.

## What I did in the meantime

`services/monolith/internal/auth/credentials/handler.go` exports a package-private
`writeValidationFailed(c, details)` helper that produces the identical envelope shape using
`c.AbortWithStatusJSON`. This violates the letter of the "only through httpkit" rule, is
marked with a `// TODO: AMD-004` comment pointing here, and will be replaced with
`httpkit.ErrorWithDetails` once this amendment is accepted.

The function is five lines and matches the httpkit envelope format exactly, so there is no
semantic drift — only a source-location violation.

---

## Instructor decision

**Decision:** **Accepted, deferred implementation.**

**Reasoning:** This gap was found independently by two separate executions of this slice (the
first, discarded for an unrelated reason — a fabricated `authdomain` — reached the same
conclusion about `httpkit` before being thrown away; the second, accepted, found it again on
its own). Two independent reads of the same contract landing on the same gap is strong
confirmation the gap is real, not a misreading.

The interim workaround is accepted as-is for this run: it's five lines, produces the
byte-identical envelope shape (verified: the two enumeration-neutral response pairs in this
slice's own tests assert raw body equality, and both pass), stays inside the slice's own
allowlist, and is clearly marked for removal. Promoting it to `libs/httpkit.ErrorWithDetails`
is correct and low-risk (no signature changes to `Error`, no consumer of the old function
affected) but is **not required to unblock this run** — no other in-flight slice needs it, and
`libs/httpkit` is not on the allowlist of any wave-3 or wave-4 slice, so implementing it now
would need a dedicated small slice of its own for a one-function, no-urgency addition.

**Contract updated:** No change to `contract.md` (the shape in §1.1 already specifies
`details`; this amendment is about *where the code that emits it lives*, not what it emits).

**Action:** Filed as a follow-up, not built in this run. `services/monolith/internal/auth/
credentials/handler.go`'s `writeValidationError` stays as the interim implementation
post-integration; the `// TODO: AMD-004` comment remains the pointer for whoever picks this up.

**Slices to re-verify:** None. No slice consumes `libs/httpkit.Error`'s signature in a way
this would change.

**Notified:** Recorded here; no other in-flight slice was affected.
