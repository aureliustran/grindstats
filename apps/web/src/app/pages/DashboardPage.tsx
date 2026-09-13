/* =============================================================================
   Dashboard — protected placeholder page.
   Unauthenticated visitors are redirected to / by the router.
   ============================================================================= */

import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../AuthContext";
import { UnverifiedBanner } from "../../features/auth";

export function DashboardPage() {
  const { t } = useTranslation();
  const { user, logout, emailVerified } = useAuth();
  const navigate = useNavigate();

  async function handleLogout() {
    await logout();
    navigate("/", { replace: true });
  }

  return (
    <div className="flex min-h-screen flex-col bg-paper-1">
      <UnverifiedBanner emailVerified={emailVerified} />
      <main className="flex flex-1 flex-col items-center justify-center gap-4 p-4">
        <h1 className="text-2xl font-bold text-ink-0">{t("app.dashboard.heading")}</h1>
        {user && (
          <p className="text-base text-ink-2">
            {t("app.dashboard.signed_in_as", { email: user.email })}
          </p>
        )}
        <p className="text-base text-ink-2">{t("app.dashboard.placeholder")}</p>
        <button
          type="button"
          disabled={!emailVerified}
          data-testid="dashboard-write-control"
          className="min-h-[44px] rounded-sm bg-accent px-4 py-2 text-base font-medium text-accent-ink transition-colors hover:bg-accent-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent disabled:cursor-not-allowed disabled:opacity-50"
        >
          {t("app.dashboard.logout")}
        </button>
        <button
          type="button"
          onClick={() => void handleLogout()}
          className="min-h-[44px] rounded-sm border border-paper-3 bg-paper-0 px-4 py-2 text-base font-medium text-ink-0 hover:bg-paper-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
        >
          {t("app.dashboard.logout")}
        </button>
      </main>
    </div>
  );
}
