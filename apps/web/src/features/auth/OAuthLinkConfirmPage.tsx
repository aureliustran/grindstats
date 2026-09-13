/* =============================================================================
   OAuthLinkConfirmPage — confirms linking Google OAuth to an existing local account.
   ============================================================================= */

import { useState } from "react";
import { useSearchParams, Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { CheckCircle, AlertTriangle } from "lucide-react";
import { authApi } from "../../api/auth";

export function OAuthLinkConfirmPage() {
  const { t } = useTranslation();
  const [searchParams] = useSearchParams();
  const token = searchParams.get("token")?.trim();

  const [loading, setLoading] = useState(false);
  const [success, setSuccess] = useState(false);
  const [error, setError] = useState<boolean>(() => !token);

  if (!token) {
    return (
      <main className="flex min-h-screen items-center justify-center bg-paper-1 p-4">
        <div className="w-full max-w-md rounded-md border border-paper-3 bg-paper-0 p-6 shadow-sm">
          <h1 className="text-xl font-semibold text-ink-0">
            {t("auth.oauth_link.title")}
          </h1>
          <div className="mt-4 space-y-4">
            <div className="flex items-center gap-2 text-danger">
              <AlertTriangle className="h-5 w-5 shrink-0" aria-hidden="true" />
              <p className="text-sm">
                {t("auth.oauth_link.invalid_token")}
              </p>
            </div>
            <Link
              to="/"
              className="inline-flex min-h-target items-center justify-center rounded-sm border border-paper-3 bg-paper-0 px-4 py-2 text-sm font-medium text-ink-0 transition-colors hover:bg-paper-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
            >
              {t("auth.common.back_to_home")}
            </Link>
          </div>
        </div>
      </main>
    );
  }

  async function handleConfirm() {
    if (!token || loading) return;
    setLoading(true);
    setError(false);

    try {
      await authApi.confirmOAuthLink({ token });
      setSuccess(true);
    } catch {
      setError(true);
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="flex min-h-screen items-center justify-center bg-paper-1 p-4">
      <div className="w-full max-w-md rounded-md border border-paper-3 bg-paper-0 p-6 shadow-sm">
        <h1 className="text-xl font-semibold text-ink-0">
          {t("auth.oauth_link.title")}
        </h1>

        {success ? (
          <div className="mt-4 space-y-4">
            <div className="flex items-center gap-2 text-ok">
              <CheckCircle className="h-5 w-5 shrink-0" aria-hidden="true" />
              <p className="text-sm font-medium">
                {t("auth.oauth_link.success_message")}
              </p>
            </div>
            <Link
              to="/"
              className="inline-flex min-h-target items-center justify-center rounded-sm bg-accent px-4 py-2 text-sm font-medium text-accent-ink transition-colors hover:bg-accent-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
            >
              {t("auth.common.back_to_login")}
            </Link>
          </div>
        ) : error ? (
          <div className="mt-4 space-y-4">
            <div className="flex items-center gap-2 text-danger">
              <AlertTriangle className="h-5 w-5 shrink-0" aria-hidden="true" />
              <p className="text-sm">
                {t("auth.oauth_link.invalid_token")}
              </p>
            </div>
            <Link
              to="/"
              className="inline-flex min-h-target items-center justify-center rounded-sm border border-paper-3 bg-paper-0 px-4 py-2 text-sm font-medium text-ink-0 transition-colors hover:bg-paper-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
            >
              {t("auth.common.back_to_home")}
            </Link>
          </div>
        ) : (
          <div className="mt-4 space-y-4">
            <p className="text-sm text-ink-1">
              {t("auth.oauth_link.link_required_notice")}
            </p>
            <button
              type="button"
              onClick={handleConfirm}
              disabled={loading}
              className="flex w-full min-h-target items-center justify-center rounded-sm bg-accent px-4 py-2 text-sm font-medium text-accent-ink transition-colors hover:bg-accent-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent disabled:opacity-50"
            >
              {loading ? `${t("common.loading")}...` : t("auth.oauth_link.confirm_button")}
            </button>
          </div>
        )}
      </div>
    </main>
  );
}
