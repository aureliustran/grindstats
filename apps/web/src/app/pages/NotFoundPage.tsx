/* =============================================================================
   404 — not found page.
   ============================================================================= */

import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";

export function NotFoundPage() {
  const { t } = useTranslation();

  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-4 bg-paper-1 p-4">
      <h1 className="text-2xl font-bold text-ink-0">{t("app.not_found.heading")}</h1>
      <Link to="/" className="text-base text-accent underline">
        {t("app.not_found.back")}
      </Link>
    </main>
  );
}
