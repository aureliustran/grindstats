# Executor rules: frontend / micro-frontend slices

Read this if your slice touches `apps/web/`. It covers the mistakes that only surface when
parallel slices are merged, or when slices are later split into independent deployables.

Full architecture: `docs/frontend.md`. Visual and content rules, which are binding:
`docs/design-system.md`, `docs/i18n-guidelines.md`.

## 1. The import rule is your boundary

- Import only from `styles/`, `i18n/`, `api/`, and your own slice subtree.
- **Never import from another slice's subtree** — not a component, hook, type, or constant,
  no matter how small or how obviously reusable.
- The shell (`app/`) may import you. You may never import the shell.
- Something two slices genuinely both need moves *up* into a shared folder — which is a
  contract change, so it's an escalation, not something you do inside your slice.

A sideways import compiles fine today and becomes a build failure the moment slices are
split into separate bundles. You will not be the one who pays for it, which is exactly why
the rule has to be absolute rather than case-by-case.

## 2. Don't build the target topology

If the current roadmap phase says one SPA, build inside one SPA. Do not introduce module
federation, a shell application, or a separate deployable because the architecture doc
mentions them. Folder discipline is how the boundary is expressed right now.

## 3. Shared dependencies are singletons

React, the router, and the i18n instance exist exactly once in the running app. Do not add,
upgrade, or pin a different version of any of them inside your slice — two Reacts means
hooks throw, two routers means navigation silently no-ops, two i18n instances means one
never receives the locale.

A dependency only your slice uses (a chart library in `analytics`, say) is yours and needs
no coordination. The test: *would two copies of this in memory cause a problem?*

## 4. Tokens and strings — the rules broken most often

- **Never write a raw value.** No hex colors, no `px`, no `ms`. Every design value resolves
  to a token in `src/styles/tokens.css`. If the token you need doesn't exist, that's a
  shared-file change → escalate rather than inlining a value.
- **Never hardcode a user-facing string.** Including `aria-label`, `alt`, `title`, and
  `placeholder`. Every string is a translation key present in **both** `en-US.json` and
  `vi-VN.json`.
- **Adding translation keys is a shared-file edit.** Two slices appending to the same
  catalog is a merge conflict. If your brief doesn't explicitly grant you the catalogs,
  list the keys you need in your completion report instead of adding them.
- **Every number goes through `Intl.NumberFormat`** with the active locale, in the mono face
  with tabular figures, carrying its unit. `vi-VN` inverts separators — `2.620` is 2620 in
  Vietnamese, not 2.62 — so a raw `toFixed()` in the DOM is a real defect in a calorie app.
- Check your screen at `?lng=vi`. Vietnamese runs 10–30% longer than English and stacks
  diacritics vertically; a layout sized to English text clips.

## 5. Talking to the backend

- Use the generated client in `src/api/`. **Never hand-edit it** — the next regeneration
  reverts you, silently.
- If the endpoint you need doesn't exist in the generated client, the contract is
  incomplete. **Escalate.** Do not hand-roll a `fetch` against an endpoint you assume
  exists; that's how a frontend ships against an API that was never agreed.
- Call only your own domain's endpoints, plus shared ones (auth, user).
- **Two-step LLM writes are your obligation too**: `estimate` → the user sees and edits the
  draft → `confirm`. Never auto-confirm on the user's behalf. The point is that a person
  approved the number.
- Aggregation endpoints return pre-bucketed series. If you're doing analytics arithmetic in
  a component, either the endpoint is wrong or the component is — that's a contract question,
  not something to solve locally.

## 6. Before reporting done

- The slice builds, and type-checks with no new errors
- Renders correctly in light **and** dark theme
- Renders correctly at `?lng=vi` with no clipped or overflowing text
- No raw design values, no hardcoded strings
- Keyboard reachable, visible `:focus-visible`, ≥44px targets
- `python3 scripts/check_i18n_parity.py` passes
- You touched nothing outside your allowlist — check your own diff before reporting
