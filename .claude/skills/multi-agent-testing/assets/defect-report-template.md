# DEF-<NNN>: <one-line symptom>

**Run:** `docs/stories/<CODE>-<slug>/`
**Commit under test:** <hash>
**Raised by:** <tester id>
**Severity:** S1 | S2 | S3 | S4
**Classification (tester's judgment):** slice | contract-or-partition | spec
**Suspected slice(s):** <slice-id> — <why: the brief that names the failing test case, the
file the wrong behavior lives in, ...>
**Status:** open | in-fix | fixed-pending-retest | closed | reopened | deferred

## Expected

<What should happen, and where that expectation comes from — quote the story AC scenario,
the test-case ID, or the contract clause. If you can't trace it to one of those, it may be a
`spec` defect.>

## Actual

<What happens. Paste real output: the response body, the log line, the audit row, the
rendered text. Screenshots for visual defects.>

## Reproduce

<Exact, minimal, starting from a known state. Someone who has never seen this feature runs
these and sees the failure.>

1. Starting state: <fresh DB / user X exists / ...>
2. Locale: `en-US` | `vi-VN` | both
3. <step>
4. <step>

**Story test case(s) failing:** TC-<..>
**Also blocks:** TC-<..> (cases that couldn't be run because of this)

## Notes

<Anything that helps the fixer without telling them how to fix it: related executor
"noticed" items, whether it reproduces in one locale only, whether it appeared after an
amendment.>

---

## Instructor triage

**Classification confirmed:** slice | contract-or-partition | spec
**Routed to:** phase 4 (fix brief `fixes/DEF-<NNN>.md`) | phase 1 (amendment) | story update
**Reasoning:** <if you changed the tester's classification, say why>

---

## Retest

| Pass | Commit | Result | Tester | Notes |
|------|--------|--------|--------|-------|
| 1 | <hash> | closed / reopened | <id> | |
