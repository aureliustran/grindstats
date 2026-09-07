/* =============================================================================
   Mock Google OAuth consent page — stand-in for Google's real consent screen.
   Only rendered when the mock is active (the route is only registered then).

   Approve → creates/finds google.user@example.com, writes session, marks
             authenticated, navigates to /dashboard.
   Deny    → navigates to /?auth_error=oauth_cancelled.
   ============================================================================= */

import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { mockOAuth } from "../../api/auth";
import { useAuth } from "../AuthContext";

export function MockOAuthPage() {
  const { t } = useTranslation();
  const { markAuthenticated } = useAuth();
  const navigate = useNavigate();

  async function handleApprove() {
    const session = mockOAuth();
    await markAuthenticated({ csrfToken: session.csrfToken });
    navigate("/dashboard", { replace: true });
  }

  function handleDeny() {
    navigate("/?auth_error=oauth_cancelled", { replace: true });
  }

  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-6 bg-paper-1 p-4">
      <div className="flex w-full max-w-content flex-col gap-4 rounded bg-paper-0 p-8">
        <h1 className="text-xl font-bold text-ink-0">{t("app.mock_oauth.heading")}</h1>
        <p className="text-base text-ink-2">{t("app.mock_oauth.body")}</p>
        <div className="flex gap-3">
          <button
            type="button"
            onClick={() => void handleApprove()}
            className="rounded bg-accent px-4 py-2 text-base font-medium text-accent-ink"
          >
            {t("app.mock_oauth.approve")}
          </button>
          <button
            type="button"
            onClick={handleDeny}
            className="rounded bg-paper-2 px-4 py-2 text-base font-medium text-ink-1"
          >
            {t("app.mock_oauth.deny")}
          </button>
        </div>
      </div>
    </main>
  );
}
