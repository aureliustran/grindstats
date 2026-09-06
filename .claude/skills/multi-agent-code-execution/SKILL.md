---
name: multi-agent-code-execution
description: Plan and execute a feature across multiple parallel agents split into two phases — a single instructor agent that writes the contract and partitions the work, then multiple executor agents (Claude subagents, other AI agents, or people) that build disjoint slices against it. Use whenever a task spans backend and frontend at once, touches more than one service or micro-frontend, is big enough to want parallel agents, or when the user asks to "parallelize", "fan out", "split this across agents", "use subagents to build", or hands over a story/spec to implement end-to-end. Also use when a previous multi-agent run drifted, conflicted on files, or produced mismatched API assumptions — the partitioning and contract-freeze rules here are the fix.
---

# Multi-agent code execution

Two phases, two kinds of agent. **Phase 1 is one instructor** that decides the shape of the
work and never writes feature code. **Phase 2 is many executors** that write code inside
boundaries the instructor drew and are not permitted to redraw them.

The split exists because the expensive failures in parallel agent work are not coding
failures. They are coordination failures — two agents editing one file, two agents
inventing incompatible versions of the same API, two agents each numbering a migration
`0007`. Every one of those is decided before any code is written, which is exactly what
Phase 1 is for.

## When this applies

Use it when work spans backend and frontend simultaneously, touches more than one service
or slice, or is simply large enough that serial execution wastes time. **Don't** use it for
a change that lives inside one file or one slice — the coordination overhead exceeds the
parallelism gain, and a single agent doing it directly is strictly better.

Honest test: if you cannot name at least two slices with genuinely disjoint file ownership,
this skill is the wrong tool. Say so and do the work directly.

---

# Phase 1 — Instructor (exactly one agent)

The instructor produces a plan, a contract, and a brief per executor. **It writes no feature
code.** If the instructor starts implementing, it stops being able to hold the whole shape
in view, and the partition quality — the only thing that makes Phase 2 safe — degrades.

## Step 1: Read before deciding

Read, in this order, and don't skip to partitioning:

1. The feature spec — a story under `docs/stories/<CODE>-<slug>/` if one exists (see the
   `user-story-documentation` skill), or whatever the user provided
2. `docs/shared-contract.md` — what already crosses boundaries, and the standing policies
3. `docs/backend.md` and/or `docs/frontend.md` — whichever domains this touches

**If those three root documents don't exist, create them first.** They are the substrate
this entire skill runs on; partitioning without them means each executor invents its own
idea of the boundaries. Seed them from whatever the repo already establishes rather than
writing empty templates — an architecture doc that says "TODO" gets ignored, and an ignored
document is worse than an absent one because it looks like coverage.

## Step 2: Check the roadmap phase

Before designing anything, confirm what the project is *supposed* to have built by now
(`CLAUDE.md`, the blueprint roadmap). Both root architecture docs describe a target
architecture that deliberately does not exist yet.

This matters more here than in single-agent work: an instructor that partitions along
*target* boundaries will hand executors briefs telling them to stand up services, message
queues, or module federation that the current phase doesn't call for. Partition along the
boundaries that exist **as rules** today — packages and folders — not the ones that will
exist as deployments later.

## Step 3: Write or amend the contract — before any partitioning

Whatever crosses a boundary in this feature gets written down first: endpoints, payloads,
error codes, events, and the interfaces between domains.

Express it in whatever a machine can check — OpenAPI, JSON Schema, TypeScript types, Go
interfaces — and put prose only where a schema can't carry the meaning (ordering,
idempotency, policy, reasoning). A prose restatement of a schema drifts from it silently;
a generated client that stops compiling does not.

This comes before partitioning because **the contract is what makes the slices independent**.
Until the seam is pinned down, every slice is potentially coupled to every other one and no
honest partition is possible.

## Step 4: Partition into slices with disjoint ownership

This is the step the whole skill exists for. A slice is a unit of work with:

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

**Shared foundations run first, not in parallel.** Design tokens, shared types, the
generated API client, i18n catalog keys — anything multiple slices consume — is either
already in place or is its own slice that completes before the consumers start. A slice that
must wait for another slice is a dependency, and dependencies are sequenced, not raced.

Then order the slices explicitly: what must be serial, what may run in parallel, and what
each waits on.

## Step 5: Write a brief per slice

One self-contained file per executor, using `assets/executor-brief-template.md`.

Write each brief so that **an agent with no memory of this conversation, and no access to
the other briefs, can execute it correctly.** That constraint is not hypothetical: executors
may be Claude subagents with fresh context, a different AI tool entirely, or a person
picking up a ticket next week. A brief that assumes shared context silently becomes a brief
only you can execute.

Concretely, each brief carries its own copy (or a precise link plus the relevant excerpt) of
the contract it implements against. "See the shared contract" is not enough — say which
endpoints, which payloads, which error codes.

Include the stop condition verbatim in every brief:

> If you find the contract wrong, ambiguous, or insufficient: **stop work on that surface,
> write an amendment request, and do not proceed.** Do not fix the contract yourself, and do
> not implement around it.

## Step 6: Dispatch

For Claude subagents, spawn them in parallel — one Task per slice, each given its brief as
the prompt. Launch them in the same turn rather than sequentially, so they actually run
concurrently.

For non-Claude executors, hand over the brief files; they're written to be tool-agnostic.

Dispatch only slices whose dependencies have completed. A slice waiting on the migration
slice does not start "optimistically."

---

# Phase 2 — Executors (many agents)

Each executor implements one slice. The rules are few and absolute, because an executor
cannot see the other executors and therefore cannot judge when an exception is safe.

1. **Write only inside your allowlist.** Not one file outside it, however obvious the fix.
   A file outside your list belongs to someone else who is editing it right now.
2. **Never edit the contract.** It is frozen for the duration. If it's wrong, stop and
   escalate (`assets/amendment-request-template.md`).
3. **Never edit another slice's files to make your slice work.** That's the coordination
   failure this whole structure prevents, arriving through the back door.
4. **Implement what the brief says, not what you'd have designed.** If the brief is wrong,
   that's an escalation, not a silent improvement. An executor that "improves" the design
   unilaterally breaks the assumptions three other executors are building on.
5. **Verify before reporting done.** Run the commands in the brief's done-criteria. "It
   should work" is not a completion report.
6. **Report structurally**: what you changed, what you verified and how, what you could not
   do, and anything you noticed that's outside your scope (don't fix it — report it).

Domain-specific rules an executor must follow, read the one that matches the slice:

- **Frontend / micro-frontend slices** → `references/frontend-microfrontend.md`
- **Backend / distributed-service slices** → `references/backend-distributed.md`

Both are short and cover the mistakes that only show up when parallel work is merged.

---

# Phase 1 (again) — Integration

The instructor closes the loop. This is not a formality; it's where coordination failures
that survived the process get caught.

1. **Contract conformance, not just green tests.** Every slice's tests can pass while the
   slices disagree with each other — that's precisely the failure mode. Check the generated
   client still compiles against the spec, event payloads validate on both sides, and
   documented error codes match returnable ones. See `docs/shared-contract.md` §5.
2. **Run the thing end to end.** Build, migrate, start it, exercise the actual user path
   from the story. Parallel slices that each work in isolation frequently fail on first
   contact.
3. **Reconcile the reports.** Read what each executor said it couldn't do or noticed out of
   scope. This is where a real problem usually surfaces first — an executor almost always
   sees the integration bug before integration does, and mentions it in passing.
4. **Handle failures by re-scoping, not patching.** If a slice came back wrong, decide
   whether to re-brief it or absorb the work — don't quietly fix it yourself and leave the
   brief describing something that didn't happen. The briefs are the record of what was
   built.
5. **Update the docs the work changed**: the contract, the story's status, and the document
   registry in `docs/shared-contract.md` §6 if new specs were added.

If an amendment happened mid-flight, verify every affected slice — including ones that
finished *before* the amendment. Those are the ones nobody thinks to re-check, and they're
the ones now silently building against a superseded contract.
