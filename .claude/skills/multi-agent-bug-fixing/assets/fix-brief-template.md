# Fix brief: DEF-<NNN>

<!--
Written by the instructor when routing the defect to phase 4, or by the fixer before
touching any code if the instructor didn't. Either way it is acknowledged before the first
edit. The allowlist is a SUBSET of the original slice's — a fix that needs more than the
slice owned is a partition problem, not a fix.
-->

**Defect:** `docs/stories/<CODE>-<slug>/defects/DEF-<NNN>.md` (S<n>, classified `slice`,
confirmed by <instructor>)
**Also closes (same root cause):** DEF-<..> or "none"
**Original slice:** <slice-id> — brief at `briefs/<slice-id>.md`
**Fixer:** <id> — **wrote the original slice:** yes | no
**Load skill:** `multi-agent-bug-fixing`

## You own (exclusive write access for this fix)

- `path/to/file.go`
- `path/to/file_test.go`

## Do not touch

- The contract, specs, `libs/auditmodel/model.yaml`, i18n catalogs (frozen — escalate)
- `<path>` — owned by <other slice>
- Test expectations for the failing case(s) — if they look wrong, it's a `spec` defect

## Done when

- [ ] Reproduced first (output pasted below)
- [ ] Regression test added at `<path>`, fails before fix, passes after (both runs below)
- [ ] Original slice done-criteria still pass: `<commands from slice brief>`
- [ ] Story test cases TC-<..> pass
- [ ] `python3 scripts/gen_audit_model.py --check` and `python3 scripts/check_i18n_parity.py` pass
- [ ] Diff is inside the allowlist above
- [ ] Defect status set to `fixed-pending-retest` — **not** closed

---

## Fixer report

**Reproduced:** yes — <paste> | no — <environment, what was observed>

**Root cause:** <one or two sentences — the cause, not the symptom>

**Changed:**
- `<file>` — <what and why>

**Regression test:** `<path>::<name>`
- Before fix: <paste failing output>
- After fix: <paste passing output>

**Full verification:** <commands and real output>

**Noticed, not fixed:** <candidate defects with evidence, or "none">

**Escalated:** <amendment / new defect filed, or "none">
