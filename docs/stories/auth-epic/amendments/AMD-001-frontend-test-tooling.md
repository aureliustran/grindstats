# AMD-001 — Frontend test tooling does not exist; `python3` is not the Python on this machine

**Raised by:** the frontend executor, during brief review, **before wave 1 was dispatched**
**Decided by:** the instructor, 2026-09-07
**Status:** accepted (both parts)
**Class:** **partition / plan defect** — not a contract defect. No endpoint, payload, error code,
key, table, interface or type changes, so **the freeze commit does not move**.

One line of `contract.md` is edited: §9's conformance list says `py` rather than `python3`. That
is a correction to how a check is *invoked*, not to what it asserts, and it is recorded here so
the edit is not mistaken later for an unrecorded change to a frozen document.

---

## 1. What the plan said

`plan.md` §3, dependency table:

> | Frontend | nothing new; Vitest + RTL are already configured | — |

Both frontend briefs list `cd apps/web && npx vitest run` under Done-when, and
`fe-auth-flows` requires a React Testing Library component test
(`screen.getByText("auth.unverified.banner")`) for `UnverifiedBanner.test.tsx`.

## 2. What is actually true

Verified against the repo:

- `apps/web/package.json` declares **no** `vitest`, `jsdom`, `@testing-library/react` or
  `@testing-library/user-event`, in dependencies or devDependencies
- `node_modules/@testing-library` does not exist
- `apps/web/vite.config.ts` has **no `test` block** — no `environment: "jsdom"`, no setup file
- there is not a single `*.test.ts(x)` anywhere under `apps/web/src`

`docs/frontend.md` §7 describes the tooling in the present tense — "Vitest (it shares
`vite.config.ts`) with `@testing-library/react`… `jsdom` environment" — and the instructor took
that as a statement of fact rather than of intent. It is the frontend's *testing convention*,
written before any frontend test existed. **That is the instructor's error**, and it is the
specific failure mode phase 1 is supposed to catch: an architecture document describing a target
was read as describing the present, which is the same mistake `backend.md` §1 and `frontend.md`
§1 both open with a warning about.

Second, smaller finding: the Done-when criteria say `python3 scripts/gen_audit_model.py --check`
and `package.json`'s `check-i18n` script calls `python3`. On this machine `python3` and `python`
are Microsoft Store execution-alias stubs; the real interpreter is **`py` (Python 3.13.3)**. So
the generator and the parity script are runnable after all — the briefs' hedge ("Python may be
unavailable, explain its absence") was wrong in a way that would have let a required check get
skipped with a blessed excuse.

## 3. Why an executor could not fix this themselves

`apps/web/package.json` and `package-lock.json` are assigned exclusively to `be-wiring`
(`plan.md` §3), precisely so that no two slices collide on a manifest. A frontend executor
installing its own test runner would have violated its allowlist — so it stopped and escalated,
which is the protocol working exactly as intended.

`apps/web/vite.config.ts`, `apps/web/tsconfig*.json` and a test setup file were owned by
**nobody**, which is the actual partition hole: a file every frontend slice needs changed and no
slice may write is a file that blocks the wave.

## 4. Decision

**Accepted, both parts.** Changes, all to plan and briefs — the contract is untouched:

1. `be-wiring`'s **pre-step** now installs the frontend test tooling as well as the Go modules,
   and configures it: `vitest`, `jsdom`, `@testing-library/react`, `@testing-library/user-event`,
   `@testing-library/jest-dom`, the `test` block in `vite.config.ts` (jsdom environment,
   globals, setup file, `passWithNoTests` so a green run with zero tests is not a false failure),
   a `test` script in `package.json`, and the `vitest/globals` type entry in `tsconfig.app.json`.
2. `apps/web/vite.config.ts`, `apps/web/tsconfig*.json` and `apps/web/src/test/**` are added to
   `be-wiring`'s allowlist and to the ownership map. They were previously unowned.
3. The pre-step's done-criteria now include a **proof the tooling works**: a throwaway test that
   renders a component and asserts on it, run and then deleted, reported with its actual output.
   "Installed the packages" is not evidence that `screen.getByText` works in this repo's
   TypeScript and jsdom setup.
4. `fe-auth-client` and `fe-auth-flows` change from "Depends on: none / …" to depend on the
   **pre-step** explicitly. `fe-auth-client` was documented as startable immediately; it was not.
5. Every `python3` invocation in the briefs and in `package.json`'s `check-i18n` script becomes
   **`py`**, and the "explain why you skipped it" hedge is removed. The check is runnable, so it
   is required.

## 5. Affected slices — including ones that had already been briefed

Per `shared-contract.md` §4 step 3, every slice building against the changed surface is listed,
not only the one that raised it. **No slice had started**, so no re-verification is needed:

| Slice | Change |
|---|---|
| `be-wiring` | pre-step scope grows; allowlist gains four paths; new done-criteria |
| `fe-auth-client` | `Depends on` now names the pre-step; `python3` → `py` |
| `fe-auth-flows` | `Depends on` now names the pre-step; `python3` → `py` |
| `plat-audit-model` | `python3` → `py`; the generator check is now mandatory, not excusable |
| all others | unaffected |

## 6. Not accepted — and why it was right to raise anyway

The executor's **Blocker 2** (that `fe-auth-flows` cannot start until `fe-auth-client` and
`plat-audit-model` clear gate 2) is not a defect. That is the plan working as designed:
`fe-auth-flows` is wave 2 and `plan.md` §5 already sequences it there. Nothing changes. Raising
it was correct — an executor cannot tell a deliberate dependency from an oversight without
asking, and the cost of asking is one paragraph.

## 7. Follow-up outside this run

`docs/frontend.md` §7 still describes the tooling in the present tense and will mislead the next
reader the same way. Once the pre-step lands, that section becomes accurate — but the "Target vs.
now" table in §1 should grow a tooling row so this class of mistake is visible where the document
already warns about it. **Not done in this run**: `docs/` is frozen for executors, and a doc edit
is not a slice. Filed here so it is not lost.
