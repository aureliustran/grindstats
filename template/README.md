# GrindStats — static landing template

Self-contained design template: `index.html` + `styles.css` + `app.js`. Tailwind via Play CDN,
icons via [Lucide](https://lucide.dev) (`data-lucide="…"`), fonts via Google Fonts (Vietnamese
subsets included). No build step — open `index.html` directly, or `npx serve template`.

Tokens mirror `apps/web/src/styles/tokens.css` (`--paper-*`, `--ink-*`, `--accent`), so
anything approved here ports to the React slice without re-picking values.

## Effects included

| Effect | Where | Respects reduced motion |
|---|---|---|
| Liquid glass (blur + saturate + refracted highlight) | header, cards, modal | n/a |
| Cursor spotlight on cards | `.spotlight` | n/a |
| 3D tilt | hero checklist card | yes |
| Magnetic CTA | primary buttons `.magnetic` | yes |
| Film grain overlay | whole page | yes |
| Ambient accent glow | fixed background | n/a |
| Regimen marquee ticker | below hero | yes |
| Scroll reveal + staggered delays (`--d`) | every `.reveal` | yes |
| Animated counters (`data-count`, `data-decimals`) | hero card, Numbers | yes (jumps to final) |
| Parallax backdrop (`data-parallax`) | hero image slot | yes |
| Segmented tab with sliding thumb | auth modal | — |
| Light/dark toggle (persisted) | header | — |

## Media

`media/saitama-vs-garou.mp4` fills **image slot 1, the hero backdrop** — muted, looping,
autoplaying, desaturated + darkened (`.hero-bg__video`) so the regimen type stays legible.
If the file is missing in a checkout, `app.js` catches the `<video>` error event and falls
back to the same diagonal placeholder fill the other empty slots use.

`media/download.jpg` fills **image slot 7, the Pricing side art** — framed at `aspect-[3/4]`
next to the plan cards, normalized to grayscale/contrast (`.pricing-art img`) and blended
into the section with the same accent-glow treatment used behind the hero checklist card,
so it reads as part of the system rather than a pasted-in photo.

Both are real *One Punch Man* content (Saitama vs. Garou footage; illustrated Saitama/Boros
art), included here as permanent content. They also ship in `apps/web` — see
`docs/design-system.md` §5.

**Image slot 5** (the editorial photo next to the quote) is a **Pinterest pin embed**
(`<iframe src="https://assets.pinterest.com/ext/embed.html?...">`), not an image file.
Pinterest's embed is a fixed 345×714 box, title/publisher caption included; `.pin-embed-wrap`
+ `scalePinEmbed()` in `app.js` measure the column's actual width and scale the whole box down
proportionally (`transform: scale()`, with the wrapper's height adjusted to match) rather than
stretching or cropping it, so the caption at the bottom never gets cut off. It gets the same
`grayscale(1) contrast(1.05)` filter as the other slots for consistency — CSS `filter` applies
visually to a cross-origin iframe's rendered output, so this works even though the content is
on `pinterest.com`. Remove the filter in `.pin-embed iframe` if the pin's original color should
show. Swap the `id=` in the `src` URL for a different pin without touching anything else.

The scaling is deliberately layered (an immediate call, a `ResizeObserver` on the wrapper, a
`resize` listener, and two delayed `requestAnimationFrame` calls) because a single early
measurement races the Tailwind Play CDN's asynchronous stylesheet injection — the real grid
width isn't final until that finishes, and injecting a `<style>` tag fires no resize event to
recover from a stale measurement.

## Image slots — what to send me

Each `.img-slot` shows a dashed label until you drop an `<img>` inside it (the label hides
automatically via `:has(img)`). Suggested sources: Unsplash / Pexels (free, attribution-free),
or your own shots. Everything should be **monochrome or desaturated, high contrast** — the
accent stays the only colour.

| # | Slot (`data-slot`) | What | Size | Ideas / search terms |
|---|---|---|---|---|
| 1 | `hero-backdrop` | ~~Wide atmospheric backdrop~~ — **filled** with `media/saitama-vs-garou.mp4`, see Media above | 2400×1600 | — |
| 2 | `screen-checklist` | App screenshot / mockup: daily checklist | 1080×1350 | I can build these three as HTML mockups if you don't have screens yet |
| 3 | `screen-trend` | App screenshot: weight trend chart with confidence band | 1080×1350 | " |
| 4 | `screen-coach` | App screenshot: coach message with cited numbers | 1080×1350 | " |
| 5 | `portrait` | ~~Editorial photo~~ — **filled** with a Pinterest pin embed, see Media above | 345×714 (fixed) | — |
| 6 | `cta-texture` | Texture for the final CTA block | 2000×800 | asphalt, concrete, chalkboard, worn rubber floor |
| 7 | `pricing-side` | ~~Pricing side art~~ — **filled** with `media/download.jpg`, see Media above | 3:4 | — |
| — | `avatar` | Small round mark next to the quote | 80×80 | a logo mark or leave as is |

Drop-in: `<div class="img-slot …" data-slot="hero-backdrop"><img src="img/hero.jpg" alt="" /></div>`.

## Icons in use (Lucide)

activity · calendar-check · arrow-right · play · shield-check · sigma · camera · flame ·
check-square · square · ruler · scale · dumbbell · brain-circuit · route · calculator · tag ·
check · minus · sparkles · sun · moon · menu · x · mail · lock · github.

Swap any with another Lucide name; if you'd rather use Phosphor or Heroicons, tell me and I'll
switch the loader.

## Other ideas not built yet (say the word)

- **Scroll-driven hero**: the regimen lines stack in one by one as you scroll (CSS scroll-timeline).
- **Live number strip**: a thin bar under the header showing today's kcal in/out counting live.
- **Before/after slider** on the Numbers section (formula prior vs adaptive TDEE).
- **Interactive checklist demo** — tick boxes in the hero card and watch the kcal/progress update.
- **Lottie / SVG line-draw** of the silhouette on load.
- **Testimonial wall** (needs real quotes) or a **"streak calendar" heatmap** as a visual.
