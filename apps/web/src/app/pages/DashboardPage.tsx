/* =============================================================================
   Dashboard — protected placeholder page.
   Unauthenticated visitors are redirected to / by the router.
   ============================================================================= */

import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../AuthContext";

export function DashboardPage() {
  const { t } = useTranslation();
  const { user, logout } = useAuth();
  const navigate = useNavigate();

  async function handleLogout() {
    await logout();
    navigate("/", { replace: true });
  }

  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-4 bg-paper-1 p-4">
      <h1 className="text-2xl font-bold text-ink-0">{t("app.dashboard.heading")}</h1>
      {user && (
        <p className="text-base text-ink-2">
          {t("app.dashboard.signed_in_as", { email: user.email })}
        </p>
      )}
      <p className="text-base text-ink-2">{t("app.dashboard.placeholder")}</p>
      <button
        type="button"
        onClick={() => void handleLogout()}
        className="rounded bg-accent px-4 py-2 text-base font-medium text-accent-ink"
      >
        {t("app.dashboard.logout")}
      </button>
    </main>
  );
}
