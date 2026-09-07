# Executor brief: <slice-id>

<!--
Written by the instructor (Phase 1). Must be executable by an agent with NO memory of the
planning conversation and NO access to the other briefs — a Claude subagent with fresh
context, a different AI tool, or a person picking this up next week. If a section needs
knowledge you only have from the planning discussion, that knowledge belongs in this file.
-->

**Slice ID:** <e.g. AUTH-002-be-login-endpoint>
**Story / spec:** <link, e.g. docs/stories/AUTH-002-auth-login/story.md>
**Domain:** backend | frontend | migration
**Depends on:** <slice IDs that must complete first, or "none — may start immediately">
**Load skill:** `multi-agent-execution` (`.claude/skills/multi-agent-execution/SKILL.md`)
**Read first:** `docs/shared-contract.md` + <`docs/backend.md` or `docs/frontend.md`> +
<`.claude/skills/multi-agent-execution/references/backend-distributed.md` or `.../frontend-microfrontend.md`>

## Task

<What to build, in outcome terms. Specific enough that two competent implementers would
produce compatible results; not so prescriptive that it's just code in prose.>

## You own (exclusive write access)

<Explicit path allowlist. Only these paths. Be precise — a directory means that directory.>

- `path/to/thing/`
- `path/to/specific-file.go`

## Read-only context

<What to read to do the job, but must not modify.>

- `docs/shared-contract.md`
- <other paths>

## Do not touch

<Files another slice owns, or shared files frozen for this run. Name the ones this slice
would most plausibly want to edit — that's where the collision would happen.>

- `<shared file>` — owned by <slice-id> this run
- Any contract, spec, or schema file (frozen — see stop condition)

## Contract you implement against

<The actual relevant excerpt, not just a pointer. Endpoints, payload shapes, error codes,
event names and payloads. If it's long, link the schema AND state which parts apply.>

## Acceptance-criteria scenarios this slice covers with automated tests

<Copied from the AC coverage map in `plan.md`. Each row is a test you write as part of this
slice — the code is not done without it. The scenario text is the test's name; the linked
test-case rows in `test-cases.md` give preconditions and data. Phase 3 verifies these exist
and pass, and suspects this slice when a scenario fails.>

| Scenario | Test cases | Side | Test kind | Test location (inside your allowlist) |
|---|---|---|---|---|
| <scenario title verbatim from acceptance-criteria.md> | TC-<..> | server / client | <handler / unit / component / hook / integration / e2e> | `<path>` |

## Done when

<Verifiable criteria plus the commands to prove them. "It works" is not a criterion.>

- [ ] Every scenario in the table above has an automated test, named after the scenario,
      that asserts its *Then* clause — and passes
- [ ] <any behavioral criterion not captured by a scenario>
- [ ] Builds and type-checks: `<command>`
- [ ] Tests pass: `<command>`
- [ ] <domain-specific checks from the relevant reference file>
- [ ] Nothing modified outside the allowlist above (check your own diff)

## Stop condition — read this before starting

If you find the contract wrong, ambiguous, or insufficient for this task:

> **Stop work on the affected surface. Write an amendment request using
> `.claude/skills/multi-agent-execution/assets/amendment-request-template.md`. Do not proceed on that surface.**

Do not fix the contract yourself, and do not implement around it. Other slices are being
built against the current version right now; a local fix produces two implementations that
each believe they conformed, and nothing detects the mismatch until runtime.

Continue any part of your slice that doesn't depend on the disputed shape.

## Report back

Write the report to `docs/stories/<CODE>-<slug>/reports/<slice-id>.md`.

1. **Changed:** files touched, and what each change does
2. **Verified:** which commands you ran and their results — not what you expect to pass.
   Include a scenario → test → result line for every row in the scenarios table.
3. **Could not do:** anything blocked, and why
4. **Noticed:** problems outside your scope. **Report them, don't fix them** — a fix outside
   your allowlist collides with whoever owns that file.
