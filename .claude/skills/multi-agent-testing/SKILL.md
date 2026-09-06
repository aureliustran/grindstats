---
name: multi-agent-testing
description: Phase 3 of the multi-agent pipeline — the tester role, run by agents who did not write the slices under test. Verifies the assembled work against the story's test cases, the frozen contract (conformance, not just green unit tests) and the real end-to-end user path; produces a test report and one defect report per failure with a reproducible case, severity, classification (slice / contract-or-partition / spec) and a suspected slice. Testers change no source code — ever. Use after gate 2 passes, and again after every bug-fixing pass until the exit criterion holds. Also use when a previous run "passed all tests" but broke on first contact between slices — that is the gap this phase exists to close.
---

# Phase 3 — Testing

You are a tester. You verify what the executors built, as a whole, against what was
specified — and you **do not fix anything**. Not a one-line bug, not a flaky test, not a
typo. The moment you edit source, nobody knows what was verified against what, and the
defect you were about to report disappears from the record while the class of bug stays.

You were chosen because you did not write the slice you're testing. Keep it that way: if
you are asked to test something you built, say so and hand it off.

Pipeline context and gates: `.claude/skills/multi-agent-code-execution/SKILL.md`.

## Before you start — check gate 2

Verify these yourself; don't take the executors' or instructor's word for it:

- [ ] Every slice in `plan.md` has a report in `reports/` with real command output
- [ ] No open amendment requests in `amendments/` (each has an instructor decision, and
      the re-verify list was actioned)
- [ ] Each executor's diff stays inside its allowlist — actually diff it against
      `plan.md`'s ownership map
- [ ] Clean checkout builds: backend compiles, frontend type-checks,
      `python3 scripts/gen_audit_model.py --check` and
      `python3 scripts/check_i18n_parity.py` pass

If any fails, **stop and send it back to phase 2** via the instructor. Don't test a tree
that doesn't build — every defect you'd file is about the build, and that's noise that
buries the real ones.

## What to test, in order

### 1. Contract conformance — before anything else

Every slice's unit tests can pass while the slices disagree with each other. That is the
failure mode this phase exists for, so check the seams first:

- the generated API client compiles against the current spec, and the backend serves what
  the spec says (shape, status codes, error envelope `{ "error": { "code", "message" } }`)
- every error code a handler can return is in `libs/auditmodel/model.yaml`, and every code
  the frontend switches on exists in `ERROR_MESSAGE_KEYS` (`apps/web/src/i18n/errorMessages.ts`)
- event payloads validate on both the publishing and consuming side
- `Accept-Language` is honored: the same failing request returns an `en-US` and a `vi-VN`
  message; the audit row written for it is `en-US` regardless
- the checks in `docs/shared-contract.md` §5

### 2. Story test cases

Run every row of `docs/stories/<CODE>-<slug>/test-cases.md`. Record each ID as pass / fail
/ blocked in `test-report.md`. A blocked case (can't be exercised because of an earlier
failure) is recorded as blocked, not skipped and forgotten.

For a failing case, the brief that named that test case tells you which slice to *suspect*.
Write that down as a suspicion. Assignment is the instructor's job — the same symptom is
often two slices each half-right about a seam.

### 3. End-to-end user path

Build, migrate, start it, and walk the actual path from `story.md` as the user would,
in both locales. Parallel slices that each work in isolation frequently fail on first
contact; this is where you find out. Check the design-system and i18n definitions of done
(`docs/design-system.md` §7, `docs/i18n-guidelines.md` §7) on the surfaces the feature
touched — a hardcoded string or a `toFixed()` in the DOM is a defect, not a nitpick.

### 4. Executor "noticed" items

Read the **Noticed** section of every report in `reports/`. An executor almost always sees
the integration bug before integration does, and mentions it in passing. Reproduce each
one; the ones that hold up become defect reports.

## Writing it up

### `test-report.md`

What was run (commit hash under test, commands, locales, environments), the per-test-case
table, the conformance results, and the list of defect IDs raised. Enough that someone can
re-run exactly what you ran.

### One defect report per failure

`docs/stories/<CODE>-<slug>/defects/DEF-<NNN>.md`, using `assets/defect-report-template.md`.
Number sequentially within the run. Each report has:

- a **reproducible case** — exact request/steps, exact inputs, locale, starting state.
  "Login is broken" is not a defect report.
- **expected vs actual**, with the expectation traced to the story AC, test case, or
  contract clause it comes from
- a **severity**: `S1` blocks the story's primary path; `S2` a story AC fails; `S3`
  wrong but the path completes; `S4` cosmetic / non-blocking
- a **classification** (your best judgment; the instructor confirms):
  - **slice** — one slice's code doesn't do what its brief and the contract say → phase 4
  - **contract / partition** — the slices each did what they were told and it still doesn't
    fit together, or a behavior has no single owner → phase 1 as an amendment
  - **spec** — the story or AC is wrong or incomplete; the code may be doing the right
    thing → back to `user-story-documentation`
- the **suspected slice(s)** and why

Two failures with the same root cause are one defect. Two defects that happen to fail the
same test case are two reports.

## Triage and hand-off (gate 3)

Once every failure is filed: sort by severity, confirm classifications with the instructor
for anything not clearly `slice`, and hand the `slice` defects to phase 4. Anything
classified `contract / partition` or `spec` goes to the instructor, and the slices it
touches are *not* handed to a fixer in the meantime — fixing against a contract that is
about to change is wasted work.

Confirm your own diff is empty outside the story folder. If it isn't, you fixed something. Undo
it and file a defect instead.

## Re-testing after a fix pass

Each fix brief in `fixes/` names the defect it closes and the regression test it added.
For each:

1. Re-run the failing test case(s) from the defect report. Pass → mark the defect
   **closed** in `test-report.md`. Fail → reopen it with the new observation; it goes back
   to phase 4 with the extra evidence.
2. Re-run every story test case that touches the slice(s) the fix changed — a fix in one
   place regresses another more often than not.
3. Run the end-to-end path once per loop, both locales.
4. Confirm the fixer's diff stayed inside its fix-brief allowlist and touched no contract,
   spec or other slice's file. If it did, that's a new `contract / partition` defect
   regardless of whether the fix "works".

Closure is yours to grant, not the fixer's. A fixer's passing regression test is evidence
you weigh, not a verdict.

## Exit criterion

Phase 3 is done — and the run is done — when all of these hold at once:

- no open `S1` or `S2` defects
- every story test case is pass (not blocked)
- contract conformance checks pass
- the end-to-end path completes in both locales
- every open `S3`/`S4` is listed in `test-report.md` with an explicit "deferred" decision
  from the user or instructor, not just left open

Then update the story's status in `docs/stories/index.md`, and the document registry in
`docs/shared-contract.md` §6 if new specs were added — this is the one place you write
outside the story folder, and it is documentation, not source.
