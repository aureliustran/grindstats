/* =============================================================================
   i18n setup — GrindStats
   =============================================================================

   Locales: en-US (source of truth) and vi-VN.
   Library: react-i18next.

   THE RULE THIS FILE EXISTS TO ENFORCE:
   No user-facing string is ever written into a component. Not a label, not a
   button, not an error, not an aria-label, not a placeholder, not alt text.
   A codebase with 95% of its strings externalized isn't 95% translated — it's
   broken in Vietnamese in five places nobody can find. See
   docs/i18n-guidelines.md for the full rules.

   LOCALE SELECTION — auto-detection only, no user-facing switcher yet.
   This is a deliberate scope decision with real costs; read §5 of the
   guidelines before assuming locale support is finished. The short version:
   a Vietnamese speaker whose browser is set to English currently cannot reach
   the Vietnamese interface at all. A switcher is a follow-up story.

   Because of that, keep locale resolution in i18next's hands. Do not scatter
   navigator.language checks through components — that's what would make adding
   the switcher painful later.
   ============================================================================= */

import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import LanguageDetector from "i18next-browser-languagedetector";

import enUS from "./locales/en-US.json";
import viVN from "./locales/vi-VN.json";

export const SUPPORTED_LOCALES = ["en-US", "vi-VN"] as const;
export type SupportedLocale = (typeof SUPPORTED_LOCALES)[number];
export const DEFAULT_LOCALE: SupportedLocale = "en-US";

void i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources: {
      "en-US": { translation: enUS },
      "vi-VN": { translation: viVN },
    },
    supportedLngs: SUPPORTED_LOCALES,
    fallbackLng: DEFAULT_LOCALE,

    // Maps any vi* tag (vi, vi-VN, vi-Hani-VN) onto our vi-VN catalog rather
    // than falling through to English. Without this, a browser reporting plain
    // "vi" silently gets the English UI.
    load: "languageOnly",
    nonExplicitSupportedLngs: true,

    detection: {
      // Order matters:
      //   querystring — ?lng=vi. Development and QA only in intent, but it must
      //     work in production builds too: with no switcher, this is the ONLY
      //     way to reproduce or test a vi-VN bug. Do not strip it from prod.
      //   navigator   — Accept-Language / navigator.language. The real
      //     mechanism for actual users today.
      order: ["querystring", "navigator"],
      lookupQuerystring: "lng",
      // No caching to cookie/localStorage: with detection-only selection there
      // is no user choice to remember, and a stale cache would mean a visitor
      // whose browser language changed keeps getting the old locale. When the
      // switcher story lands, add the cache here — that's the natural place.
      caches: [],
    },

    interpolation: {
      // React escapes by default; double-escaping mangles Vietnamese text.
      escapeValue: false,
    },

    returnEmptyString: false,
  });

/* <html lang> must track the active locale on every render path. Screen readers
   choose pronunciation from it, and Vietnamese read with English phonemes is
   unintelligible. This is wired here rather than in a component so it cannot be
   forgotten on a route that doesn't happen to render the layout. */
const applyDocumentLanguage = (locale: string) => {
  document.documentElement.setAttribute("lang", locale);
};

applyDocumentLanguage(i18n.resolvedLanguage ?? DEFAULT_LOCALE);
i18n.on("languageChanged", applyDocumentLanguage);

export default i18n;

/* -----------------------------------------------------------------------------
   FORMATTING HELPERS

   Every number and date the user sees goes through these. This is not a style
   preference — vi-VN inverts the separators, so "2.620" means 2620 in
   Vietnamese, not 2.62. A user reading a TDEE of "2.620 kcal" as two-point-six
   is a genuine safety-adjacent bug in a calorie app.

   Never interpolate a raw number or a toFixed() result into the DOM.
   The wire format is not the display format: APIs exchange ISO dates and raw
   numeric values; formatting happens only at the render boundary.
   -------------------------------------------------------------------------- */

export const formatNumber = (
  value: number,
  options?: Intl.NumberFormatOptions,
): string =>
  new Intl.NumberFormat(i18n.resolvedLanguage ?? DEFAULT_LOCALE, options).format(value);

export const formatDate = (
  value: Date | string,
  options?: Intl.DateTimeFormatOptions,
): string =>
  new Intl.DateTimeFormat(i18n.resolvedLanguage ?? DEFAULT_LOCALE, options).format(
    typeof value === "string" ? new Date(value) : value,
  );
