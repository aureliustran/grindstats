---
name: multi-agent-execution
description: Phase 2 of the multi-agent pipeline — the executor role. Load when you have been handed an executor brief (`docs/stories/<CODE>-<slug>/briefs/<slice-id>.md`) and are implementing one slice of a feature alongside other agents you cannot see. Covers the absolute rules (write only inside your allowlist, never edit the contract, never touch another slice's files, implement the brief not your preference), how to turn each assigned acceptance-criteria scenario into an automated server or client test as part of the slice, the stop-and-escalate protocol when the contract is wrong, the domain-specific rules for frontend/micro-frontend and backend/distributed-service slices, and the structured completion report. Not for planning (that's `multi-agent-instruction`), not for testing other slices (`multi-agent-testing`), not for fixing defects found after execution (`multi-agent-bug-fixing`).
---

# Phase 2 — Execution

You are an executor. You implement **one slice**, described by a brief someone else wrote,
while other executors you cannot see implement theirs. Because you cannot see them, you
cannot judge when an exception to the rules below is safe — so there are no exceptions.

Pipeline context and gates: `.claude/skills/multi-agent-code-execution/SKILL.md`.

## Before you start — check gate 1 yourself

Your brief should give you all of this. If it doesn't, stop and ask the instructor rather
than guessing:

- [ ] a slice ID, a story/spec link, and the domain
- [ ] an explicit write allowlist (paths, not descriptions)
- [ ] the contract excerpt you implement against, not just a pointer
- [ ] done-criteria with runnable commands
- [ ] the acceptance-criteria scenarios your slice must cover with automated tests, with
      side, kind and location
- [ ] the dependencies you wait on, and confirmation they have completed

A brief missing any of these is a phase-1 defect. Starting anyway means you'll invent the
missing part, and your invention won't match the executor next to you.

## The rules

1. **Write only inside your allowlist.** Not one file outside it, however obvious the fix.
   A file outside your list belongs to someone else who is editing it right now. If your
   slice genuinely cannot be completed without a change outside the allowlist, that is an
   escalation (below), not a reason to widen the list yourself.
2. **Never edit the contract.** `docs/shared-contract.md`, linked specs, schemas, the
   OpenAPI file, `libs/auditmodel/model.yaml`, the i18n catalogs — frozen for the run
   unless your allowlist names them (it will only for a foundation slice). If it's wrong,
   stop and escalate.
3. **Never edit another slice's files to make your slice work.** That's the coordination
   failure this whole structure prevents, arriving through the back door.
4. **Implement what the brief says, not what you'd have designed.** If the brief is wrong,
   that's an escalation, not a silent improvement. An executor that "improves" the design
   unilaterally breaks the assumptions three other executors are building on.
5. **The scenario tests are part of the slice.** Your brief lists acceptance-criteria
   scenarios with a side and a test kind; each one becomes an automated test you write,
   inside your allowlist, as part of the same deliverable as the code. See "Tests from
   acceptance criteria" below. Code without its scenario tests is an unfinished slice.
6. **Verify before reporting done.** Run the commands in the brief's done-criteria,
   including every scenario test. "It should work" is not a completion report. This is
   *self-verification*, not the testing phase — it earns you the right to report done, not
   a sign-off.
7. **Report structurally, in the run folder.** What you changed, what you verified and
   how, what you could not do, and anything you noticed outside your scope (don't fix it —
   report it). Template at the bottom of your brief; write it to
   `docs/stories/<CODE>-<slug>/reports/<slice-id>.md`.

## Stop and escalate

If you find the contract wrong, ambiguous or insufficient for your task:

> **Stop work on the affected surface. Write an amendment request using
> `assets/amendment-request-template.md` to `docs/stories/<CODE>-<slug>/amendments/`. Do not
> proceed on that surface.**

Do not fix the contract yourself, and do not implement around it. Other slices are being
built against the current version right now; a local workaround produces two
implementations that each believe they conformed, and nothing detects the mismatch until
runtime.

Continue any part of your slice that doesn't depend on the disputed shape. Say in your
report exactly which parts you finished and which you left, so the instructor knows the
actual state.

The same applies to a brief that is internally inconsistent, or that asks for something
your allowlist can't reach: escalate to the instructor with the specific conflict. Don't
resolve it by picking the interpretation you like.

## Tests from acceptance criteria

Each scenario row in your brief is a Given/When/Then statement from the story. Turn it into
a test the same way, on the side your brief says:

- **Name the test after the scenario**, verbatim or nearly — `TestLogin_WrongPasswordGivesGenericFailure`,
  `it("wrong password gives a generic failure without revealing which factor failed")`.
  Phase 3 audits coverage by matching scenario titles to test names; an unrecognisable
  name reads as a missing test.
- **Given** → the test's arrangement: fixtures, seeded rows, mocked API responses. Take
  preconditions and test data from the linked `test-cases.md` rows rather than inventing
  them, so the automated test and the manual test case agree.
- **When** → one action: the request, the render + interaction, the function call.
- **Then** → assert exactly what the scenario says is observable on *your* side, and
  nothing about the other side's internals. A server test asserts status, body shape,
  error code, cookie flags, audit row; a client test asserts what renders, which
  translation key is used, what is disabled, what the API client was called with. Where the
  scenario says two responses are identical (enumeration protection), assert byte-equality —
  don't assert each one separately.
- **Test against the contract, not against the other side.** Client tests mock the API at
  the contract's shapes (`apps/web/src/api/*.types.ts`, generated types). Server tests call
  the handler with contract-shaped requests. If you need the other slice's code to make your
  test pass, the seam is wrong — escalate.
- **Locale-sensitive *Then*s are tested in both `en-US` and `vi-VN`** on the server
  (rendered message) and, on the client, by asserting the key rather than the text.
- Follow the placement and tooling conventions in `docs/backend.md` §7 and
  `docs/frontend.md` §7. If the right location is outside your allowlist, escalate — don't
  put the test somewhere odd to stay inside it.

Scenarios marked `e2e` in your brief are the exception: you write the spec file in the
location your allowlist names, and it runs in phase 3 with both sides live.

## Domain-specific rules

Read the one that matches your slice before writing code. Both are short and cover the
mistakes that only show up when parallel work is merged:

- **Frontend / micro-frontend slices** → `references/frontend-microfrontend.md`
- **Backend / distributed-service slices** → `references/backend-distributed.md`

Cross-cutting rules that apply to every slice regardless of domain:

- Error codes, audit events and enums come from generated code
  (`libs/auditmodel/generated.go`, `apps/web/src/api/generated/audit.ts`). If the one you
  need doesn't exist, that's an amendment — the model is single-owner.
- No user-facing string is hardcoded; every one resolves through a catalog key. A missing
  key in the frontend catalog is yours to add **only** if `apps/web/src/i18n/locales/` is
  in your allowlist; otherwise escalate.
- Numbers go through `Intl.NumberFormat`; design values go through `tokens.css`. See
  `CLAUDE.md` for the three rules most often broken by accident.

## Reporting done

Your report is the input to gate 2 and to the tester in phase 3. It is read by agents who
did not watch you work. Make it checkable:

1. **Changed:** files touched, and what each change does. The tester will diff this
   against your allowlist.
2. **Verified:** commands run and their *actual* output — paste it, don't paraphrase.
   One line per scenario in your brief: scenario → test name → pass/fail.
3. **Could not do:** anything blocked, and why. An amendment you filed and are waiting on
   belongs here.
4. **Noticed:** problems outside your scope. Report them; don't fix them. The tester will
   turn them into defect reports if they hold up.

Then stop. You do not merge, integrate, run the end-to-end path, or write the test report.
Those are phase 3.
