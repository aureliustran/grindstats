/* =============================================================================
   Tailwind configuration — GrindStats
   =============================================================================

   THIS FILE DEFINES NO DESIGN VALUES. Every entry below maps a Tailwind utility
   name onto a CSS custom property declared in src/styles/tokens.css.

   Why it's built this way: we use Tailwind and plain CSS together. Two styling
   systems only coexist without drifting if exactly one of them owns the values.
   tokens.css owns them. This file is a bridge, not a second source of truth.

   So: `bg-paper-1` and `var(--paper-1)` are the same value by construction, and
   a token edited in tokens.css updates both systems at once — including dark
   theme, since the custom properties are what get swapped there.

   If you are ever tempted to write a literal here (a hex, a px, a ms), stop:
   that value belongs in tokens.css with a comment explaining its job.

   See docs/design-system.md §2.
   ============================================================================= */

/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],

  // Dark theme is driven by BOTH the system preference and an explicit
  // [data-theme] attribute, matching the two blocks in tokens.css. Using the
  // attribute selector here lets a future theme toggle override the system
  // preference in either direction.
  darkMode: ["selector", '[data-theme="dark"]'],

  theme: {
    // `extend` is deliberately NOT used for these scales — they replace
    // Tailwind's defaults outright. Leaving Tailwind's palette and spacing in
    // place would let someone write `bg-slate-400` or `p-[13px]` and bypass the
    // token system without anyone noticing in review.
    colors: {
      transparent: "transparent",
      current: "currentColor",

      paper: {
        0: "var(--paper-0)",
        1: "var(--paper-1)",
        2: "var(--paper-2)",
        3: "var(--paper-3)",
      },
      ink: {
        0: "var(--ink-0)",
        1: "var(--ink-1)",
        2: "var(--ink-2)",
        // ink-3 fails 4.5:1 — large text and non-text elements only.
        3: "var(--ink-3)",
      },

      accent: {
        DEFAULT: "var(--accent)",
        hover: "var(--accent-hover)",
        ink: "var(--accent-ink)",
      },

      // Semantic only. If you reach for these to decorate, the design is wrong.
      ok: "var(--ok)",
      warn: "var(--warn)",
      danger: "var(--danger)",
      "muted-data": "var(--muted-data)",
      "low-confidence": "var(--low-confidence)",
    },

    spacing: {
      0: "0",
      1: "var(--space-1)",
      2: "var(--space-2)",
      3: "var(--space-3)",
      4: "var(--space-4)",
      6: "var(--space-6)",
      8: "var(--space-8)",
      12: "var(--space-12)",
      16: "var(--space-16)",
      24: "var(--space-24)",
      32: "var(--space-32)",
      target: "var(--target-min)",
    },

    fontFamily: {
      display: "var(--font-display)",
      body: "var(--font-body)",
      // Numbers always. Pair with the .u-num class (or font-variant-numeric)
      // so figures are tabular — see globals.css.
      mono: "var(--font-mono)",
    },

    fontSize: {
      xs: "var(--text-xs)",
      sm: "var(--text-sm)",
      base: "var(--text-base)",
      lg: "var(--text-lg)",
      xl: "var(--text-xl)",
      "2xl": "var(--text-2xl)",
      "3xl": "var(--text-3xl)",
      "4xl": "var(--text-4xl)",
      "5xl": "var(--text-5xl)",
      hero: "var(--text-hero)",
    },

    fontWeight: {
      regular: "var(--weight-regular)",
      medium: "var(--weight-medium)",
      bold: "var(--weight-bold)",
      black: "var(--weight-black)",
    },

    // Set for Vietnamese stacked diacritics, not English. Don't tighten these
    // because an English mockup looks airy — see docs/i18n-guidelines.md §6.
    lineHeight: {
      display: "var(--leading-display)",
      tight: "var(--leading-tight)",
      normal: "var(--leading-normal)",
      relaxed: "var(--leading-relaxed)",
    },

    letterSpacing: {
      tight: "var(--tracking-tight)",
      normal: "var(--tracking-normal)",
      wide: "var(--tracking-wide)",
    },

    borderRadius: {
      none: "var(--radius-0)",
      sm: "var(--radius-1)",
      DEFAULT: "var(--radius-2)",
      // No pill/full token. The aesthetic is near-square on purpose.
    },

    borderWidth: {
      0: "0",
      DEFAULT: "var(--border-hairline)",
      heavy: "var(--border-heavy)",
    },

    zIndex: {
      base: "var(--z-base)",
      sticky: "var(--z-sticky)",
      dropdown: "var(--z-dropdown)",
      backdrop: "var(--z-backdrop)",
      modal: "var(--z-modal)",
      toast: "var(--z-toast)",
    },

    transitionDuration: {
      fast: "var(--motion-fast)",
      DEFAULT: "var(--motion-base)",
      slow: "var(--motion-slow)",
    },

    transitionTimingFunction: {
      out: "var(--ease-out)",
      "in-out": "var(--ease-in-out)",
    },

    // Breakpoints are the one thing that CANNOT come from custom properties —
    // CSS variables don't work in media queries. These literals are therefore
    // the source of truth for breakpoints, and tokens.css documents them in
    // prose so the numbers are stated in both places on purpose. md (768px) is
    // the mobile-menu breakpoint.
    screens: {
      sm: "640px",
      md: "768px",
      lg: "1024px",
      xl: "1280px",
    },

    extend: {
      maxWidth: {
        content: "var(--content-max)",
        measure: "68ch",
      },
    },
  },

  plugins: [],
};
