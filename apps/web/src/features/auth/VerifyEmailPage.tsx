/* =============================================================================
   VerifyEmailPage — handles email confirmation from emailed link.
   ============================================================================= */

import { useEffect, useState } from "react";
import { useSearchParams, Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { CheckCircle, AlertTriangle } from "lucide-react";
import { authApi } from "../../api/auth";

type VerifyState = "verifying" | "success" | "error";

export function VerifyEmailPage() {
  const { t } = useTranslation();
  const [searchParams] = useSearchParams();
  const token = searchParams.get("token")?.trim();

  const [state, setState] = useState<VerifyState>(() => (token ? "verifying" : "error"));

  useEffect(() => {
    if (!token) {
      setState("error");
      return;
    }

    let cancelled = false;
    authApi
      .verifyEmail({ token })
      .then(() => {
        if (!cancelled) setState("success");
      })
      .catch(() => {
        if (!cancelled) setState("error");
      });

    return () => {
      cancelled = true;
    };
  }, [token]);

  return (
    <main className="flex min-h-screen items-center justify-center bg-paper-1 p-4">
      <div className="w-full max-w-md rounded-md border border-paper-3 bg-paper-0 p-6 shadow-sm">
        <h1 className="text-xl font-semibold text-ink-0">
          {t("auth.verify_email.title")}
        </h1>

        {state === "verifying" && (
          <p className="mt-4 text-sm text-ink-2">{t("common.loading")}...</p>
        )}

        {state === "success" && (
          <div className="mt-4 space-y-4">
            <div className="flex items-center gap-2 text-ok">
              <CheckCircle className="h-5 w-5 shrink-0" aria-hidden="true" />
              <p className="text-sm font-medium">
                {t("auth.verify_email.success_message")}
              </p>
            </div>
            <Link
              to="/"
              className="inline-flex min-h-target items-center justify-center rounded-sm bg-accent px-4 py-2 text-sm font-medium text-accent-ink transition-colors hover:bg-accent-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
            >
              {t("auth.verify_email.action_login")}
            </Link>
          </div>
        )}

        {state === "error" && (
          <div className="mt-4 space-y-4">
            <div className="flex items-center gap-2 text-danger">
              <AlertTriangle className="h-5 w-5 shrink-0" aria-hidden="true" />
              <p className="text-sm">
                {t("auth.verify_email.invalid_or_expired")}
              </p>
            </div>
            <Link
              to="/"
              className="inline-flex min-h-target items-center justify-center rounded-sm border border-paper-3 bg-paper-0 px-4 py-2 text-sm font-medium text-ink-0 transition-colors hover:bg-paper-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
            >
              {t("auth.common.back_to_home")}
            </Link>
          </div>
        )}
      </div>
    </main>
  );
}
