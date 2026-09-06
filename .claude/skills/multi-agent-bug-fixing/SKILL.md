---
name: multi-agent-bug-fixing
description: Phase 4 of the multi-agent pipeline — the fixer role. Load when handed a triaged defect report (`docs/stories/<CODE>-<slug>/defects/DEF-NNN.md`) classified as a slice bug. Covers turning a defect into a scoped fix brief with its own allowlist, reproducing before touching code, writing the regression test that fails first, fixing the root cause inside scope, and reporting for retest — the fixer never closes its own defect. Also covers what a fixer must not do: widen scope to adjacent problems, touch the contract, spec or another slice's files (escalate to `multi-agent-instruction` instead), or "fix" a test to pass. Not for defects classified as contract/partition or spec — those go back to phase 1.
---

# Phase 4 — Bug fixing

You are a fixer. You resolve **one defect** (or one cluster the instructor grouped because
they share a root cause), inside a scope written down before you start, and you hand the
result back to a tester for closure. You don't decide it's fixed; you produce evidence that
it is.

The pressure in this phase is always toward scope creep — the defect is in front of you,
the adjacent problems are obvious, and the contract clause that would make the fix clean is
right there. Resist all three. Each is a coordination failure with a friendly face.

Pipeline context and gates: `.claude/skills/multi-agent-code-execution/SKILL.md`.

## Before you start — check gate 3

- [ ] The defect is triaged: severity set, classification **confirmed** by the instructor
      as `slice`. If it says `contract-or-partition` or `spec`, or the instructor hasn't
      confirmed, it isn't yours yet — send it back.
- [ ] A fix brief exists at `fixes/DEF-<NNN>.md` (`assets/fix-brief-template.md`) with an
      explicit write allowlist. If the instructor didn't write one, write it yourself
      **first**, from the defect report and the original slice brief, and get it
      acknowledged before editing anything. The allowlist is normally a *subset* of the
      original slice's — never a superset.
- [ ] You did not write the slice under fix — or, if you did (allowed when no one else is
      available), you've said so in the brief, and the retest will be done by someone else.

## The method

### 1. Reproduce first

Run the defect report's **Reproduce** steps exactly. If it doesn't reproduce, do not start
changing code on a theory. Record what you saw and send it back to the tester as
"cannot reproduce" with your environment details. A fix for a bug you can't see is a change
you can't verify.

### 2. Write the regression test before the fix

Add a test that encodes the defect's **Expected** and fails against the current code for the
reason the defect describes. Run it; paste the failing output into the fix brief. This is the
half of the evidence that a passing test alone can't give: proof the test actually detects
the bug.

Put it in the test location the original slice owns. If the right place is outside your
allowlist, that's an escalation, not a reason to put the test somewhere odd.

### 3. Find the root cause, then fix it — inside scope

Fix the cause the reproduction points at, not the symptom. A `nil` check that stops the
crash while the wrong value keeps flowing is a symptom fix; it will come back as a different
defect next loop.

Inside scope means:

- **only files in the fix brief's allowlist**
- **no contract, spec, schema, model or catalog changes** — if the correct fix needs one,
  stop: this was misclassified, and it goes back to phase 1 as an amendment
  (`multi-agent-execution/assets/amendment-request-template.md`, raised by you)
- **no changes to another slice's files**, even if the bug is "really" theirs — report it
  as a new defect with your evidence and let the instructor route it
- **no changing the test's expectation to make it pass.** If you believe the expectation
  is wrong, the defect is a `spec` defect; say so and send it back.

### 4. Run everything the fix could have touched

- the new regression test → passes; paste the output
- the original slice's done-criteria commands from its brief
- the story test cases the slice brief named
- `python3 scripts/gen_audit_model.py --check`, `python3 scripts/check_i18n_parity.py`,
  the build and type-check

A fix that turns one red test green and two others red is not done.

### 5. Report — then stop

Fill in the fixer's section of the fix brief: root cause in one or two sentences, files
changed, the regression test and both its runs (failing before, passing after), the
full-suite results, and anything you noticed and left alone. Set the defect's status to
`fixed-pending-retest`. Do **not** mark it closed.

Then stop. Retest is phase 3, done by a tester, on the whole affected area — not by you, on
the one test you wrote.

## Things you notice along the way

You will see other problems. Every one of them goes into the **Noticed** section of your
report as a candidate defect with whatever evidence you have. None of them gets fixed in
this pass, for two reasons: it's outside the allowlist that was written for *this* defect,
and a fix nobody asked for is a fix nobody will retest.

The one exception is a second defect the instructor explicitly clustered with yours in the
fix brief because they share a root cause. That's in scope by definition.

## When to escalate instead of fix

Stop and go to the instructor (`multi-agent-instruction`) when any of these turns out to be
true:

- the fix needs a change to the contract, a spec, `libs/auditmodel/model.yaml`, or either
  i18n catalog (unless the catalog is in your allowlist)
- the root cause is in a file another slice owns
- two slices each implement half the behavior and neither is wrong on its own
- the defect's **Expected** contradicts the story or AC when you read them closely

Each of those means the defect was a `contract-or-partition` or `spec` class wearing a
`slice` label. Fixing it as a slice bug would work locally and break the slices that
finished against the current contract.
