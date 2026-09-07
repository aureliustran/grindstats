---
name: multi-agent-code-execution
description: Entry point for building a feature with multiple parallel agents. Runs a four-phase pipeline with a gate between each phase — instruction (one instructor writes the contract and partitions work), execution (many executors build disjoint slices), testing (independent testers verify against the story and the contract, never fixing), and bug fixing (fixers resolve triaged defects with regression tests, then hand back to testing). Each phase is its own skill; this one says which phase you are in, what must be true before the next one starts, and who is not allowed to do what. Use whenever a task spans backend and frontend at once, touches more than one service or micro-frontend, is big enough to want parallel agents, or when the user asks to "parallelize", "fan out", "split this across agents", or hands over a story/spec to implement end-to-end. Also use when a previous multi-agent run drifted, conflicted on files, shipped untested slices, or fixed bugs by silently widening scope.
---

# Multi-agent code execution — the pipeline

Four phases, four skills, three gates. Every phase is done by a **different role**, and the
roles do not overlap: the agent that partitioned the work does not build it, the agent that
built a slice does not sign off on testing it, the agent that finds a defect does not fix
it, and the agent that fixes it does not decide it is fixed.

| # | Phase | Skill | Who | Produces |
|---|-------|-------|-----|----------|
| 1 | Instruction | `multi-agent-instruction` | exactly one instructor | frozen contract, slice partition, one brief per executor |
| 2 | Execution | `multi-agent-execution` | many executors, one per slice | code **and the automated tests for its acceptance-criteria scenarios** inside each slice's allowlist, structured completion reports |
| 3 | Testing | `multi-agent-testing` | testers who did not write the slice under test | AC-coverage audit, contract conformance, end-to-end runs; test report and defect reports — **no code changes** |
| 4 | Bug fixing | `multi-agent-bug-fixing` | fixers, one per triaged defect cluster | fixes with regression tests, then back to phase 3 |

Phases 3 → 4 → 3 loop until the exit criterion in `multi-agent-testing` is met. Nothing
else loops: a defect that turns out to be a contract or partition problem goes back to phase
1 as an amendment, not sideways into a fixer's hands.

## Why the phases are separate skills

The expensive failures in parallel agent work are coordination failures, and each one lives
at a *boundary* between activities, not inside one:

- Building while still designing → two agents invent incompatible APIs. (Gate 1 stops it.)
- Testing by the agent that built → "it passes my tests" while slices disagree with each
  other. (Gate 2 and the role rule stop it.)
- Fixing during testing → the tester patches what it finds, the fix lands outside any
  allowlist, and nobody knows what was actually verified. (The tester's no-code rule stops it.)
- Fixing by redesigning → a "bug fix" quietly changes a contract that three finished slices
  depend on. (The fixer's scope rule and the escalate-to-phase-1 path stop it.)

Putting the activities in one skill invites an agent to slide from one to the next without
noticing. Separate skills with explicit gates make the slide a visible decision.

## Tests are built in development, verified in testing

The automated tests — server unit/integration tests and client unit/component tests — are
**development work**, written in phases 1 and 2, not something the testing phase adds
afterwards. They are derived from the story's acceptance criteria: every Gherkin scenario
in `acceptance-criteria.md` is assigned to a slice in phase 1 and gets an automated test in
phase 2, on whichever side of the seam (server, client, or both) the scenario's *Then*
clause is observable.

Phase 3 does not write those tests. It checks that they exist, that they assert what the
scenario says, and that they pass — and then does the work automated tests can't: contract
conformance across slices and the end-to-end path. A scenario with no automated test is a
defect in its own right, filed against the slice that owned it (or against the partition,
if none did).

## When this applies

Use it when work spans backend and frontend simultaneously, touches more than one service
or slice, or is large enough that serial execution wastes time. **Don't** use it for a
change that lives inside one file or one slice — the coordination overhead exceeds the
parallelism gain, and a single agent doing it directly is strictly better.

Honest test: if you cannot name at least two slices with genuinely disjoint file ownership,
this pipeline is the wrong tool. Say so and do the work directly.

## Where a run lives

A run's artifacts live **in the story folder**, next to the spec they implement, so the four
phases (and anyone auditing later) find them in one place:

```
docs/stories/<CODE>-<slug>/
  story.md, acceptance-criteria.md, test-cases.md, diagram.md   the spec (stable)
  contract.md            phase 1: this feature's frozen contract, or the excerpt of
                         docs/shared-contract.md it implements against
  plan.md                phase 1: slice list, ownership map, order, dependencies, run number
  briefs/<slice-id>.md   phase 1: one executor brief per slice
  amendments/            phase 2: amendment requests + instructor decisions
  reports/<slice-id>.md  phase 2: executor completion reports
  test-report.md         phase 3: what was run, against what, results, per run
  defects/DEF-<NNN>.md   phase 3: one defect report each
  fixes/DEF-<NNN>.md     phase 4: fix brief + fixer report per defect cluster
```

The spec files are stable across runs; everything else is a record of building. When a
feature needs a second run (a re-partition, or a later phase of the roadmap), number it in
`plan.md` and `contract.md` ("frozen for execution run 2") and keep the earlier run's
reports — don't overwrite the record.

## Gates

A gate is a checklist that the **next** phase's agent verifies before starting. It is not the
previous phase's agent declaring itself done — that's the whole point.

### Gate 1 — instruction → execution

- [ ] `docs/shared-contract.md` (and any linked spec) covers every surface this feature
      crosses, in machine-checkable form where possible, and is **frozen** for the run
- [ ] `plan.md` lists every slice with an owner, an exclusive write allowlist, a read-only
      list, and done-criteria containing runnable commands
- [ ] No path appears in two allowlists (grep the plan; don't eyeball it)
- [ ] Migrations, if any, are one slice owned by one executor
- [ ] Slice order is explicit: what runs first, what runs in parallel, what waits on what
- [ ] One brief per slice exists and is self-contained (executable with no memory of the
      planning conversation)
- [ ] `plan.md` has an **AC coverage map**: every scenario in `acceptance-criteria.md` →
      the slice(s) that must cover it with an automated test, and on which side (server /
      client / both). No scenario is unassigned; no scenario is "covered" only by the
      end-to-end pass in phase 3

### Gate 2 — execution → testing

- [ ] Every slice has a completion report in `reports/` with the done-criteria commands
      actually run and their real output
- [ ] No open amendment requests — each one has an instructor decision, and every slice the
      decision names as "re-verify" has been re-verified (finished ones included)
- [ ] Each executor's diff stays inside its allowlist (check the diff, not the report)
- [ ] Every scenario in the AC coverage map has the automated test(s) the brief asked for,
      named after the scenario, and the report shows them passing — a slice with code but
      no scenario tests is not done
- [ ] The whole thing builds from a clean checkout: backend compiles, frontend type-checks,
      generated files are current (`scripts/gen_audit_model.py --check`,
      `scripts/check_i18n_parity.py`)

If gate 2 fails, the failure goes **back to phase 2** (re-brief the slice) — not forward
to testing with a note. Testing an unbuildable tree produces defect reports about the
build, which is noise.

### Gate 3 — testing → bug fixing

- [ ] `test-report.md` exists and names what was run: which story test cases
      (`docs/stories/<CODE>/test-cases.md`), which contract conformance checks, which
      end-to-end paths
- [ ] Every failure is a defect report in `defects/` with a reproducible case, expected vs
      actual, and the tester's *suspected* slice — a suspicion, not an assignment
- [ ] Defects are triaged: severity set, and each one classified as **slice bug** (goes to
      phase 4), **contract or partition bug** (goes to phase 1 as an amendment), or
      **spec bug** (goes back to the story via `user-story-documentation`)
- [ ] The tester changed no source files (check the diff — it should be empty outside
      the story folder)

### Gate 3' — bug fixing → testing (the loop)

- [ ] Every fix has a regression test that failed before the fix and passes after, and the
      fix brief records both runs
- [ ] Each fixer's diff stays inside the allowlist named in its fix brief
- [ ] No fix touched the contract, a spec, or another slice's files — if one needed to,
      it should have been escalated, and the run goes back to phase 1 instead
- [ ] The tree still builds clean, generated files are still current

Then phase 3 re-runs — at minimum the failed cases plus everything in the affected
slices, and the full end-to-end path once per loop. Exit when `multi-agent-testing`'s
exit criterion holds.

## Role separation — the rules that don't bend

1. **The instructor writes no feature code.** If it starts implementing, it loses the shape
   of the whole and the partition quality degrades.
2. **An executor tests its own slice only to the extent of its done-criteria.** That is
   self-verification, not the testing phase. It does not write the test report.
3. **A tester never edits source.** Not a one-line fix, not a flaky test, not a typo in a
   comment. It reports; someone else fixes. The moment a tester edits, the record of what
   was verified against what is gone.
4. **A fixer fixes the defect it was handed, inside the allowlist it was handed.** It does
   not fix adjacent things it notices (report them as new defects), and it does not touch
   contract, spec or another slice's paths (escalate instead).
5. **Nobody tests their own fix into "closed".** The fixer's regression test is evidence;
   closure is the tester's call in the next phase-3 pass.

For Claude subagents these are usually different subagent invocations with different
prompts. For mixed teams (Claude + other tools + people) they are different assignments.
The same person or agent may wear two hats across *different* runs; never within one run
on the same slice.

## Starting a run

1. Read this file. Decide whether the honest test above passes.
2. Confirm the story folder exists (run `user-story-documentation` first if it doesn't); the run's
   artifacts go in it.
3. Load `multi-agent-instruction` and proceed as the instructor.
4. Each subsequent phase's agent loads its own skill and checks its entry gate first.
