# Frontend architecture

**Status:** authoritative for client-side work.
**Companions:** [`backend.md`](backend.md) · [`shared-contract.md`](shared-contract.md) ·
[`design-system.md`](design-system.md) · [`i18n-guidelines.md`](i18n-guidelines.md)

> The design system and i18n guidelines are **not optional companions to this document** —
> they are binding on every line of UI written. This document covers structure and
> boundaries; those two cover what the result must look like and say.

---

## 1. Target vs. now

| | Target | Now (roadmap Phase 1–2) |
|---|---|---|
| Structure | Multiple independently-buildable frontends behind a shell | **One Vite + React SPA**, `apps/web/` |
| Boundaries | Remote modules, independently deployable | **Feature folders** under `src/features/` |
| Sharing | Published shared packages, versioned | Direct imports from `src/styles/`, `src/i18n/` |

As with the backend, the boundaries are real now as **folder and import rules**, even
though every feature ships in one bundle. A feature folder that respects them can be
extracted into its own deployable later; one that reaches sideways into another feature's
internals cannot.

**Do not stand up module federation, a shell app, or a separate deployable during Phase 1.**
The equivalent discipline today is the import rule in §3.

---

## 2. Structure

```
apps/web/
├── index.html
├── vite.config.ts
├── tailwind.config.js          # maps utilities onto tokens.css variables; defines no values
├── postcss.config.js
└── src/
    ├── main.tsx
    ├── styles/                 # SHARED — tokens.css (source of truth), globals.css
    ├── i18n/                   # SHARED — config, Intl helpers, en-US + vi-VN catalogs
    ├── api/                    # SHARED — generated typed client from the OpenAPI contract
    ├── app/                    # SHELL — routing, layout, auth context, error boundaries
    └── features/               # FEATURE SLICES — one folder per bounded capability
        ├── routines/           # checklist UI, occurrence view, ad-hoc add
        ├── nutrition/          # food log, camera capture, angle guide, estimate review
        └── analytics/          # dashboard, trend and volume charts
```

Each folder under `features/` is a **slice**: a candidate micro-frontend. It maps to a
backend domain (`routines` ↔ routine, `nutrition` ↔ nutrition, `analytics` ↔ metrics), which
is what keeps a feature's server dependency legible and its ownership assignable to one
executor.

---

## 3. Boundaries — the import rule

This is the whole architecture, expressed as four lines:

1. A slice may import from `styles/`, `i18n/`, `api/`, and its own subtree.
2. A slice may **never** import from another slice's subtree. Not a component, not a hook,
   not a type, not a constant.
3. The shell (`app/`) may import slices. Slices may **never** import the shell.
4. Anything two slices genuinely both need moves *up* into a shared folder — it does not get
   imported sideways.

Rule 2 is the one that gets violated first, usually with a reasonable-sounding excuse
("it's just a formatting helper"). The reason it's absolute: a sideways import is invisible
coupling that compiles fine today and becomes a build failure the moment the slices are
split into separately-deployed bundles. The cost of the violation is paid months later by
someone who didn't create it.

When two slices need to interact at runtime, they do it through the shell or through
shared state — never by importing each other. Cross-slice communication is a shell concern.

---

## 4. Shared dependency discipline

Once slices are independently built (target state), duplicated framework copies are the
classic and confusing failure: two Reacts means hooks throw, two routers means navigation
silently no-ops, two i18n instances means one of them never gets the locale.

So, from now:

- **React, the router, and the i18n instance are singletons.** There is exactly one of each
  in the running application, whatever the build topology.
- Their versions are agreed across all slices and pinned; a slice does not unilaterally
  upgrade a shared dependency.
- A slice-specific dependency (a chart library used only by `analytics`) is that slice's
  business and needs no coordination.

The distinction to check before adding a dependency: *would two copies of this in memory
cause a problem?* If yes, it's shared and coordinated. If no, it's local and free.

---

## 5. Styling and content — non-negotiable

Full rules in [`design-system.md`](design-system.md) and
[`i18n-guidelines.md`](i18n-guidelines.md). The parts a slice author breaks most often:

- **`src/styles/tokens.css` is the single source of truth for every design value.** Tailwind
  consumes those custom properties; it defines none of its own. Never write a raw hex, px,
  or ms into a component — if the value doesn't exist as a token, add the token with a
  comment saying what it's for.
- **No user-facing string is hardcoded** — not labels, errors, `aria-label`, `alt`, `title`,
  or `placeholder`. Every one resolves through a translation key present in **both**
  `en-US.json` and `vi-VN.json` (`scripts/check_i18n_parity.py` enforces this).
- **Every number renders through `Intl.NumberFormat` with the active locale**, in the mono
  face with tabular figures, carrying its unit. `vi-VN` inverts the separators — `2.620`
  means 2620 in Vietnamese — so a raw `toFixed()` in the DOM is a real defect here, not a
  formatting nitpick.
- Layouts are designed for Vietnamese, which runs 10–30% longer than English and stacks
  diacritics vertically. An English-only mockup is not a finished design.

## 6. Talking to the backend

- The typed client in `src/api/` is **generated from the OpenAPI spec** referenced by
  [`shared-contract.md`](shared-contract.md) — not hand-written, and not a place to add
  behavior. Hand-editing it means the next regeneration silently reverts you.
- A slice calls only the endpoints its own domain owns, plus shared ones (auth, user).
  Reaching for another domain's endpoint is the frontend expression of the same boundary
  violation §3 forbids.
- **Two-step LLM writes are a UI obligation, not just an API shape.** `estimate` → user sees
  and edits the draft → `confirm`. A slice must never auto-confirm an estimate on the user's
  behalf; the whole point is that a person approved the number.
- Aggregation endpoints return pre-bucketed series. If a component is doing analytics
  arithmetic in the browser, either the endpoint is wrong or the component is — resolve it
  at the contract, not with a local calculation.
- Estimated values carry their confidence marker (`.u-estimated`). A number without its
  uncertainty is not a shortcut, it's a false claim about measurement precision.
