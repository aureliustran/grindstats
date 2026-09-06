# Executor rules: backend / distributed-service slices

Read this if your slice touches server code or the database. It covers what goes wrong when
parallel slices are merged, and what makes code extractable into a real service later.

Full architecture: `docs/backend.md`. Cross-boundary shapes: `docs/shared-contract.md`.

## 1. Domain ownership is your boundary

Your slice owns one domain (or an explicitly listed set). Within it you're free; outside it
you are not, even though the compiler will let you and everything is in one binary today.

You may **not**:
- read or write another domain's tables, directly or through a shared repository type
- import another domain's internal packages
- assume another domain's storage layout, including through a shared struct

You **may** reach another domain only through a seam declared in `docs/shared-contract.md`
— an exported interface or an event. If the seam you need isn't declared, that's an
escalation, not an improvisation.

The schema namespace makes this checkable: a query naming a schema your package doesn't own
is a violation you can find with grep. Check your own diff for one before reporting done.

## 2. Don't build the target topology

If the roadmap phase says one binary, work in `services/monolith/internal/<domain>/`. Do not
create a new service, add RabbitMQ, or introduce a service-to-service HTTP call because the
architecture doc describes them. The in-process seam *is* the correct implementation for the
current phase — written so it can be extracted later without a rewrite.

## 3. Migrations — the highest-risk thing you can touch

**If your brief does not explicitly assign you migrations, you do not write one.** Migration
files are globally ordered by number. Two parallel agents both create `0007_...`, and one is
silently lost or the merge leaves a half-applied schema. Only one slice owns migrations for
any unit of work.

If you own them:

- **Additive first (expand/contract).** Add the new column, backfill, write to both shapes;
  drop the old shape in a *later, separate* migration once nothing reads it. A migration
  that adds one column and drops another breaks every server instance still running the old
  code during a rolling deploy.
- **Never edit a merged migration.** Correct a mistake with a new one. Editing history means
  databases that already applied it never get the fix.
- Provide a rollback, or state in the file why it's irreversible.
- Don't apply migrations from application startup; migration is a deliberate separate step.
- Keep seed data in its own migration, distinguishable from schema changes.

## 4. Events

- **Assume at-least-once delivery** — your consumer must be idempotent. Processing the same
  event twice must produce the same result as once. Carry an event ID; make the write
  conditional on not having seen it.
- **Publish facts, not commands.** `training.logged`, not `update_analytics`. A command
  implies you know who acts on it, which re-couples what the event decoupled.
- **Payloads are self-contained.** A consumer must never have to call back to interpret an
  event — that recreates the synchronous dependency you removed.
- **Arrival order isn't guaranteed.** If ordering matters, make it derivable from the
  payload (timestamps, versions), not from sequence.

## 5. API surface

- Versioned `/api/v1/...`; the universal error envelope `{ "error": { "code", "message" } }`
- The effective user ID comes from the token, never a client-supplied parameter
- Aggregation endpoints return pre-bucketed series — the client never does the arithmetic
- **Two-step writes for anything LLM-generated**: the `estimate`/`propose` call must not
  persist. Only `confirm` writes.
- Route comments are the source of the OpenAPI spec the frontend generates its client from.
  An undocumented or wrongly-documented route means the frontend builds against a fiction —
  treat the comment as part of the deliverable, not a nicety.

## 6. Computation rules

- **"Services compute, the LLM narrates."** Every user-visible number is produced by
  deterministic code and handed to the model pre-computed. Never ask a model to do
  arithmetic.
- Formulas live once in `libs/physiology/` — import, never reimplement. A second copy of a
  BMR formula is a second thing to get wrong.
- Cite the source for any new formula, and keep the population-level / not-clinical caveat
  attached in code comments and user-facing copy.

## 7. Before reporting done

- Builds; tests pass; linter clean
- Nothing outside your allowlist was modified — check your own diff
- No query touches a schema your domain doesn't own
- New/changed routes have accurate comments, and the OpenAPI spec regenerates cleanly
- If you own migrations: they apply on a clean database *and* on one at the previous
  revision, and they're additive
- Every documented error code is actually returnable; every code you return is documented
