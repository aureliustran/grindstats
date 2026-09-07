# GrindStats design system

**Status:** authoritative. When this document and an individual component disagree, this
document wins — fix the component. When this document and the published blueprint
disagree, the blueprint wins for *architecture*; this document wins for *visual and
interaction rules*.

**Who this is for:** the next person or agent to write UI in this repo. Read §1 to
understand *why* the rules exist, then work from §3 onward. Rules whose reasoning you
understand survive refactors; rules you follow blindly get deleted by the next person.

---

## 1. Philosophy

### The work is the aesthetic

GrindStats is about doing unglamorous things repeatedly and measuring what happens. The
interface should feel like that: stark, plain, unhurried, faintly severe. It should not
feel like a wellness app. No gradients-as-mood, no illustration of smiling people, no
motivational confetti when you complete a set. The user did 100 push-ups; the interface's
job is to record it accurately, not to congratulate them.

This is a design constraint with teeth: **when in doubt, remove.** A screen that looks
slightly too plain is correct. A screen that looks designed is suspect.

### Color is data, not decoration

The product is mostly numbers and charts — trend lines, volume against landmarks, intake
against target. If the interface itself is colorful, color stops meaning anything when a
chart needs it. So the UI is monochrome by default, with exactly one accent for action.
Every other color in the system is **semantic**: it encodes a data meaning (below/above a
landmark, deficit/surplus, low confidence) and appears only where that meaning applies.

If you find yourself picking a color because a section "needs some visual interest", that
is the signal to stop.

### Numbers are the interface

Most of what a user reads here is numeric. That has concrete consequences that override
normal web typography habits:

- Numbers render in the mono face with **tabular figures**, always. Proportional digits
  make columns ragged and make a live-updating value jitter as digits change width.
- A number never appears without its unit, and never at a precision the measurement
  doesn't support. `82.4 kg` is honest; `82.4174 kg` is a lie about a bathroom scale.
- **A number without its confidence is a lie.** The blueprint's standing caveats (§13)
  aren't legal boilerplate — BMR is ±10%, consumer body-fat readings are ±3–4 points. Any
  value the system estimates rather than measures must carry a visible marker, and the
  marker must be explainable on hover/tap. This is a design rule because it is the fastest
  way for this product to lose trust.

### The voice is deadpan

Copy is second person, present tense, and direct. It states what is true and what will
happen. No exclamation marks, no hype, no "Great job!". The one permitted flourish is the
dry parenthetical aside — the joke is that the regimen is absurd and you do it anyway.

> Good: "You logged 14 hard sets for chest this week. Your range is 12–20."
> Good: "10km. Every single day. (Nobody said it was interesting.)"
> Bad: "Amazing work crushing your goals this week! 🔥"

### Accessible by construction, not by audit

Contrast, focus visibility, target size and keyboard operability are decided when a
component is written, not fixed later. A component that fails these is unfinished, the
same way an untested endpoint is unfinished.

---

## 2. How the two styling systems fit together

We use **Tailwind and plain global CSS together**, in a Vite + React app. That only works
without drift if one of them owns the values. So:

> **`tokens.css` is the single source of truth for every design value. Tailwind consumes
> those CSS custom properties; it does not define its own scale.**

The practical division:

| Use plain CSS (`src/styles/*.css`) for | Use Tailwind utilities in markup for |
|---|---|
| Token definitions (`tokens.css`) | Layout, spacing, sizing on specific elements |
| Base element styles and reset (`globals.css`) | Typography scale application |
| Theme switching (`:root` / `[data-theme]` blocks) | Responsive variants, state variants (`hover:`, `focus-visible:`) |
| `@keyframes`, complex selectors, `:has()`, print styles | One-off composition of existing tokens |
| Anything a designer or agent should be able to read as *rules* | Anything that is purely "this instance looks like this" |

**Never hardcode a raw value in a component.** No `#0a0a0a`, no `13px`, no
`margin-top: 17px`. If a value you need doesn't exist as a token, that's a design decision
— add the token (and a line in this doc explaining what it's for), don't inline it. This
is the rule that keeps a codebase from accumulating forty-one slightly different greys.

### Where files live (Vite structure)

```
apps/web/
├── index.html
├── vite.config.ts
├── tailwind.config.js          # reads CSS variables; defines no raw values
├── postcss.config.js
└── src/
    ├── main.tsx
    ├── styles/
    │   ├── tokens.css          # ← source of truth: colors, type, space, motion
    │   ├── globals.css         # ← reset + base element styles + Tailwind layers
    │   └── <component>.css     # only when a component genuinely needs plain CSS
    ├── i18n/
    │   ├── index.ts            # react-i18next init + Accept-Language detection
    │   └── locales/
    │       ├── en-US.json      # source of truth for copy
    │       └── vi-VN.json      # must have every key en-US has
    ├── app/                    # routing, layout, auth context
    ├── features/               # routines/, nutrition/, analytics/
    └── api/                    # generated/typed API client
```

---

## 3. Tokens

Full definitions with inline commentary live in `apps/web/src/styles/tokens.css`. That
file is written to be read — it explains each scale's intent, not just its values. The
summary below is the map; the CSS file is the territory.

### Color

Two ramps and nothing else. `--ink-*` is the foreground ramp, `--paper-*` the background
ramp. In dark theme they swap roles rather than being redefined ad hoc, which is why every
component that uses tokens themes correctly for free.

- **Accent** (`--accent`): exactly one. It marks the primary action on a screen. If two
  things on a screen are accent-colored, one of them is wrong.
- **Semantic** (`--ok`, `--warn`, `--danger`, `--muted-data`): reserved for data meaning
  and system state. Never used to decorate.
- **Confidence** (`--low-confidence`): the marker treatment for estimated values. See §1.

Contrast floors are non-negotiable: **4.5:1 for body text, 3:1 for large text and for any
non-text element that carries meaning** (chart strokes, borders that separate content,
icon-only controls). Both themes. If a token pair fails, the token is wrong.

### Type

Three faces, each with a job:

| Role | Face | Used for |
|---|---|---|
| Display | Big Shoulders Display | The regimen hero, section statements. Condensed, heavy, loud. |
| Body | IBM Plex Sans | Everything a person reads as prose or UI labels. |
| Mono | IBM Plex Mono | **Every number**, plus code, IDs, and tabular data. Tabular figures on. |

**Before adopting or changing any face, verify it ships the Vietnamese subset** — see §6.
The display face is the risky one: condensed display fonts frequently omit `ơ ư đ` and
stacked diacritics like `ế ộ ữ`, and a hero that renders as tofu boxes in Vietnamese is a
launch blocker, not a polish item.

The scale is a modular ramp defined in `tokens.css`. Use scale steps; don't interpolate
between them.

### Space

A 4px base scale. Everything — padding, gaps, margins, component heights — is a multiple.
This is what makes independently-written components sit together without fiddling.

### Motion

Motion is functional: it explains a state change (a modal arriving, a value updating) or
it doesn't exist. Durations are short (`--motion-fast` 120ms, `--motion-base` 200ms);
nothing decorative loops.

**Every animation must be disabled under `prefers-reduced-motion: reduce`.** `globals.css`
carries a blanket rule for this, but a component that animates via JS must check the media
query itself — the CSS rule can't reach it.

---

## 4. Component rules

- **Focus is always visible.** Never remove an outline without replacing it with something
  that meets 3:1 against its background. Use `:focus-visible`, not `:focus`, so mouse users
  don't see rings they didn't ask for.
- **Interactive targets are at least 44×44px** of hit area, even when the visual is
  smaller. Pad the target, don't grow the icon.
- **Every interactive element has an accessible name** — visible text, or `aria-label` when
  the control is icon-only.
- **Dialogs trap focus**, close on `Escape` and backdrop click, and return focus to the
  control that opened them. This is specced concretely in the landing page story's
  acceptance criteria; it applies to every dialog we ever build.
- **Loading states must terminate.** Every async control has a defined success, failure and
  timeout appearance. A spinner with no failure branch is a bug.
- **Empty states say what to do next**, not just "No data". A new user's dashboard is
  empty for weeks — that's the normal case here, not an edge case, and the 21-day floor
  before any prescription (blueprint §13) means the empty state has real work to do
  explaining why nothing is being recommended yet.
- **Disabled controls explain themselves.** A disabled button with no reason shown is a
  dead end.

---

## 5. Imagery and IP

- The landing page silhouette (`features/landing/Silhouette.tsx`) is an original
  GrindStats asset — hand-authored, not traced from any photo or character.
- Record provenance next to any original asset added to the repo (who made it, from what).
- The landing page also uses licensed/third-party media where it serves the design:
  the hero backdrop (`public/media/saitama-vs-garou.mp4`), the Pricing side art
  (`public/media/download.jpg`), and a live Pinterest pin embed
  (`features/landing/Quote.tsx`). No further sign-off is needed to swap or extend this
  media going forward.

---

## 6. Internationalization (en-US, vi-VN)

Full rules live in `docs/i18n-guidelines.md`. The design-relevant parts:

- **English is not the design target; it's one of two.** Vietnamese UI strings run
  roughly 10–30% longer than their English equivalents. Never size a container, button, or
  nav bar to fit the English string. Test every layout in `vi-VN` before calling it done.
- **Vietnamese stacks diacritics vertically** (`ế`, `ộ`, `ữ`), which needs more line-height
  headroom than English. The line-height tokens are set for Vietnamese; don't tighten them
  because an English mockup looks airy.
- **Number formatting is locale-specific and this is a numbers product.** Vietnamese uses
  `.` for thousands and `,` for decimals — `2.620` in `vi-VN` means two thousand six
  hundred and twenty, not 2.62. Every displayed number goes through `Intl.NumberFormat`
  with the active locale. Never `toFixed()` straight into the DOM.
- **Units are a user preference, not a locale.** Locale sets the *default* (vi-VN → metric)
  but the user's explicit metric/imperial choice always wins.

---

## 7. Rules an agent can check itself against

Before considering any UI work done, verify each of these. They're phrased so the answer is
observable, not a matter of taste.

1. No raw color, size, or duration values appear in components — every value resolves to a token.
2. Any new token is defined in `tokens.css` with a comment saying what it's for.
3. The screen renders correctly in light and dark theme.
4. The screen renders correctly in `en-US` **and** `vi-VN`, with no clipped or overflowing text.
5. No user-facing string is hardcoded; every one comes from a translation key.
6. Every key used exists in **both** `en-US.json` and `vi-VN.json`.
7. Every displayed number is formatted through `Intl` with the active locale, in the mono face with tabular figures, and carries its unit.
8. Any estimated value carries its confidence marker.
9. Body text meets 4.5:1 contrast; large text and meaningful non-text elements meet 3:1 — in both themes.
10. Every interactive element is reachable and operable by keyboard, with a visible `:focus-visible` style and a ≥44px target.
11. All motion is disabled under `prefers-reduced-motion: reduce`.
12. Every async action has a defined failure and timeout appearance.
13. Copy is deadpan, second person, no exclamation marks, no congratulation.

If you can't satisfy one of these, say so explicitly in your handoff rather than shipping
past it quietly — a known gap is manageable, a silent one isn't.
