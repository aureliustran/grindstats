---
name: multi-agent-instruction
description: Phase 1 of the multi-agent pipeline — the single instructor agent that reads the spec and architecture docs, writes or amends the shared contract, partitions the feature into slices with disjoint file ownership, writes one self-contained brief per executor, and dispatches them in dependency order. Writes no feature code. Use when starting a multi-agent run (after reading `multi-agent-code-execution`), when an executor files an amendment request that needs a decision, or when a defect from testing turns out to be a contract or partition problem rather than a slice bug. Also use when a previous run had two agents editing one file, incompatible API assumptions, or colliding migration numbers — those are partitioning failures fixed here.
---

# Phase 1 — Instruction

You are the instructor. There is exactly one of you per run. You produce a plan, a frozen
contract and a brief per executor, and you **write no feature code**. If you start
implementing, you stop being able to hold the whole shape in view, and the partition quality
— the only thing that makes the later phases safe — degrades.

You are also the phase everything escalates back to: amendment requests during execution,
and contract/partition-class defects during testing. Both come to you because they change
what *everyone else* is building against, and only the role holding the whole picture can
decide that safely.

Pipeline context, run folder layout and gates: `.claude/skills/multi-agent-code-execution/SKILL.md`.

## Step 1: Read before deciding

Read, in this order, and don't skip to partitioning:

1. The feature spec — a story under `docs/stories/<CODE>-<slug>/` if one exists (see the
   `user-story-documentation` skill), or whatever the user provided. Read its
   `test-cases.md` too: phase 3 will test against it, so the slices you cut had better
   add up to something that can pass it.
2. `docs/shared-contract.md` — what already crosses boundaries, and the standing policies
3. `docs/backend.md` and/or `docs/frontend.md` — whichever domains this touches
4. `docs/audit-and-errors.md` if the feature emits audit events or returns error codes —
   those come from `libs/auditmodel/model.yaml`, and adding to that file is a slice in
   itself (see step 4)

**If the three root architecture documents don't exist, create them first.** They are the
substrate the pipeline runs on; partitioning without them means each executor invents its
own idea of the boundaries. Seed them from whatever the repo already establishes rather
than writing empty templates — a doc that says "TODO" gets ignored, and an ignored document
is worse than an absent one because it looks like coverage.

## Step 2: Check the roadmap phase

Before designing anything, confirm what the project is *supposed* to have built by now
(`CLAUDE.md`, the blueprint roadmap). Both root architecture docs describe a target
architecture that deliberately does not exist yet.

An instructor that partitions along *target* boundaries hands executors briefs telling them
to stand up services, message queues or module federation that the current phase doesn't
call for. Partition along the boundaries that exist **as rules** today — packages and
folders — not the ones that will exist as deployments later.

## Step 3: Write or amend the contract — before any partitioning

Whatever crosses a boundary in this feature gets written down first: endpoints, payloads,
error codes, events, and the interfaces between domains.

Express it in whatever a machine can check — OpenAPI, JSON Schema, TypeScript types, Go
interfaces — and put prose only where a schema can't carry the meaning (ordering,
idempotency, policy, reasoning). A prose restatement of a schema drifts from it silently; a
generated client that stops compiling does not.

Then **freeze it** for the run. Write the freeze into `plan.md` with the commit hash of the
contract. From here on the contract changes only through the amendment protocol in
`docs/shared-contract.md` §4.

This comes before partitioning because **the contract is what makes the slices
independent**. Until the seam is pinned down, every slice is potentially coupled to every
other and no honest partition is possible.

## Step 4: Partition into slices with disjoint ownership

This is the step the phase exists for. A slice is a unit of work with:

- **an owner** (one executor)
- **an exclusive write allowlist** — the paths it and only it may modify
- **a read-only context list** — what it needs to see but must not change
- **verifiable done-criteria**, including commands that can actually be run

**Every file path is owned by exactly one slice.** If two slices need to write the same
file, that is not a partition — it's a conflict you've scheduled for later. Resolve it now
by one of:

- moving the shared file into a third slice that runs **before** the others depend on it
- serializing the two slices instead of parallelizing them
- restructuring so the shared file doesn't need two authors (usually the right answer, and
  usually reveals a boundary that was wrong to begin with)

Never "assign it to both and hope they don't collide." Two agents writing one file produces
a result where the last writer silently erases work that appeared to succeed.

**Migrations get their own single-owner slice.** Migration files are globally ordered by
number; two parallel agents will both create `0007_...` without either noticing, and one
silently loses. If a feature needs schema changes in two domains, that is *one* slice, owned
by *one* executor, that the others depend on. See `docs/backend.md` §3.

**`libs/auditmodel/model.yaml` and the server/frontend i18n catalogs are single-owner
too.** New enums, events, error codes and their catalog entries are one foundation slice
that runs first; the generated Go/TS lands with it. Consumers import from generated code and
never edit the model.

**Shared foundations run first, not in parallel.** Design tokens, shared types, the
generated API client, catalog keys — anything multiple slices consume — is either already
in place or is its own slice that completes before the consumers start. A slice that must
wait for another slice is a dependency, and dependencies are sequenced, not raced.

Then order the slices explicitly in `plan.md`: what must be serial, what may run in
parallel, and what each waits on. Include the ownership map as a flat list of paths → slice
so gate 1 can grep it for duplicates.

## Step 5: Write a brief per slice

One self-contained file per executor at `docs/stories/<CODE>-<slug>/briefs/<slice-id>.md`,
using `assets/executor-brief-template.md`.

Write each brief so that **an agent with no memory of this conversation, and no access to
the other briefs, can execute it correctly.** Executors may be Claude subagents with fresh
context, a different AI tool entirely, or a person picking up a ticket next week. A brief
that assumes shared context silently becomes a brief only you can execute.

Each brief carries its own copy (or a precise link plus the relevant excerpt) of the
contract it implements against. "See the shared contract" is not enough — say which
endpoints, which payloads, which error codes.

Each brief names the story test cases its slice is expected to make pass. That is how the
tester in phase 3 knows which slice to suspect, and how the executor knows what "done"
means beyond its own unit tests.

Include the stop condition verbatim in every brief:

> If you find the contract wrong, ambiguous, or insufficient: **stop work on that surface,
> write an amendment request, and do not proceed.** Do not fix the contract yourself, and do
> not implement around it.

## Step 6: Dispatch

For Claude subagents, spawn them in parallel — one Task per slice, each given its brief as
the prompt and told to load `multi-agent-execution`. Launch them in the same turn so they
actually run concurrently.

For non-Claude executors, hand over the brief files; they're written to be tool-agnostic.

Dispatch only slices whose dependencies have completed. A slice waiting on the migration
slice does not start "optimistically."

Then step back. Your next job is not to integrate — that is phase 3's — but to be available
for escalations.

## Handling escalations

### Amendment requests (from phase 2)

An executor found the contract wrong, ambiguous or insufficient and stopped. Read the
request (`multi-agent-execution/assets/amendment-request-template.md`), decide, record the
decision in the same file under `amendments/`, and:

1. Update the contract if accepted or modified — you are the only role allowed to.
2. List **every** slice that builds against the changed surface, including ones that have
   already finished. Finished slices are the ones nobody re-checks, and they're the ones
   that end up silently built against a superseded contract.
3. Notify each affected executor, not just the one who asked. For a finished slice, that
   means re-dispatching it with a re-verify brief.
4. Bump the frozen contract hash in `plan.md`.

### Contract / partition / spec defects (from phase 3)

The tester classified a defect as not-a-slice-bug. Confirm the classification — testers
suspect, they don't assign — then:

- **Contract bug**: treat as an amendment you raise yourself. Same steps as above.
- **Partition bug** (two slices each half-own a behavior, or a shared file turned out to
  need two authors): re-partition the affected area into a new slice with a new brief and
  dispatch it through phase 2. Do not hand it to a fixer — a fixer works inside one existing
  allowlist, and this problem is that the allowlists were wrong.
- **Spec bug** (the story itself is wrong or incomplete): send it back through
  `user-story-documentation` to amend the story, then re-check whether the contract and
  partition still hold.

## What you never do

- Write feature code, tests or fixes. Not to "unblock" someone, not because it's small.
- Let a slice start before its dependencies have reported done.
- Accept "it should work" as a completion report. Gate 2 will reject it anyway; better to
  reject it at your desk.
- Quietly fix a slice that came back wrong and leave the brief describing something that
  didn't happen. Re-brief it. The briefs are the record of what was built.
