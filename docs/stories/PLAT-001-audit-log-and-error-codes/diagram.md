# Flow: Audit Log & Error Code Model

**Code:** `PLAT-001`

```mermaid
flowchart TD
    A[Engineer edits libs/auditmodel/model.yaml:<br/>new enum value, audit event, or error code] --> B[Run scripts/gen_audit_model.py]

    B --> C{References resolve?<br/>enum refs, error_code refs}
    C -->|no| C1[FAIL: unresolved reference<br/>names event/field + target]
    C -->|yes| D{Message placeholders<br/>all match declared fields?}

    D -->|no| D1[FAIL: unresolved placeholder<br/>names event + placeholder]
    D -->|yes| E{Any field name<br/>looks like a secret?}

    E -->|yes| E1[FAIL: reject before generating<br/>names offending field]
    E -->|no| F{Enum values lower_snake_case?<br/>Enum names PascalCase?}

    F -->|no| F1[FAIL: naming violation<br/>names value/enum + expected form]
    F -->|yes| G{Error code has<br/>http_status? no inline message?}

    G -->|missing status| G1[FAIL: missing http_status]
    G -->|has inline message| G2[FAIL: message text not allowed in model]
    G -->|ok| H{Every error code has an<br/>en-US catalog entry?}

    H -->|no| H1[FAIL: code has no message<br/>entry means empty message to caller]
    H -->|yes| I{Every non-source locale<br/>covers what en-US covers?}

    I -->|no| I1[FAIL: names missing locale + code]
    I -->|yes| J{Catalog entries reference<br/>codes still in the model?}

    J -->|orphaned entry found| J1[FLAG: orphaned catalog entry<br/>not auto-deleted, surfaced for review]
    J -->|clean| K[Multiple audit events may<br/>share one error_code: allowed,<br/>not flagged as duplicate]

    K --> K1[Also walk every value under<br/>every enum in enums: the same way]
    K1 --> K2{Entry/value already has an<br/>assigned compact code?}
    K2 -->|yes, from a prior run| K3[Reuse existing compact code<br/>unchanged, byte-for-byte]
    K2 -->|no, new entry/value| K4[Assign next unused compact code:<br/>prefix = entry name segment for<br/>error_codes/events, ENUM NAME for<br/>enum values + kind digit<br/>0=error_code 1=event 2=enum value<br/>+ next sequence for that prefix,kind pair]
    K3 --> L[Write libs/auditmodel/generated.go<br/>+ apps/web/src/api/generated/audit.ts<br/>including the readable&lt;-&gt;compact map]
    K4 --> L
    J1 --> L

    L --> M{Run mode?}
    M -->|plain run| N[Files written/overwritten,<br/>including reverting any hand-edits]
    M -->|--check in CI| O{Generated files match<br/>what model.yaml would produce?}

    O -->|stale/out of date| O1[FAIL: generated file out of date<br/>working tree unchanged]
    O -->|up to date| P[CI check passes]

    N --> Q[Go build: new/changed constants<br/>available; exhaustive switches<br/>without a case now fail to compile]
    N --> R[TS type-check: new/changed types<br/>available; Record&lt;ErrorCode,string&gt;<br/>missing an entry now fails to compile]

    Q --> S[scripts/check_i18n_parity.py:<br/>verifies key coverage only,<br/>server and SPA wording may differ]
    R --> S
    S --> T[Feature code can now emit the<br/>audit event / return the error code]
```

## Notes on non-obvious branches

- **C, D, E, F, G, H, I checks are independent gates, not one combined check** — each corresponds
  to its own AC scenario (unresolved enum reference, message placeholder, secret-shaped field,
  naming convention, missing `http_status` / inline message, missing en-US entry, missing
  non-source locale). The generator is expected to fail fast at the first violated gate rather
  than continue and report everything at once, per the scenarios in
  [acceptance-criteria.md](acceptance-criteria.md).
- **J (orphaned catalog entries) is a flag, not a hard failure that blocks generation** — see
  *Scenario: orphaned catalog entry after an error code is removed*. It's surfaced for a human
  to deliberately delete, not silently cleaned up, because an automatic delete could remove a
  catalog entry someone was mid-way through re-adding for a re-introduced code.
- **K (shared error codes) is explicitly a non-branch** — it's drawn to make clear that the
  generator must *not* treat two audit events sharing one `error_code` as a conflict, per
  *Scenario: several audit events share one error code deliberately*. This is the enumeration-
  protection pattern (`AUTH_INVALID_CREDENTIALS` covering both `unknown_email` and
  `bad_password`) that `docs/audit-and-errors.md` §4 calls "the most important rule in this
  document."
- **M splits into two outcomes** because the same validation logic serves two different callers:
  a plain run is expected to *fix* drift (regenerate and overwrite, including reverting a
  hand-edit — see *Scenario: hand-edited generated file is reverted*), while `--check` is
  expected to *detect* drift without writing anything, which is what makes it safe to run in CI
  before merge (*Scenario: stale generated file caught by --check*).
- **Q and R are compiler-enforced, not generator-enforced** — the generator's job ends at
  producing typed consumers; the "you must handle new values" guarantee comes from Go's
  exhaustive-switch compile error and TypeScript's `Record<ErrorCode, string>` mapped-type
  requirement, which is why AC calls this out as an intended, not incidental, effect.
- **K2/K3/K4 is where compact-code permanence lives** — an existing entry's compact code is
  never recomputed, only looked up and re-emitted (*Scenario: compact code is stable across
  unrelated regeneration*), and a new entry's assignment walks forward from the highest
  sequence number ever used for that (prefix, kind) pair, including retired ones
  (*Scenario: removed entry's compact code is retired, not recycled*).
- **K1's prefix source differs from error_codes/events** — an enum value's prefix comes from its
  *enum's* name, not the value string, specifically so `RoutineStatus.active` and
  `OccurrenceStatus.active` land on different prefixes despite an identical value spelling
  (*Scenario: enum value's compact prefix derives from the enum name, not the value*). Adding one
  new value to an existing enum only assigns that one value a code; its siblings' codes are
  untouched (*Scenario: adding a new value to an existing enum doesn't disturb existing codes*).

## Flow: compact code at runtime (read/write path)

The generation flow above is build-time. Separately, every time a service reads or writes a row
that carries a compact code — an audit record, an error condition, or a domain table column
typed to one of the enums in `model.yaml` (e.g. `routines.status`) — it must translate at the
boundary. This is a per-request sequence, not a branching decision tree, so it's shown as a
sequence diagram instead.

```mermaid
sequenceDiagram
    participant Caller as API caller / SPA
    participant Svc as Backend service
    participant Map as Generated readable<->compact map<br/>(in-process, from generated.go/audit.ts)
    participant DB as PostgreSQL

    Note over Svc,DB: Write path (e.g. routine created as active)
    Svc->>Map: Translate readable -> compact<br/>RoutineStatus.active
    Map-->>Svc: ROUT2001
    Svc->>DB: INSERT ... status = 'ROUT2001'

    Note over Caller,DB: Read path (e.g. login failure, or reading the row back)
    Caller->>Svc: Request that triggers a failure,<br/>reads audit history, or reads a routine
    Svc->>DB: Query row(s)
    DB-->>Svc: Row(s) with compact code(s)<br/>e.g. AUTH0001 / AUTH1003 / ROUT2001
    Svc->>Map: Translate compact -> readable
    Map-->>Svc: AUTH_INVALID_CREDENTIALS /<br/>auth.login.failed / RoutineStatus.active
    Svc-->>Caller: Response, rendered audit view, or<br/>domain object carries ONLY the readable code
```

The compact code never crosses the `Svc-->>Caller` boundary, and application logic (an exhaustive
switch on `RoutineStatus`, a comparison against `AUTH_INVALID_CREDENTIALS`) only ever sees the
readable form (*Scenario: compact code never reaches the API response*;
*Scenario: a domain table storing an enum sees only the compact code*) — translation happens
entirely within the backend service, using the same generated map the build-time flow populates,
with no database join and no separate lookup table.
