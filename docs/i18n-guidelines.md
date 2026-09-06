# Internationalization guidelines (en-US, vi-VN)

**Status:** authoritative for all user-facing text. Locale support is `en-US` (source of
truth) and `vi-VN`. Adding a third locale later should require no code changes — only a
new catalog — and these rules are written to keep that true.

**Library:** `react-i18next`. **Locale selection:** `Accept-Language` auto-detection only —
there is deliberately no user-facing language switcher yet (see §5 for the consequences,
which you should read before assuming this is finished).

---

## 1. The one rule everything else follows from

**No user-facing string is written into a component.** Not a label, not a button, not an
error message, not an `aria-label`, not a `placeholder`, not a `title` attribute, not the
`alt` text on an image. Every one of them resolves through a translation key.

The reason this has to be absolute rather than "mostly": a codebase with 95% of its strings
externalized is not 95% translated, it's broken in Vietnamese in five places nobody can
find without clicking through every screen. Partial externalization is worse than none,
because it looks done.

Strings that are *not* user-facing — log messages, error codes, test fixtures, developer
comments — stay in English in code and are never translated.

---

## 2. Key naming and catalog structure

Keys are namespaced by feature, then by area, then by element:

```
<feature>.<area>.<element>
```

```json
{
  "landing": {
    "hero": {
      "regimen_pushups": "100 PUSH-UPS",
      "regimen_situps": "100 SIT-UPS",
      "regimen_squats": "100 SQUATS",
      "regimen_run": "10KM RUN",
      "punchline": "EVERY SINGLE DAY",
      "aside": "(Nobody said it was interesting.)",
      "cta_primary": "Get started"
    },
    "nav": {
      "features": "Features",
      "how_it_works": "How it works",
      "pricing": "Pricing",
      "login": "Log in",
      "menu_open": "Open menu",
      "menu_close": "Close menu"
    },
    "auth": {
      "tab_login": "Log in",
      "tab_signup": "Sign up",
      "email_label": "Email",
      "password_label": "Password",
      "google_cta": "Continue with Google",
      "error_credentials": "Email or password is incorrect.",
      "error_unreachable": "We couldn't reach the server. Try again.",
      "error_rate_limited": "Too many attempts. Wait a moment and try again."
    }
  }
}
```

Rules:

- **Name keys for meaning, not for the English words.** `auth.error_credentials`, not
  `auth.email_or_password_is_incorrect`. When the English copy is reworded — and it will be
  — a meaning-named key survives; a text-named key becomes a lie.
- **Never reuse one key in two places** just because the English happens to match. "Save"
  as a verb on a button and "Save" in a menu can translate differently in Vietnamese;
  sharing the key makes one of them permanently wrong.
- **`en-US.json` is the source of truth.** New copy is added there first.
- **Every key in `en-US.json` must exist in `vi-VN.json`.** A missing key is a CI failure,
  not a runtime fallback we shrug at — see §6.

---

## 2a. Voice-critical strings: translate faithfully, not "better"

Some strings — the hero slogan, the punchline, anything that is the brand's
voice rather than incidental UI copy — carry a specific claim, not just a
vibe. "EVERY SINGLE DAY" is an intensified claim about frequency. A fluent
but reinterpreted Vietnamese line that swaps in a different idea (e.g. one
that claims "no rest" instead of "every day") is not a better translation —
it's a different sentence that happens to sit in the same place.

The failure mode here is specifically the opposite of the usual translation
sin. Normally we worry about translations that are too literal and read as
stilted. For a slogan, the temptation runs the other way: producing something
that sounds punchier or more natural in Vietnamese by drifting from what the
source actually asserts. Resist that. If the literal translation reads
awkwardly, fix the awkwardness without changing the claim — don't reach for a
locally punchier idea instead.

Mark voice-critical keys as such (a comment in the source catalog, or list
them in the story/PR that introduces them) so a future translator — human or
agent — knows these specific strings need sign-off from someone fluent in
both the language and the intended meaning, not just a fluency check.

## 3. Composing strings

**Never build a sentence by concatenation.** Word order differs between English and
Vietnamese, so `t('you_have') + count + t('sets_left')` cannot be translated correctly, no
matter how the translator tries.

Use interpolation, with the whole sentence in one key:

```json
{ "analytics.volume.summary": "You logged {{count}} hard sets for {{muscle}} this week." }
```

```ts
t('analytics.volume.summary', { count: sets, muscle: t(`muscles.${muscleKey}`) })
```

Same for anything with embedded markup (a link inside a sentence): use `<Trans>` so the
translator controls where the link sits in the sentence, rather than splitting the sentence
into fragments around it.

**Pluralization** goes through i18next's plural handling, never a hand-rolled
`count === 1 ? x : y`. Vietnamese has no plural inflection, so `vi-VN` catalogs typically
carry a single form where English carries `_one` and `_other` — that asymmetry is normal
and expected, and it's exactly why the ternary approach breaks.

---

## 4. Numbers, dates and units — the part that matters most here

GrindStats is a numbers product, and this is where a naive i18n implementation does real
damage rather than cosmetic damage.

- **Vietnamese separators are inverted relative to English.** `vi-VN` uses `.` for
  thousands and `,` for decimals. `2.620` in Vietnamese is **2620**, not 2.62. A user
  reading a TDEE of "2.620 kcal" as two-point-six is a genuine safety-adjacent bug in a
  calorie app.
- **Every displayed number goes through `Intl.NumberFormat`** with the active locale.
  Never interpolate a raw JS number or a `toFixed()` result into the DOM.
- **Dates and times go through `Intl.DateTimeFormat`**, never a hand-built
  `DD/MM/YYYY`. Occurrence dates in particular must render in the user's locale while
  remaining an unambiguous ISO date on the wire.
- **The wire format is not the display format.** APIs exchange ISO-8601 dates and raw
  numeric values. Formatting happens at the render boundary and nowhere else. Never send a
  locale-formatted number to the API, and never store one.
- **Units are a user preference, not a locale.** Locale sets the initial default (`vi-VN` →
  metric) but the user's explicit metric/imperial choice always wins and is stored on their
  profile. Unit *labels* (`kg`, `lb`, `kcal`) are translation keys, because they're read as
  words.

---

## 5. Locale detection, and what this decision costs

Detection order, configured in `src/i18n/index.ts`:

1. `?lng=` query parameter — **development and QA only**, but it must work in production
   builds too, because it is the only way to test or reproduce a `vi-VN` bug.
2. `navigator.language` / `Accept-Language`, matched to `vi-VN` for any `vi*` tag.
3. Fall back to `en-US`.

**Read this before treating locale support as done.** With auto-detection and no switcher:

- A Vietnamese speaker whose browser is set to English **cannot reach the Vietnamese
  interface at all**. That's a real population, not a hypothetical one.
- A Vietnamese page cannot be linked, bookmarked, or shared as Vietnamese, since the URL is
  identical for both languages.
- Search engines see one language per URL and will index only one.

These are accepted trade-offs for now, deliberately taken to keep the landing page story
small. A language switcher (and probably `/en` + `/vi` URL prefixes with a `lang` attribute
per locale) is a follow-up story, and the code should not make that harder: keep the locale
in i18next's control rather than scattering `navigator.language` checks through components.

`<html lang>` must be set from the active locale on every render path — screen readers pick
pronunciation from it, and getting Vietnamese read with English phonemes is unintelligible.

---

## 6. Fonts and text rendering

- **Any font adopted must ship the Vietnamese subset.** Verify before adopting, not after:
  check `ơ ư đ ă â ê ô` and the stacked-diacritic set `ế ộ ữ ằ ậ` render as glyphs rather
  than boxes or fallback-font mismatches.
- The **display face is the high-risk one** — condensed display fonts commonly drop
  Vietnamese. If the chosen display face fails this check, it is replaced, not worked
  around by rendering the hero in English only.
- Load the Vietnamese subset explicitly (`&subset=vietnamese` / the `unicode-range` the
  provider gives you) so Vietnamese users don't get an invisible fallback swap.
- Line-height tokens are set with stacked diacritics in mind; don't tighten them to suit an
  English mockup (see `docs/design-system.md` §3).

---

## 7. Definition of done for any UI work

1. No hardcoded user-facing strings anywhere, including `aria-label`, `alt`, `title` and `placeholder`.
2. Every key used exists in both `en-US.json` and `vi-VN.json`.
3. No string built by concatenation; plurals go through i18next.
4. Every number and date rendered through `Intl` with the active locale.
5. The screen has been viewed at `?lng=vi` with no clipped, overflowing or overlapping text.
6. No layout assumes English string length.
7. `<html lang>` reflects the active locale.
8. Vietnamese glyphs render correctly in every face used on the screen.

A CI check should enforce #2 mechanically — a script comparing the key sets of the two
catalogs and failing on any asymmetry. It's twenty lines and it's the difference between
catching a missing translation at commit time and finding it in production.
