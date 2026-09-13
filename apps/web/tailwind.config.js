/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],

  darkMode: ["selector", '.dark, [data-theme="dark"]'],

  theme: {
    extend: {
      colors: {
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
          3: "var(--ink-3)",
        },
        accent: {
          DEFAULT: "var(--accent)",
          hover: "var(--accent-hover)",
          ink: "var(--accent-ink)",
        },
        ok: "var(--ok)",
        warn: "var(--warn)",
        danger: "var(--danger)",
        "muted-data": "var(--muted-data)",
        "low-confidence": "var(--low-confidence)",
        "media-fg": "#ffffff",
        "media-scrim": "#000000",
      },
      fontFamily: {
        display: ['"Big Shoulders Display"', "Impact", "sans-serif"],
        body: ['"IBM Plex Sans"', "system-ui", "sans-serif"],
        mono: ['"IBM Plex Mono"', "ui-monospace", "monospace"],
      },
      maxWidth: {
        content: "var(--content-max)",
        measure: "68ch",
        "6xl": "72rem",
      },
      minHeight: {
        target: "var(--target-min)",
      },
      minWidth: {
        target: "var(--target-min)",
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
    },
  },

  plugins: [],
};
