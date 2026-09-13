# Execution report: libs-auditlog

**Slice:** `libs-auditlog`
**Date:** 2026-09-13
**Branch/worktree:** `worktree-agent-a1dc1715a568764ac`
**Status:** Done — all ungated tests pass; DB-gated integration test skipped (no live Postgres).

---

## 1. Changed

| File | What it does |
|---|---|
| `libs/auditlog/writer.go` | `WriteContext`, `Writer` interface, `PgxWriter` implementation with `New`, `validate`, `renderMessage`, `compactEnumValue`, `eventAllFields` table. |
| `libs/auditlog/fake.go` | `Record` struct, `Fake` implementation with `Write`, `Records`, `EventsOf`, `Count`, `Reset`. |
| `libs/auditlog/writer_test.go` | Unit tests for validation (required field, undeclared field, credential rejection, unknown event), message rendering, compact-code translation; DB-gated integration test (skipped without `GRINDSTATS_TEST_DB`). |
| `libs/auditlog/fake_test.go` | Tests for Fake: records stored, severity/outcome from spec, field assertion by value, fields readable (not compacted), validation errors surfaced, EventsOf filtering, Reset, en-US message, `Writer` interface compliance. |
| `docs/stories/auth-epic/amendments/AMD-002-audit-event-spec-all-fields.md` | Amendment request for `OptionalFields []string` in `AuditEventSpec` — see §3. |

**Nothing outside `libs/auditlog/**` and the amendments directory was modified.**
The merge of `f01ca6a` (be-wiring pre-step) was required to make `github.com/google/uuid` available; the merge is a fast-forward with no conflict and no owned-file overlap.

---

## 2. Verified

### Build and vet

```
$ go build ./libs/auditlog/...
(exit 0, no output)

$ go vet ./libs/auditlog/...
(exit 0, no output)

$ go build ./...
(exit 0, no output)
```

### Unit test suite (ungated)

```
$ go test ./libs/auditlog/... -v
=== RUN   TestFake_RecordsEvent                              --- PASS (0.00s)
=== RUN   TestFake_SeverityAndOutcomeFromSpec                --- PASS (0.00s)
=== RUN   TestFake_FieldAssertionByValue                     --- PASS (0.00s)
=== RUN   TestFake_FieldsAreReadable                         --- PASS (0.00s)
=== RUN   TestFake_ValidationErrors_Surfaced                 --- PASS (0.00s)
=== RUN   TestFake_EventsOf_FiltersByEvent                   --- PASS (0.00s)
=== RUN   TestFake_Reset                                     --- PASS (0.00s)
=== RUN   TestFake_MessageRenderedEnUs                       --- PASS (0.00s)
=== RUN   TestFake_ImplementsWriter                          --- PASS (0.00s)
=== RUN   TestValidate_MissingRequiredField                  --- PASS (0.00s)
=== RUN   TestValidate_AllRequiredFieldsPresent              --- PASS (0.00s)
=== RUN   TestValidate_UndeclaredField                       --- PASS (0.00s)
=== RUN   TestValidate_DeclaredOptionalField_Accepted        --- PASS (0.00s)
=== RUN   TestValidate_CredentialFieldName_Rejected
    /password /Password /PASSWORD /token /secret /hash /key /authorization
                                                             --- PASS (0.00s)
=== RUN   TestValidate_UnknownEvent                          --- PASS (0.00s)
=== RUN   TestRenderMessage_BasicSubstitution                --- PASS (0.00s)
=== RUN   TestRenderMessage_OptionalFieldAbsent_NoLiteralPlaceholder  --- PASS (0.00s)
=== RUN   TestRenderMessage_IsEnUsRegardlessOfAmbientState   --- PASS (0.00s)
=== RUN   TestCompactEnumValue_LoginFailureReason            --- PASS (0.00s)
=== RUN   TestCompactEnumValue_TokenRejectReason             --- PASS (0.00s)
=== RUN   TestCompactEnumValue_NonEnum_PassedThrough         --- PASS (0.00s)
=== RUN   TestCompactEnumValue_AllEnumsHaveCompactCode       --- PASS (0.00s)
=== RUN   TestWriter_CompactCodes_UnitPreview                --- PASS (0.00s)
=== RUN   TestPgxWriter_Integration_ValidEventRoundTrips
    SKIP: GRINDSTATS_TEST_DB not set; skipping database-gated integration test
ok  grindstats/libs/auditlog  0.117s
```

### Full suite (no regressions)

```
$ go test ./...
ok  grindstats/libs/auditlog             0.136s
?   grindstats/libs/auditmodel           [no test files]
ok  grindstats/libs/httpkit              0.113s
ok  grindstats/libs/i18n                 0.318s
?   grindstats/services/monolith/cmd/server [no test files]
ok  grindstats/services/monolith/internal/gateway          0.113s
ok  grindstats/services/monolith/internal/gateway/health   0.304s
ok  grindstats/services/monolith/internal/gateway/middleware 0.114s
ok  grindstats/services/monolith/internal/platform/cache   2.217s
ok  grindstats/services/monolith/internal/platform/config  0.291s
ok  grindstats/services/monolith/internal/platform/db      0.581s
```

### Scenario-to-test map

| Scenario | Test name | Result |
|---|---|---|
| A valid event round-trips: readable in, compact stored, en-US message rendered | `TestWriter_CompactCodes_UnitPreview` (unit) + `TestPgxWriter_Integration_ValidEventRoundTrips` (DB-gated, skipped) | PASS / SKIP |
| A missing required field fails at the call site | `TestValidate_MissingRequiredField` | PASS |
| An undeclared field fails at the call site | `TestValidate_UndeclaredField` | PASS |
| A credential-looking field name is rejected | `TestValidate_CredentialFieldName_Rejected` (8 sub-cases) | PASS |
| The rendered message is en-US regardless of any ambient locale state | `TestRenderMessage_IsEnUsRegardlessOfAmbientState` | PASS |
| The Fake records what the real writer would have written, for the same call | `TestFake_RecordsEvent`, `TestFake_FieldAssertionByValue`, `TestFake_FieldsAreReadable`, `TestFake_MessageRenderedEnUs`, `TestFake_ImplementsWriter` | PASS |

---

## 3. Could not do

### DB-gated integration test

`TestPgxWriter_Integration_ValidEventRoundTrips` requires `GRINDSTATS_TEST_DB` to be set to a Postgres DSN with the `audit` schema applied. No database was available in this sandbox. The test skips cleanly with `t.Skip(...)` and is ready to run once the migrations from the `db-migrations` slice are applied.

### Compact codes for plat-audit-model enum types

The contract §7 adds three new enum types (`AccountStatus`, `LinkKind`, `LinkRejectReason`). The `compactEnumValue` function in `writer.go` uses a type switch; cases for these types cannot be added until plat-audit-model's generated constants exist. Their fields will be stored as readable strings until then. A `// TODO` comment marks the location in `compactEnumValue`.

---

## 4. Noticed

### AMD-002 filed — `AuditEventSpec` missing `OptionalFields`

`libs/auditmodel/generated.go` exposes `RequiredFields []string` but not optional fields. Without the full field set, undeclared-field validation requires a local shadow table (`eventAllFields` in `writer.go`). This duplicates knowledge from `model.yaml` into a second file, which is exactly what the single-source-of-truth rule exists to prevent. Amendment request `AMD-002` proposes adding `OptionalFields []string` to `AuditEventSpec`; when accepted, the shadow table can be removed.

### model.yaml note: `LoginFailureReason.account_suspended` note missing

Contract §0 D2 says `LoginFailureReason.account_suspended` "gains a `note:` recording that D2 retired its use." The current `model.yaml` has the value but no `note:` on it. This is cosmetic (compact code is permanent, value is still valid), but the contract says it should be there. Out of scope for this slice; noted for plat-audit-model or the instructor.

### `auth.login.succeeded` message template references optional field

The message template `"Login succeeded for {user_id} from {device_label}"` references `device_label`, which is optional. When absent the rendered message ends with "from " (trailing space). This is a cosmetic issue with the model template, not with this package. Noted for plat-audit-model.
